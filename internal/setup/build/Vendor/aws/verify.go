package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// manifest — 정규화 직전 rootfs 의 파일별 요약: 경로 → "모드 종류 내용해시(또는 링크 대상)". 측정에 들어가는 것 그대로다
// (파일 내용과 모드; 시각·소유자는 재포장이 0 으로 고정하니 뺀다). 두 빌드가 다를 때 어느 파일이 달랐는지 여기서 나온다.
type manifest map[string]string

// readManifest — 풀어낸 트리는 root 소유라 runner(sudo)로 find 한다. 파일은 sha256, 링크는 대상, 디렉터리는 모드만.
func readManifest(ctx context.Context, r runner, root string) (manifest, error) {
	m := manifest{}
	out, err := r.output(ctx, "find", root, "-mindepth", "1", "(", "-type", "f", "-o", "-type", "l", "-o", "-type", "d", ")",
		"-printf", "%m %y %p\t%l\n")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		head, link, _ := strings.Cut(line, "\t")
		f := strings.SplitN(head, " ", 3)
		if len(f) != 3 {
			continue
		}
		rel := strings.TrimPrefix(f[2], root)
		switch f[1] {
		case "l":
			m[rel] = f[0] + " l → " + link
		case "d":
			m[rel] = f[0] + " d"
		default:
			m[rel] = f[0] + " f"
		}
	}
	out, err = r.output(ctx, "find", root, "-type", "f", "-exec", "sha256sum", "{}", "+")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		sum, path, ok := strings.Cut(line, "  ")
		if !ok {
			continue
		}
		rel := strings.TrimPrefix(path, root)
		m[rel] = m[rel] + " " + sum
	}
	return m, nil
}

// diff — b 가 a 와 다른 곳. 한쪽에만 있는 것, 내용·모드·링크가 다른 것. 경로순.
func (a manifest) diff(b manifest) []string {
	var out []string
	for p, va := range a {
		vb, ok := b[p]
		switch {
		case !ok:
			out = append(out, p+"  only in build 1")
		case va != vb:
			out = append(out, fmt.Sprintf("%s  build 1: %s  build 2: %s", p, short(va), short(vb)))
		}
	}
	for p := range b {
		if _, ok := a[p]; !ok {
			out = append(out, p+"  only in build 2")
		}
	}
	sort.Strings(out)
	return out
}

func short(v string) string {
	f := strings.Fields(v)
	if len(f) == 3 && len(f[2]) > 12 {
		f[2] = f[2][:12] + "…"
	}
	return strings.Join(f, " ")
}

// verify — 같은 Dockerfile 을 캐시 없이 한 번 더 빌드해 같은 PCR 이 나오는지 본다. 정적 검사(lint)는 "달라질 수 있다"까지고,
// 이게 "정말 달라지는가"다. 같은 호스트에서 두 번 같다고 다른 호스트에서도 같다는 보장은 아니지만, 여기서 다르면 어디서도
// 안 같다 — 그리고 어느 파일 때문인지 바로 나온다(실측: redis 는 redis-server 바이너리, haproxy 는 apk db 두 파일).
// 다르면 오류를 돌려준다: 첫 빌드의 EIF·측정값은 이미 out/ 에 있지만, 그 값을 넘겨봐야 Auditor 가 재현 못 한다.
func (b Build) verify(ctx context.Context, r runner, appDir, appImage string, first manifest, firstPCR map[string]string) error {
	checkApp := appImage + "-verify"
	checkImage := b.Name + ":tb-verify"
	fmt.Fprintf(r.log, "\n[verify 1/3] building %s again from scratch (--no-cache) → %s\n", appDir, checkApp)
	if err := r.run(ctx, "docker", "build", "-q", "--no-cache", "-t", checkApp, appDir); err != nil {
		return err
	}
	defer r.cmd(ctx, "docker", "rmi", "-f", checkApp, checkImage).Run()
	appCmd, err := r.imageCommand(ctx, checkApp)
	if err != nil {
		return err
	}
	if err := b.layerResponder(ctx, r, checkApp, checkImage, appCmd); err != nil {
		return err
	}
	fmt.Fprintf(r.log, "\n[verify 2/3] repacking the second build\n")
	second, err := normalize(ctx, r, checkImage, b.Out)
	if err != nil {
		return err
	}
	eif := filepath.Join(b.Out, b.Name+".verify.eif")
	defer os.Remove(eif)
	fmt.Fprintf(r.log, "\n[verify 3/3] measuring the second build\n")
	out, err := r.output(ctx, "nitro-cli", "build-enclave", "--docker-uri", checkImage, "--output-file", eif)
	if err != nil {
		return err
	}
	pcr, err := ReadPCR(out)
	if err != nil {
		return err
	}
	same := true
	for _, k := range []string{"PCR0", "PCR1", "PCR2"} {
		if pcr[k] != firstPCR[k] {
			same = false
			fmt.Fprintf(r.log, "%s differs: build 1 %s / build 2 %s\n", k, firstPCR[k], pcr[k])
		}
	}
	if same {
		fmt.Fprintln(r.log, "reproducible on this host: a second build from scratch gave the same PCR0·1·2")
		return nil
	}
	d := first.diff(second)
	fmt.Fprintf(r.log, "%d path(s) differ between the two builds (mode type sha256):\n", len(d))
	for i, line := range d {
		if i == 40 {
			fmt.Fprintf(r.log, "  … %d more\n", len(d)-i)
			break
		}
		fmt.Fprintln(r.log, "  "+line)
	}
	return fmt.Errorf("not reproducible: the same Dockerfile built twice on this host gave different measurements (%d path(s) above). fix the Dockerfile before handing the value on — the Auditor's rebuild will not match either", len(d))
}
