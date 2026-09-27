// Package aws 는 A 실행·배포, AWS Nitro: 빌드가 낸 EIF 를 엔클레이브로 띄우고, 부모 인스턴스에 TCP→vsock 다리를 놓고,
// 보안 그룹 포트를 연다(port.go). 설계서(6·8쪽)가 "다 띄운 뒤 설정값에 적으라"는 RA 주소가 Result 다.
//
// 인스턴스 자체는 Case 1(문서) — 사용자가 Nitro Enclaves 를 켜서 직접 만들고 들어온다. 이 패키지는 그 안에서
// nitro-cli 만 부르고, 보안 그룹은 권한이 있을 때만 건드린다. 클라우드 자격증명은 쓰지 않는다.
package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Launch 는 한 번의 실행 입력. EIF 는 build 의 결과다.
type Launch struct {
	Name   string // 엔클레이브 이름. 비우면 EIF 파일 이름
	EIF    string // 이미지 파일
	Port   int    // RA 포트 (다리가 받고 엔클레이브 응답기가 듣는). 0 이면 5000
	Memory int    // 엔클레이브 메모리 MiB. 0 이면 EIF 크기로 정한다
	CPUs   int    // 0 이면 2
	CID    int    // 엔클레이브 vsock 주소. 0 이면 16
	Listen string // 다리가 받을 주소. 비우면 0.0.0.0
	Binary string // ra-proxy 로 띄울 tb 바이너리. 비우면 자기 자신
	Out    string // 다리 로그 디렉터리. 비우면 out

	// Measurements — 남이 준 EIF 와 같이 온 measurements.json (Case A). EIF 를 직접 잰 값이 이것과 다르면 띄우지 않는다.
	// 내가 build 로 만든 EIF(Case B)에는 줄 게 없으니 비운다.
	Measurements string
}

// Result — 설정값에 적을 것.
type Result struct {
	Name         string
	Measurements string            // out/measurements.json — 이 EIF 를 nitro-cli 로 직접 잰 값
	PCR          map[string]string // PCR0 · PCR1 · PCR2
	Memory  int    // 실제로 준 메모리 MiB
	Address string // 이 인스턴스에서 확인할 주소 127.0.0.1:port. 밖에서는 <공인 IP>:port
}

var ErrNotLinux = errors.New("launch runs on the Nitro parent instance (linux)")

// Run — 도구 · 자원 → 엔클레이브 실행 → 다리.
func (l Launch) Run(ctx context.Context, log io.Writer) (Result, error) {
	var res Result
	if runtime.GOOS != "linux" {
		return res, ErrNotLinux
	}
	if l.EIF == "" {
		return res, errors.New("eif is required")
	}
	if _, err := os.Stat(l.EIF); err != nil {
		return res, err
	}
	if l.Name == "" {
		l.Name = strings.TrimSuffix(filepath.Base(l.EIF), ".eif")
	}
	if l.Port == 0 {
		l.Port = 5000
	}
	if l.CPUs == 0 {
		l.CPUs = 2
	}
	if l.CID == 0 {
		l.CID = 16
	}
	if l.Listen == "" {
		l.Listen = "0.0.0.0"
	}
	if l.Out == "" {
		l.Out = "out"
	}
	if l.Memory == 0 {
		l.Memory = MemoryFor(l.EIF)
	}
	var err error
	if l.Binary == "" {
		if l.Binary, err = os.Executable(); err != nil {
			return res, err
		}
	}
	if err := os.MkdirAll(l.Out, 0o755); err != nil {
		return res, err
	}
	res.Name, res.Memory = l.Name, l.Memory
	r := runner{log: log, sudo: os.Geteuid() != 0}

	fmt.Fprintf(log, "== launch 1/4 tools and resources (%d cpu · %d MiB)\n", l.CPUs, l.Memory)
	if err := r.run(ctx, "dnf", "install", "-y", "-q", "aws-nitro-enclaves-cli", "aws-nitro-enclaves-cli-devel"); err != nil {
		return res, err
	}
	if err := r.run(ctx, "systemctl", "enable", "--now", "nitro-enclaves-allocator.service"); err != nil {
		return res, err
	}
	if err := r.ensureAllocator(ctx, l.Memory, l.CPUs); err != nil {
		return res, err
	}

	fmt.Fprintf(log, "== launch 2/4 measuring the EIF (nitro-cli describe-eif)\n")
	if res.PCR, err = measureEIF(ctx, r, l.EIF); err != nil {
		return res, err
	}
	for _, k := range imagePCRs {
		fmt.Fprintf(log, "%s %s\n", k, res.PCR[k])
	}
	res.Measurements = filepath.Join(l.Out, "measurements.json")
	wrote, err := writeMeasurements(res.Measurements, res.PCR)
	if err != nil {
		return res, err
	}
	if wrote {
		fmt.Fprintf(log, "→ %s\n", res.Measurements)
	} else {
		fmt.Fprintf(log, "= %s (build's file, same values)\n", res.Measurements)
	}
	if l.Measurements != "" {
		reasons, err := checkAgainst(res.PCR, l.Measurements)
		if err != nil {
			return res, err
		}
		if len(reasons) > 0 {
			fmt.Fprintf(log, "FAIL against %s\n", l.Measurements)
			for _, why := range reasons {
				fmt.Fprintf(log, "  %s\n", why)
			}
			return res, errMeasurementMismatch
		}
		fmt.Fprintf(log, "PASS — the EIF measures exactly what %s claims\n", l.Measurements)
	}

	fmt.Fprintf(log, "== launch 3/4 running the enclave (cid %d)\n", l.CID)
	_ = r.quiet(ctx, "nitro-cli", "terminate-enclave", "--enclave-name", l.Name)
	if err := r.run(ctx, "nitro-cli", "run-enclave", "--eif-path", l.EIF, "--cpu-count", strconv.Itoa(l.CPUs),
		"--memory", strconv.Itoa(l.Memory), "--enclave-cid", strconv.Itoa(l.CID), "--enclave-name", l.Name); err != nil {
		return res, err
	}
	time.Sleep(3 * time.Second)
	if err := r.enclaveRunning(ctx, l.Name); err != nil {
		return res, err
	}

	fmt.Fprintf(log, "== launch 4/4 bridge TCP %s:%d → vsock %d:%d\n", l.Listen, l.Port, l.CID, l.Port)
	if err := l.startProxy(ctx, log); err != nil {
		return res, err
	}
	res.Address = "127.0.0.1:" + strconv.Itoa(l.Port)
	return res, nil
}

// startProxy — tb 바이너리를 ra-proxy 로 따로 띄운다. launch 가 끝나도 남는다.
func (l Launch) startProxy(ctx context.Context, log io.Writer) error {
	_ = exec.CommandContext(ctx, "pkill", "-f", fmt.Sprintf("^%s ra-proxy --listen %s:%d", l.Binary, l.Listen, l.Port)).Run()
	logPath := filepath.Join(l.Out, "proxy.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(l.Binary, "ra-proxy", "--listen", fmt.Sprintf("%s:%d", l.Listen, l.Port), "--vsock", fmt.Sprintf("%d:%d", l.CID, l.Port))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = detached()
	if err := cmd.Start(); err != nil {
		return err
	}
	time.Sleep(time.Second)
	b, _ := os.ReadFile(logPath)
	fmt.Fprint(log, string(b))
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}

// MemoryFor — EIF 는 통째로 RAM 에 풀리고 그 위에서 소프트웨어가 돈다. 크기의 4 배를 512 MiB 단위로 올린다.
func MemoryFor(eif string) int {
	st, err := os.Stat(eif)
	if err != nil {
		return 512
	}
	return memoryForSize(st.Size())
}

func memoryForSize(size int64) int {
	mib := int(size / (1 << 20))
	m := (mib*4 + 511) / 512 * 512
	if m < 512 {
		m = 512
	}
	return m
}

// runner — nitro-cli · systemctl 을 부른다. root 가 아니면 sudo 를 앞에 붙인다.
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

func (r runner) quiet(ctx context.Context, name string, args ...string) error {
	return r.cmd(ctx, name, args...).Run()
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

func (r runner) enclaveRunning(ctx context.Context, name string) error {
	out, err := r.output(ctx, "nitro-cli", "describe-enclaves")
	if err != nil {
		return err
	}
	var list []struct {
		EnclaveName, EnclaveID, State string
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return fmt.Errorf("describe-enclaves: %w", err)
	}
	for _, e := range list {
		if e.EnclaveName == name {
			fmt.Fprintf(r.log, "enclave %s %s\n", e.EnclaveID, e.State)
			if e.State != "RUNNING" {
				return fmt.Errorf("enclave is %s", e.State)
			}
			return nil
		}
	}
	return errors.New("enclave is not running — the software may have exited at once; run nitro-cli run-enclave with --debug-mode --attach-console to see why")
}

const allocatorFile = "/etc/nitro_enclaves/allocator.yaml"

// ensureAllocator — 엔클레이브에 줄 메모리 · CPU 가 부족하면 늘린다. 늘리면 돌고 있던 엔클레이브는 내려간다.
func (r runner) ensureAllocator(ctx context.Context, memory, cpus int) error {
	b, err := os.ReadFile(allocatorFile)
	if err != nil {
		return err
	}
	curMem, curCPU := 0, 0
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if v, ok := strings.CutPrefix(l, "memory_mib:"); ok {
			curMem, _ = strconv.Atoi(strings.TrimSpace(v))
			if curMem < memory {
				lines[i] = "memory_mib: " + strconv.Itoa(memory)
			}
		}
		if v, ok := strings.CutPrefix(l, "cpu_count:"); ok {
			curCPU, _ = strconv.Atoi(strings.TrimSpace(v))
			if curCPU < cpus {
				lines[i] = "cpu_count: " + strconv.Itoa(cpus)
			}
		}
	}
	if curMem >= memory && curCPU >= cpus {
		return nil
	}
	fmt.Fprintf(r.log, "enclave resources %d MiB / %d cpu → %d MiB / %d cpu\n", curMem, curCPU, max(curMem, memory), max(curCPU, cpus))
	c := r.cmd(ctx, "tee", allocatorFile)
	c.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	if err := c.Run(); err != nil {
		return fmt.Errorf("write %s: %w", allocatorFile, err)
	}
	return r.run(ctx, "systemctl", "restart", "nitro-enclaves-allocator.service")
}
