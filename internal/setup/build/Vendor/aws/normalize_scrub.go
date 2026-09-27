package aws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// scratchDirs — 실행 시점엔 어차피 비어 있어야 하는 스크래치. 빌드 중 mktemp 등이 남긴 파일은 이름이 랜덤이라 매 빌드
// 다르다(실측: memcached 공식 alpine Dockerfile 이 /tmp/<랜덤> 파일 하나를 남겨, 바이너리는 모두 같은데 PCR0·PCR2 가
// 갈렸다). 디렉터리는 두고 안만 비운다.
var scratchDirs = []string{"tmp", "var/tmp"}

// scrubBuildScratch — scratchDirs 안을 비우고, 실제로 비운 디렉터리의 상대 경로를 돌려준다.
func scrubBuildScratch(ctx context.Context, r runner, root string) []string {
	var scrubbed []string
	for _, d := range scratchDirs {
		dir := filepath.Join(root, filepath.FromSlash(d))
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) == 0 {
			continue
		}
		for _, e := range entries {
			full := filepath.Join(dir, e.Name())
			if err := os.RemoveAll(full); err != nil {
				if err := r.cmd(ctx, "rm", "-rf", full).Run(); err != nil {
					fmt.Fprintf(r.log, "warning: could not remove %s: %v\n", full, err)
				}
			}
		}
		scrubbed = append(scrubbed, d)
	}
	return scrubbed
}

// apkVirtualVersion — apk add --virtual 이 붙이는 기본 버전: 설치 시각 YYYYMMDD.HHMMSS.
var apkVirtualVersion = regexp.MustCompile(`^[0-9]{8}\.[0-9]{6}$`)

// apkFixedIdentity — 고정한 버전에 맞춰 넣는 자리표시 식별자. apk 는 엔클레이브 안에서 다시 돌지 않으니 형식만 지킨다.
const apkFixedIdentity = "C:Q10000000000000000000000000000="

// fixApkVirtualVersions — apk 의 가상 패키지(--virtual, 이름이 . 으로 시작, 파일 없이 의존성만 묶는 것)는 버전이 기본으로
// 설치 시각이라 /etc/apk/world 와 /lib/apk/db/installed 에 빌드 시각이 박힌다(실측: haproxy · memcached 공식 alpine —
// 파일 수백 개 중 이 둘만 달라 PCR0·PCR2 가 갈렸다). 무엇을 깔았나가 아니라 언제 깔았나이므로 버전을 0 으로 고정하고,
// 그 버전에서 나온 식별자(C:)는 자리표시로 바꾼다. 돌려주는 것은 고친 파일들의 상대 경로.
func fixApkVirtualVersions(ctx context.Context, r runner, root string) []string {
	var fixed []string

	world := filepath.Join(root, "etc", "apk", "world")
	if b, err := readRoot(ctx, r, world); err == nil {
		changed := false
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		for i, line := range lines {
			if name, ver, ok := strings.Cut(line, "="); ok && strings.HasPrefix(name, ".") && apkVirtualVersion.MatchString(ver) {
				lines[i] = name + "=0"
				changed = true
			}
		}
		if changed && writeRoot(ctx, r, world, []byte(strings.Join(lines, "\n")+"\n")) == nil {
			fixed = append(fixed, "etc/apk/world")
		}
	}

	installed := filepath.Join(root, "lib", "apk", "db", "installed")
	if b, err := readRoot(ctx, r, installed); err == nil {
		changed := false
		blocks := strings.Split(string(b), "\n\n")
		for i, blk := range blocks {
			lines := strings.Split(blk, "\n")
			virtual := false
			for _, l := range lines {
				if strings.HasPrefix(l, "P:.") {
					virtual = true
					break
				}
			}
			if !virtual {
				continue
			}
			for j, l := range lines {
				switch {
				case strings.HasPrefix(l, "V:") && apkVirtualVersion.MatchString(l[2:]):
					lines[j] = "V:0"
					changed = true
				case strings.HasPrefix(l, "C:") && l != apkFixedIdentity:
					lines[j] = apkFixedIdentity
					changed = true
				}
			}
			blocks[i] = strings.Join(lines, "\n")
		}
		if changed && writeRoot(ctx, r, installed, []byte(strings.Join(blocks, "\n\n"))) == nil {
			fixed = append(fixed, "lib/apk/db/installed")
		}
	}
	return fixed
}

// buildCaches — 빌드 중 도구가 남기는 캐시. 내용이 파일 내용이 아니라 시각·inode 같은 그때의 상태라 같은 소스를 빌드해도
// 매번 다르고, 엔클레이브 안에서 다시 쓰이지 않는다(실측: 두 인스턴스에서 mariadb 공식 Dockerfile — 13,063 항목 중
// root/.wget-hsts(wget 의 HSTS 저장소, 방문 시각) 와 var/cache/ldconfig/aux-cache(ldconfig 가 라이브러리 stat 을 적어 둔
// 캐시) 둘만 달라 PCR0·PCR2 가 갈렸다. tomcat 도 aux-cache 가 달랐다). 지운다.
var buildCaches = []string{
	"var/cache/ldconfig/aux-cache", // glibc ldconfig: 라이브러리의 inode·mtime 을 적는다. ld.so.cache 는 내용 기반이라 둔다
	"root/.wget-hsts",              // wget: 방문한 HSTS 호스트와 시각
	"root/.gnupg/random_seed",      // gnupg: 난수 시드
	"root/.cache",                  // pip · go · npm 등이 두는 캐시 트리
}

// stripBuildCaches — 풀어낸 rootfs 에서 buildCaches 를 지우고, 실제로 지운 것의 상대 경로를 돌려준다.
func stripBuildCaches(ctx context.Context, r runner, root string) []string {
	var removed []string
	for _, p := range buildCaches {
		full := filepath.Join(root, filepath.FromSlash(p))
		if !exists(ctx, r, full) {
			continue
		}
		if err := os.RemoveAll(full); err != nil {
			if err := r.cmd(ctx, "rm", "-rf", full).Run(); err != nil {
				fmt.Fprintf(r.log, "warning: could not remove %s: %v\n", p, err)
				continue
			}
		}
		removed = append(removed, p)
	}
	return removed
}

// exists — 풀어낸 트리에서 경로가 있는지. /root(700) · /var/cache/ldconfig(700) 처럼 root 만 들여다볼 수 있는 디렉터리 안은
// os.Lstat 이 권한 오류를 내므로(실측: mariadb 의 root/.wget-hsts 가 그래서 안 지워졌다) 그때는 runner(sudo)의 test -e 로 본다.
func exists(ctx context.Context, r runner, path string) bool {
	_, err := os.Lstat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return r.cmd(ctx, "test", "-e", path).Run() == nil
}

// readRoot · writeRoot — 풀어낸 rootfs 는 tar 가 sudo 로 풀어 root 소유다. 직접 안 되면 runner(sudo)로 읽고 쓴다.
func readRoot(ctx context.Context, r runner, path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		return b, nil
	}
	return r.output(ctx, "cat", path)
}

func writeRoot(ctx context.Context, r runner, path string, b []byte) error {
	if err := os.WriteFile(path, b, 0o644); err == nil {
		return nil
	}
	c := r.cmd(ctx, "tee", path)
	c.Stdin = bytes.NewReader(b)
	return c.Run()
}
