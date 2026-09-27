// Package aws 는 B 빌드, AWS Nitro, Case 2(자동화): 소프트웨어의 Dockerfile 을 받아 EIF 와 측정값(PCR0·1·2)을 낸다.
// 설계서(9·11쪽)가 "측정값 기록에 적으라"는 값이 Result 다. 도구(docker · nitro-cli)는 라이브러리가 아니라 exec 로 부른다.
//
// 설계서에 없던 한 단계가 여기 들어간다 — T1 응답기 얹기. Nitro 엔클레이브는 안에서만 /dev/nsm 을 열 수 있고
// 네트워크가 없다. 그래서 제3자가 RA 를 요청하려면 응답기가 이미지 안에 들어가 있어야 한다. 응답기 구현은
// tee/request/auditor/aws (Extractor · vsock) 에 그대로 있고, 이 바이너리의 ra-serve 가 그것을 띄운다.
// 빌드는 소프트웨어 이미지 위에 이 바이너리와 진입점 한 장을 얹기만 한다. 소프트웨어 이미지는 고치지 않는다.
package aws

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed entrypoint.sh
var entrypointScript []byte

// responderBinaryName — cmd/ra-responder 가 만드는 파일 이름. 모든 역할이 정확히 이 이름의
// 파일을 실행파일 옆에 두고 써야, 서로 다른 역할이 빌드해도 이미지에 같은 바이트가 얹힌다.
const responderBinaryName = "tb-ra-responder"

// Build 는 한 번의 빌드 입력. AppDir 의 Dockerfile 이 소프트웨어다.
type Build struct {
	AppDir string // 소프트웨어 Dockerfile 이 있는 디렉터리
	Name   string // 이미지 · EIF 이름. 비우면 디렉터리 이름
	Port   int    // 응답기가 엔클레이브 안에서 들을 vsock 포트. 0 이면 5000

	// Binary — 얹을 T1 응답기. 비우면 실행파일과 같은 디렉터리의 tb-ra-responder 를 찾는다.
	//
	// 예전에는 여기가 비면 자기 자신(os.Executable(), 즉 tb-vendor 나 tb-auditor)을 얹었다.
	// 그건 역할마다 다른 CLI 바이너리를 이미지에 넣는 셈이라, 레이어를 아무리 정규화해도
	// Vendor 와 Auditor 가 만드는 이미지가 절대 같을 수 없었다(실측: 실제 EC2 두 대에서
	// Vendor·Auditor 가 같은 Dockerfile 을 정규화까지 거쳐 각자 빌드했는데도 PCR0·PCR2 가
	// 갈림 — 원인은 /tb-ra 파일 자체가 두 바이너리라 서로 다른 파일이었던 것). 이제는 모든
	// 역할이 cmd/ra-responder 하나로 빌드한 동일한 파일을 얹어야 한다.
	Binary string
	Out    string // 결과 디렉터리. 비우면 out
	Verify bool   // 같은 Dockerfile 을 캐시 없이 한 번 더 빌드해 같은 PCR 이 나오는지 본다 (verify.go)
}

// Result — 측정값 기록에 적을 것.
type Result struct {
	Name         string
	Image        string            // 응답기를 얹은 docker 이미지 태그
	EIF          string            // 이미지 파일
	Measurements string            // nitro-cli build-enclave 출력 원문 (json)
	PCR          map[string]string // PCR0 · PCR1 · PCR2
}

var ErrNotLinux = errors.New("build runs on the Nitro parent instance (linux)")

// Run — 도구 → 소프트웨어 이미지 → T1 응답기 얹기 → EIF · PCR.
func (b Build) Run(ctx context.Context, log io.Writer) (Result, error) {
	var res Result
	if runtime.GOOS != "linux" {
		return res, ErrNotLinux
	}
	if b.AppDir == "" {
		return res, errors.New("app dir is required")
	}
	appDir, err := filepath.Abs(b.AppDir)
	if err != nil {
		return res, err
	}
	dockerfile := filepath.Join(appDir, "Dockerfile")
	if st, err := os.Stat(dockerfile); err != nil || st.IsDir() {
		return res, fmt.Errorf("%s: Dockerfile not found", appDir)
	}
	if b.Name == "" {
		b.Name = Sanitize(filepath.Base(appDir))
	}
	if b.Port == 0 {
		b.Port = 5000
	}
	if b.Out == "" {
		b.Out = "out"
	}
	if b.Binary == "" {
		exe, err := os.Executable()
		if err != nil {
			return res, err
		}
		b.Binary = filepath.Join(filepath.Dir(exe), responderBinaryName)
	}
	if _, err := os.Stat(b.Binary); err != nil {
		return res, fmt.Errorf("%s: T1 responder not found — build cmd/ra-responder and put it there, or point Binary at it", b.Binary)
	}
	if err := os.MkdirAll(b.Out, 0o755); err != nil {
		return res, err
	}
	res.Name = b.Name
	r := runner{log: log, sudo: os.Geteuid() != 0}

	risks, err := LintDockerfile(dockerfile)
	if err != nil {
		return res, err
	}
	printRisks(log, dockerfile, risks)

	fmt.Fprintln(log, "== build 1/5 tools (nitro-cli · docker)")
	if err := r.run(ctx, "dnf", "install", "-y", "-q", "aws-nitro-enclaves-cli", "aws-nitro-enclaves-cli-devel", "docker"); err != nil {
		return res, err
	}
	if err := r.run(ctx, "systemctl", "enable", "--now", "docker"); err != nil {
		return res, err
	}

	appImage := b.Name + ":app"
	res.Image = b.Name + ":tb"
	fmt.Fprintf(log, "== build 2/5 software image: %s → %s\n", appDir, appImage)
	if err := r.run(ctx, "docker", "build", "-q", "-t", appImage, appDir); err != nil {
		return res, err
	}

	fmt.Fprintf(log, "== build 3/5 layering the T1 responder: %s → %s\n", appImage, res.Image)
	appCmd, err := r.imageCommand(ctx, appImage)
	if err != nil {
		return res, err
	}
	fmt.Fprintf(log, "software command: %s\n", strings.Join(appCmd, " "))
	if err := b.layerResponder(ctx, r, appImage, res.Image, appCmd); err != nil {
		return res, err
	}

	fmt.Fprintf(log, "== build 4/5 reproducibility: dropping the layers and repacking in name order\n")
	first, err := normalize(ctx, r, res.Image, b.Out)
	if err != nil {
		return res, err
	}

	res.EIF = filepath.Join(b.Out, b.Name+".eif")
	res.Measurements = filepath.Join(b.Out, "measurements.json")
	fmt.Fprintf(log, "== build 5/5 EIF → %s\n", res.EIF)
	out, err := r.output(ctx, "nitro-cli", "build-enclave", "--docker-uri", res.Image, "--output-file", res.EIF)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(res.Measurements, out, 0o644); err != nil {
		return res, err
	}
	if res.PCR, err = ReadPCR(out); err != nil {
		return res, err
	}
	for _, k := range []string{"PCR0", "PCR1", "PCR2"} {
		fmt.Fprintf(log, "%s %s\n", k, res.PCR[k])
	}
	if b.Verify {
		if err := b.verify(ctx, r, appDir, appImage, first, res.PCR); err != nil {
			return res, err
		}
	}
	return res, nil
}

// layerResponder — 소프트웨어 이미지 위에 tb 바이너리와 진입점을 얹는다. 원래 명령은 파일로 넘긴다:
// nitro-cli 는 스크립트 진입점 뒤의 Docker CMD 를 넘겨주지 않는다.
func (b Build) layerResponder(ctx context.Context, r runner, from, to string, appCmd []string) error {
	dir, err := os.MkdirTemp("", "tb-ra-image")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bin, err := os.ReadFile(b.Binary)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tb-ra"), bin, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "entrypoint.sh"), entrypointScript, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tb-app-cmd"), []byte(strings.Join(appCmd, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	dockerfile := fmt.Sprintf(`FROM %s
COPY tb-ra /tb-ra
COPY entrypoint.sh /tb-entrypoint.sh
COPY tb-app-cmd /tb-app-cmd
ENV TB_RA_PORT=%d
ENTRYPOINT ["/tb-entrypoint.sh"]
`, from, b.Port)
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}
	return r.run(ctx, "docker", "build", "-q", "-t", to, dir)
}

// normalize — docker build 가 쌓은 레이어를 버리고, 파일을 이름순으로 우리가 직접 재포장한다.
//
// docker build 는 실행마다 레이어 내부 파일 순서·타임스탬프를 새로 정한다. 최종 파일 내용이
// 완전히 같아도(실측: sha256 전수 일치) 그 파일을 담은 레이어 tar 의 바이트 자체가 달라서,
// Vendor 와 Auditor 가 같은 소스로 각자 독립적으로 빌드하면 PCR0·PCR2 가 갈린다(실측: 서로
// 다른 두 물리 인스턴스에서 재현됨). export 로 레이어 구분을 버리고 파일만 뽑아서, 이름순
// 정렬 + 시각 고정(tar --sort=name --mtime=@0 --owner=0 --group=0)으로 재포장한 뒤 같은
// 태그로 import 하면, 서로 다른 물리 서버·시각에 독립적으로 빌드해도 PCR0·PCR1·PCR2 가
// 정확히 일치한다(실측 확인 — 아래 커밋 메시지의 값).
//
// import 는 이미지 설정(ENTRYPOINT·ENV)을 버리므로 원래 것을 다시 넣는다 - 그것도 측정
// 대상이다: 파일은 그대로 두고 ENTRYPOINT 만 바꿔도 PCR0·PCR2 가 달라지는 것을 실측 확인했다.
//
// workDir 는 실제 디스크(b.Out)를 써야 한다 - OS 기본 임시 디렉터리(/tmp)는 Amazon Linux 에서
// RAM 기반 tmpfs 로 몇 GiB 밖에 안 되는데, 여기서 내보낸 tar·풀어낸 파일·재포장한 tar 가
// 한꺼번에 그보다 커질 수 있다(실측: "wrote only N of 10240 bytes" 로 재현됨).
func normalize(ctx context.Context, r runner, image, workDir string) (manifest, error) {
	out, err := r.output(ctx, "docker", "image", "inspect", image)
	if err != nil {
		return nil, err
	}
	var v []struct {
		Config struct {
			Env        []string
			Entrypoint []string
			Cmd        []string
		}
	}
	if err := json.Unmarshal(out, &v); err != nil || len(v) != 1 {
		return nil, fmt.Errorf("docker inspect %s: unexpected output", image)
	}
	cfg := v[0].Config

	dir, err := os.MkdirTemp(workDir, "tb-canon-*")
	if err != nil {
		return nil, err
	}
	// tar 가 sudo 로 풀어 놓은 트리는 root 소유라 os.RemoveAll 로는 못 지운다 — 그대로 두면 빌드마다 out/ 에 쌓인다.
	defer func() {
		if os.RemoveAll(dir) != nil {
			r.cmd(ctx, "rm", "-rf", dir).Run()
		}
	}()

	container := fmt.Sprintf("tb-canon-%d", os.Getpid())
	r.cmd(ctx, "docker", "rm", "-f", container).Run() // 이전 실패 잔여물, 있으면 지운다
	if err := r.run(ctx, "docker", "create", "--name", container, image); err != nil {
		return nil, err
	}
	defer r.cmd(ctx, "docker", "rm", container).Run()

	rootfs := filepath.Join(dir, "rootfs.tar")
	if err := r.run(ctx, "docker", "export", container, "-o", rootfs); err != nil {
		return nil, err
	}
	extract := filepath.Join(dir, "extract")
	if err := os.Mkdir(extract, 0o755); err != nil {
		return nil, err
	}
	if err := r.run(ctx, "tar", "-xf", rootfs, "-C", extract); err != nil {
		return nil, err
	}
	os.Remove(rootfs) // 다 풀었으니 더 안 필요하다 - 재포장 중 피크 디스크 사용량을 줄인다
	for _, d := range scrubBuildScratch(ctx, r, extract) {
		fmt.Fprintf(r.log, "emptied build scratch: /%s\n", d)
	}
	for _, p := range fixApkVirtualVersions(ctx, r, extract) {
		fmt.Fprintf(r.log, "pinned apk virtual-package versions (they were the install time): /%s\n", p)
	}
	for _, p := range stripBuildLogs(ctx, r, extract) {
		fmt.Fprintf(r.log, "removed a log stamped with the build time: /%s\n", p)
	}
	for _, p := range stripBuildCaches(ctx, r, extract) {
		fmt.Fprintf(r.log, "removed a build-time cache (it records when, not what): /%s\n", p)
	}

	m, err := readManifest(ctx, r, extract)
	if err != nil {
		return nil, err
	}

	canon := filepath.Join(dir, "canon.tar")
	if err := r.run(ctx, "tar", "--sort=name", "--mtime=@0", "--owner=0", "--group=0", "--numeric-owner",
		"-cf", canon, "-C", extract, "."); err != nil {
		return nil, err
	}

	args := []string{"import"}
	for _, e := range cfg.Env {
		args = append(args, "--change", "ENV "+e)
	}
	if len(cfg.Entrypoint) > 0 {
		b, _ := json.Marshal(cfg.Entrypoint)
		args = append(args, "--change", "ENTRYPOINT "+string(b))
	}
	if len(cfg.Cmd) > 0 {
		b, _ := json.Marshal(cfg.Cmd)
		args = append(args, "--change", "CMD "+string(b))
	}
	args = append(args, canon, image)
	if err := r.run(ctx, "docker", args...); err != nil {
		return nil, err
	}
	return m, nil
}

// buildLogs — 패키지 관리자가 docker build 중에 남기는 로그. 무엇을 깔았나가 아니라 언제 깔았나를 적어서
// (apk: "Running `apk add …` at 2026-09-26 08:14:36") 같은 소스를 빌드해도 내용이 매번 다르다. 실측: nginx 공식
// alpine-slim Dockerfile 을 Vendor · Auditor 가 각자 빌드했을 때 725개 파일 중 /var/log/apk.log 하나만 달라
// PCR0·PCR2 가 갈렸다. 파일이 아니라 시각을 재는 것이니 측정 대상에서 뺀다. 소프트웨어 자체의 로그는 건드리지 않는다.
var buildLogs = []string{
	"var/log/apk.log",                                             // alpine
	"var/log/dpkg.log", "var/log/alternatives.log", "var/log/apt", // debian · ubuntu
	"var/log/yum.log", "var/log/dnf.log", "var/log/dnf.rpm.log", "var/log/dnf.librepo.log", "var/log/hawkey.log", // rhel · amazon
}

// stripBuildLogs — 풀어낸 rootfs 에서 buildLogs 를 지우고, 실제로 지운 것의 상대 경로를 돌려준다.
// rootfs 는 tar 가 sudo 로 풀어 root 소유라, 직접 못 지우면 runner(sudo rm)로 지운다.
func stripBuildLogs(ctx context.Context, r runner, root string) []string {
	var removed []string
	for _, p := range buildLogs {
		full := filepath.Join(root, filepath.FromSlash(p))
		if _, err := os.Lstat(full); err != nil {
			continue
		}
		if err := os.RemoveAll(full); err != nil {
			if err := r.cmd(ctx, "rm", "-rf", full).Run(); err != nil {
				fmt.Fprintf(r.log, "warning: could not remove %s — the build time stays in the measurements: %v\n", p, err)
				continue
			}
		}
		removed = append(removed, p)
	}
	return removed
}

// ReadPCR — nitro-cli build-enclave 출력에서 측정값을 꺼낸다.
func ReadPCR(buildOutput []byte) (map[string]string, error) {
	var v struct {
		Measurements map[string]string
	}
	if err := json.Unmarshal(buildOutput, &v); err != nil {
		return nil, fmt.Errorf("build-enclave output: %w", err)
	}
	return v.Measurements, nil
}

// Sanitize — 디렉터리 이름을 docker 태그 · 엔클레이브 이름으로 쓸 수 있게 다듬는다.
func Sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		out = "app"
	}
	return out
}

// runner — docker · nitro-cli · systemctl 을 부른다. root 가 아니면 sudo 를 앞에 붙인다.
type runner struct {
	log  io.Writer
	sudo bool
}

func (r runner) cmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	if r.sudo {
		return exec.CommandContext(ctx, "sudo", append([]string{name}, args...)...)
	}
	return exec.CommandContext(ctx, name, args...)
}

func (r runner) run(ctx context.Context, name string, args ...string) error {
	c := r.cmd(ctx, name, args...)
	c.Stdout = r.log
	c.Stderr = r.log
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, args[0], err)
	}
	return nil
}

func (r runner) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	c := r.cmd(ctx, name, args...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %v\n%s", name, args[0], err, stderr.String())
	}
	return out, nil
}

// imageCommand — 이미지의 ENTRYPOINT + CMD.
func (r runner) imageCommand(ctx context.Context, image string) ([]string, error) {
	out, err := r.output(ctx, "docker", "image", "inspect", image)
	if err != nil {
		return nil, err
	}
	var v []struct {
		Config struct {
			Entrypoint []string
			Cmd        []string
		}
	}
	if err := json.Unmarshal(out, &v); err != nil || len(v) != 1 {
		return nil, fmt.Errorf("docker inspect %s: unexpected output", image)
	}
	cmd := append(v[0].Config.Entrypoint, v[0].Config.Cmd...)
	if len(cmd) == 0 {
		return nil, fmt.Errorf("%s has no ENTRYPOINT or CMD; nothing to run", image)
	}
	return cmd, nil
}
