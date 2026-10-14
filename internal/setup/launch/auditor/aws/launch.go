// Package aws launches the EIF a build produced as an enclave, sets up a TCP-to-vsock bridge on the parent instance.
// OpenPort (port.go) opens the security group port. Result holds the RA address to write into the configuration once everything is up.
//
// The instance itself is created by the user, who enables Nitro Enclaves and logs in. This package only calls
// nitro-cli from there, touches the security group only when it has permission, and uses no cloud credentials.
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

	"attestamp/internal/cli"
)

// Launch is the input to one launch. The EIF is the result of build.
type Launch struct {
	EnclaveName string // enclave name. Defaults to the EIF file name
	EIF         string // enclave image file
	Port        int    // RA port (the bridge accepts on it and the enclave responder listens on it). 0 means 5000
	Memory      int    // enclave memory in MiB. 0 means sized from the EIF size
	CPUs        int    // 0 means 2
	CID         int    // enclave vsock address. 0 means 16
	Listen      string // address the bridge accepts on. Defaults to 0.0.0.0
	ProxyBinary string // att binary that runs ra-proxy. Defaults to the running executable
	OutDir      string // bridge log directory. Defaults to out

	// BuildMeasurements is the measurements.json that came with an EIF received from someone else. If measuring the EIF directly gives a different value, it is not launched.
	// Leave it empty for an EIF built here with build, since there is nothing to compare with.
	BuildMeasurements string
}

// Result is what goes into the configuration.
type Result struct {
	Name         string
	Measurements string            // out/measurements.json: values from measuring this EIF directly with nitro-cli
	PCR          map[string]string // PCR0 · PCR1 · PCR2
	Memory       int               // memory actually given, in MiB
	Address      string            // address to check from this instance: 127.0.0.1:port. From outside: <public IP>:port
}

var ErrNotLinux = errors.New("launch runs on the Nitro parent instance (linux)")

// Run goes through: tools and resources, enclave start, bridge.
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
	if l.EnclaveName == "" {
		l.EnclaveName = strings.TrimSuffix(filepath.Base(l.EIF), ".eif")
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
	if l.OutDir == "" {
		l.OutDir = "out"
	}
	if l.Memory == 0 {
		l.Memory = MemoryFor(l.EIF)
	}
	var err error
	if l.ProxyBinary == "" {
		if l.ProxyBinary, err = os.Executable(); err != nil {
			return res, err
		}
	}
	if err := os.MkdirAll(l.OutDir, 0o755); err != nil {
		return res, err
	}
	res.Name, res.Memory = l.EnclaveName, l.Memory
	r := runner{log: log, sudo: os.Geteuid() != 0}

	launchStep(log, 1, fmt.Sprintf("Reserving enclave resources (%d cpu · %d MiB)", l.CPUs, l.Memory), "an enclave takes CPUs and memory away from this host for itself; the allocator sets them aside")
	if err := r.run(ctx, "dnf", "install", "-y", "-q", "aws-nitro-enclaves-cli", "aws-nitro-enclaves-cli-devel"); err != nil {
		return res, err
	}
	if err := r.run(ctx, "systemctl", "enable", "--now", "nitro-enclaves-allocator.service"); err != nil {
		return res, err
	}
	if err := r.ensureAllocator(ctx, l.Memory, l.CPUs); err != nil {
		return res, err
	}

	// Only an EIF received from someone else (it comes with --build-measurements) is measured again. For one built or packed here, the values measured then are already in out/.
	if l.BuildMeasurements != "" {
		launchStep(log, 2, "Measuring the received enclave image myself (nitro-cli describe-eif)", "the sender's measurements.json is only a claim. the file itself is measured here and must say the same")
		if res.PCR, err = measureEIF(ctx, r, l.EIF); err != nil {
			return res, err
		}
		for _, k := range imagePCRs {
			fmt.Fprintf(log, "    %s %s\n", k, res.PCR[k])
		}
		reasons, err := checkAgainst(res.PCR, l.BuildMeasurements)
		if err != nil {
			return res, err
		}
		if len(reasons) > 0 {
			fmt.Fprintln(log, cli.Red("✗ it does NOT measure to what "+l.BuildMeasurements+" says; not running it"))
			for _, why := range reasons {
				fmt.Fprintln(log, cli.Red("    "+why))
			}
			return res, errMeasurementMismatch
		}
		fmt.Fprintln(log, cli.Green("✓ it measures exactly what "+l.BuildMeasurements+" says")+":", "the file is what the sender described; safe to run")
		res.Measurements = filepath.Join(l.OutDir, "measurements.json")
		if _, err := writeMeasurements(res.Measurements, res.PCR); err != nil {
			return res, err
		}
		fmt.Fprintf(log, "    → %s (what I measured myself; this is the value to register)\n", res.Measurements)
	} else {
		launchStep(log, 2, "Skipping the measurement", "this is my own enclave image: build/pack measured it when it was made, and that value is already in out/measurements.json")
	}

	launchStep(log, 3, fmt.Sprintf("Starting the enclave (cid %d)", l.CID), "the image boots inside the Nitro enclave; from here on nothing on this host can read or change its memory")
	_ = r.quiet(ctx, "nitro-cli", "terminate-enclave", "--enclave-name", l.EnclaveName)
	if err := r.run(ctx, "nitro-cli", "run-enclave", "--eif-path", l.EIF, "--cpu-count", strconv.Itoa(l.CPUs),
		"--memory", strconv.Itoa(l.Memory), "--enclave-cid", strconv.Itoa(l.CID), "--enclave-name", l.EnclaveName); err != nil {
		return res, err
	}
	time.Sleep(3 * time.Second)
	if err := r.enclaveRunning(ctx, l.EnclaveName); err != nil {
		return res, err
	}

	launchStep(log, 4, fmt.Sprintf("Opening TCP %s:%d → the enclave (vsock %d:%d)", l.Listen, l.Port, l.CID, l.Port), "an enclave has no network of its own; this bridge on the host forwards that port to it so the Auditor and Users can ask it for attestations")
	if err := l.startProxy(ctx, log); err != nil {
		return res, err
	}
	res.Address = "127.0.0.1:" + strconv.Itoa(l.Port)
	return res, nil
}

// startProxy starts ProxyBinary as a detached ra-proxy process that keeps running after launch ends.
func (l Launch) startProxy(ctx context.Context, log io.Writer) error {
	_ = exec.CommandContext(ctx, "pkill", "-f", fmt.Sprintf("^%s ra-proxy --listen %s:%d", l.ProxyBinary, l.Listen, l.Port)).Run()
	logPath := filepath.Join(l.OutDir, "proxy.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(l.ProxyBinary, "ra-proxy", "--listen", fmt.Sprintf("%s:%d", l.Listen, l.Port), "--vsock", fmt.Sprintf("%d:%d", l.CID, l.Port))
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

// MemoryFor sizes enclave memory for an EIF: the EIF is unpacked entirely into RAM and the software runs on top of it,
// so it returns four times the EIF size, rounded up to 512 MiB units (minimum 512).
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

// runner calls nitro-cli and systemctl. It prefixes sudo when not root.
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
	return errors.New("enclave is not running: the software may have exited at once; run nitro-cli run-enclave with --debug-mode --attach-console to see why")
}

const allocatorFile = "/etc/nitro_enclaves/allocator.yaml"

// ensureAllocator raises the memory and CPUs reserved for enclaves if they fall short. Raising them takes down any enclave that was running.
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

// launchStep: two lines per step, what is being done and one sentence on why.
func launchStep(log io.Writer, n int, what, why string) {
	fmt.Fprintf(log, "\n%s %s…\n             %s\n", cli.Green(fmt.Sprintf("[launch %d/4]", n)), what, why)
}
