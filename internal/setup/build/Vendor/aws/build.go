// Package aws 는 B 빌드, AWS Nitro, Case 2(자동화): 소프트웨어의 Dockerfile 을 받아 EIF 와 측정값(PCR0·1·2)을 낸다.
// 설계서(9·11쪽)가 "측정값 기록에 적으라"는 값이 Result 다. 도구(docker · nitro-cli)는 라이브러리가 아니라 exec 로 부른다.
//
// 설계서에 없던 한 단계가 여기 들어간다 — T1 응답기 얹기. Nitro 엔클레이브는 안에서만 /dev/nsm 을 열 수 있고
// 네트워크가 없다. 그래서 제3자가 RA 를 요청하려면 응답기가 이미지 안에 들어가 있어야 한다. 응답기 구현은
// tee/request/Vendor/aws (Extractor · vsock) 에 그대로 있고, 이 바이너리의 ra-serve 가 그것을 띄운다.
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

// Build 는 한 번의 빌드 입력. AppDir 의 Dockerfile 이 소프트웨어다.
type Build struct {
	AppDir string // 소프트웨어 Dockerfile 이 있는 디렉터리
	Name   string // 이미지 · EIF 이름. 비우면 디렉터리 이름
	Port   int    // 응답기가 엔클레이브 안에서 들을 vsock 포트. 0 이면 5000
	Binary string // 얹을 tb 바이너리 (ra-serve · lo-up 가 들어 있는). 비우면 자기 자신
	Out    string // 결과 디렉터리. 비우면 out
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
	if st, err := os.Stat(filepath.Join(appDir, "Dockerfile")); err != nil || st.IsDir() {
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
		if b.Binary, err = os.Executable(); err != nil {
			return res, err
		}
	}
	if err := os.MkdirAll(b.Out, 0o755); err != nil {
		return res, err
	}
	res.Name = b.Name
	r := runner{log: log, sudo: os.Geteuid() != 0}

	fmt.Fprintln(log, "== build 1/4 도구 (nitro-cli · docker)")
	if err := r.run(ctx, "dnf", "install", "-y", "-q", "aws-nitro-enclaves-cli", "aws-nitro-enclaves-cli-devel", "docker"); err != nil {
		return res, err
	}
	if err := r.run(ctx, "systemctl", "enable", "--now", "docker"); err != nil {
		return res, err
	}

	appImage := b.Name + ":app"
	res.Image = b.Name + ":tb"
	fmt.Fprintf(log, "== build 2/4 소프트웨어 이미지: %s → %s\n", appDir, appImage)
	if err := r.run(ctx, "docker", "build", "-q", "-t", appImage, appDir); err != nil {
		return res, err
	}

	fmt.Fprintf(log, "== build 3/4 T1 응답기 얹기: %s → %s\n", appImage, res.Image)
	appCmd, err := r.imageCommand(ctx, appImage)
	if err != nil {
		return res, err
	}
	fmt.Fprintf(log, "소프트웨어 명령: %s\n", strings.Join(appCmd, " "))
	if err := b.layerResponder(ctx, r, appImage, res.Image, appCmd); err != nil {
		return res, err
	}

	res.EIF = filepath.Join(b.Out, b.Name+".eif")
	res.Measurements = filepath.Join(b.Out, "measurements.json")
	fmt.Fprintf(log, "== build 4/4 EIF → %s\n", res.EIF)
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
