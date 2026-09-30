// build_env — 명령줄 끝에서 끝까지. tb-vendor send 가 자기 빌드 환경(OS·아키텍처·Go)을 build-config.json 에
// 적고, tb-auditor receive 가 그것을 자기 인스턴스와 견줘 다르면 거절한다(--ignore-env 면 경고만).
// 원격은 로컬 bare 저장소, 실제 git 이 돈다 — internal/core/transfer 의 git_test 와 같은 방식.
package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRemoteRepo — bare 저장소 하나에 main.go 한 개를 seed 로 넣고, 그 경로와 seed 커밋을 돌려준다.
func newRemoteRepo(t *testing.T) (remote, seedCommit string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	remote = filepath.Join(t.TempDir(), "remote.git")
	git(t, "", "init", "--quiet", "--bare", "--initial-branch=main", remote)
	seed := cloneRemote(t, remote)
	if err := os.WriteFile(filepath.Join(seed, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "add", "main.go")
	git(t, seed, "commit", "--quiet", "-m", "seed")
	git(t, seed, "push", "--quiet", "-u", "origin", "main")
	return remote, git(t, seed, "rev-parse", "HEAD")
}

func cloneRemote(t *testing.T, remote string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "work")
	git(t, "", "clone", "--quiet", remote, dir)
	git(t, dir, "config", "user.email", "t@t")
	git(t, dir, "config", "user.name", "t")
	return dir
}

// buildConfigJSON — send 에 넘길 설정값. build_env 는 비워 둔다: send 가 채우는지 보는 게 목적.
func buildConfigJSON(t *testing.T, commit string, env map[string]any) []byte {
	t.Helper()
	m := map[string]any{
		"schema": "tb.build-config/1", "service": "svc", "version": "1.2.0", "platform": "aws",
		"source":   map[string]string{"repo": "https://example.com/svc.git", "commit": commit},
		"tee_info": map[string]string{"instance_id": "i-0abc", "region": "ap-northeast-2", "ami_id": "ami-0123"},
	}
	if env != nil {
		m["build_env"] = env
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// pushConfig — Vendor 를 흉내: build-config.json 을 원격에 커밋 · push (send 를 거치지 않고, 내용을 직접 정한다).
func pushConfig(t *testing.T, remote string, cfg []byte) {
	t.Helper()
	work := cloneRemote(t, remote)
	if err := os.WriteFile(filepath.Join(work, "build-config.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", "build-config.json")
	git(t, work, "commit", "--quiet", "-m", "cfg")
	git(t, work, "push", "--quiet")
}

func runCmd(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("%s %v: %v", bin, args, err)
		}
		code = ee.ExitCode()
	}
	return o.String(), e.String(), code
}

func otherArch() string {
	if runtime.GOARCH == "amd64" {
		return "arm64"
	}
	return "amd64"
}

// tb-vendor send 는 자기가 도는 OS/아키텍처/Go 를 build_env 로 적어 push 한다.
func TestBuildEnv_SendRecordsRunningEnv(t *testing.T) {
	dir := t.TempDir()
	vendor := build(t, dir, "./cmd/vendor")
	remote, _ := newRemoteRepo(t)
	cfgPath := filepath.Join(dir, "build-config.json")
	if err := os.WriteFile(cfgPath, buildConfigJSON(t, "", nil), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCmd(t, vendor, "send", "--repo", remote, "--config", cfgPath, "--workdir", cloneRemote(t, remote))
	if code != 0 {
		t.Fatalf("send exit %d\n%s%s", code, stdout, stderr)
	}
	wantLine := "build env: " + runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version()
	if !strings.Contains(stdout, wantLine) {
		t.Errorf("send must print the declared env %q, got:\n%s", wantLine, stdout)
	}

	// 원격에서 다시 받아 읽는다
	check := cloneRemote(t, remote)
	b, err := os.ReadFile(filepath.Join(check, "build-config.json"))
	if err != nil {
		t.Fatalf("build-config.json not pushed: %v", err)
	}
	var got struct {
		BuildEnv struct {
			OS       string `json:"os"`
			Arch     string `json:"arch"`
			Go       string `json:"go"`
			Trimpath bool   `json:"trimpath"`
		} `json:"build_env"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.BuildEnv.OS != runtime.GOOS || got.BuildEnv.Arch != runtime.GOARCH || got.BuildEnv.Go != runtime.Version() {
		t.Errorf("pushed build_env = %+v, want %s/%s %s", got.BuildEnv, runtime.GOOS, runtime.GOARCH, runtime.Version())
	}
}

// send → receive, 같은 기계: 받는 쪽이 자기 환경과 같다고 찍고 0 으로 끝난다.
func TestBuildEnv_ReceiveMatchingEnvPasses(t *testing.T) {
	dir := t.TempDir()
	vendor := build(t, dir, "./cmd/vendor")
	auditor := build(t, dir, "./cmd/auditor")
	remote, _ := newRemoteRepo(t)
	cfgPath := filepath.Join(dir, "build-config.json")
	if err := os.WriteFile(cfgPath, buildConfigJSON(t, "", nil), 0o644); err != nil {
		t.Fatal(err)
	}
	if stdout, stderr, code := runCmd(t, vendor, "send", "--repo", remote, "--config", cfgPath, "--workdir", cloneRemote(t, remote)); code != 0 {
		t.Fatalf("send exit %d\n%s%s", code, stdout, stderr)
	}

	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code != 0 {
		t.Fatalf("receive exit %d\n%s%s", code, stdout, stderr)
	}
	env := runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version()
	for _, want := range []string{"build env: " + env, "(Vendor)", "(this instance)", "build env matches the Vendor's"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("receive stdout must contain %q, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stderr, "cannot produce") {
		t.Errorf("same env must not be refused:\n%s", stderr)
	}
}

// Vendor 가 다른 아키텍처를 적어 두면 receive 는 두 아키텍처를 모두 이름 대며 거절한다(exit ≠ 0).
// --ignore-env 면 같은 말을 경고로만 찍고 0 으로 끝난다.
func TestBuildEnv_ReceiveRejectsOtherArch(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	pushConfig(t, remote, buildConfigJSON(t, seed, map[string]any{
		"os": runtime.GOOS, "arch": otherArch(), "go": runtime.Version(), "trimpath": true,
	}))

	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code == 0 {
		t.Fatalf("receive on the wrong architecture must fail\n%s%s", stdout, stderr)
	}
	for _, want := range []string{otherArch(), runtime.GOARCH, "cannot produce the Vendor's measurements", "--ignore-env"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr must name %q, got:\n%s", want, stderr)
		}
	}
	if !strings.Contains(stdout, "build env: "+runtime.GOOS+"/"+otherArch()) || !strings.Contains(stdout, "build env: "+runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("both envs must be printed before refusing, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "matches") {
		t.Errorf("must not claim a match:\n%s", stdout)
	}

	stdout, stderr, code = runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"), "--ignore-env")
	if code != 0 {
		t.Fatalf("--ignore-env must let it through, exit %d\n%s%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, otherArch()) {
		t.Errorf("--ignore-env must still warn naming the arch, got:\n%s", stderr)
	}
}

// Go 버전만 달라도 거절 — 툴체인이 바뀌면 측정값이 바뀐다.
func TestBuildEnv_ReceiveRejectsOtherGoVersion(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	pushConfig(t, remote, buildConfigJSON(t, seed, map[string]any{
		"os": runtime.GOOS, "arch": runtime.GOARCH, "go": "go1.0.0-not-this-one", "trimpath": true,
	}))
	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code == 0 {
		t.Fatalf("receive on another Go version must fail\n%s%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "go1.0.0-not-this-one") || !strings.Contains(stderr, runtime.Version()) {
		t.Errorf("stderr must name both Go versions, got:\n%s", stderr)
	}
}

// trimpath 없이 빌드한 Vendor 는 경고만 — 거절하지 않는다.
func TestBuildEnv_ReceiveWarnsOnNoTrimpath(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	pushConfig(t, remote, buildConfigJSON(t, seed, map[string]any{
		"os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(), "trimpath": false,
	}))
	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code != 0 {
		t.Fatalf("trimpath=false is a warning, not a refusal; exit %d\n%s%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "trimpath") {
		t.Errorf("want a trimpath warning on stderr, got:\n%s", stderr)
	}
}

// build_env 가 아예 없는(옛) build-config.json 은 aws 에서 받지 않는다.
func TestBuildEnv_ReceiveRejectsConfigWithoutBuildEnv(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	pushConfig(t, remote, buildConfigJSON(t, seed, nil))
	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code == 0 {
		t.Fatalf("a config without build_env must be rejected\n%s%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "build_env") {
		t.Errorf("stderr must say what is missing, got:\n%s", stderr)
	}
}
