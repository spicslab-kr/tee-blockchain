// 런타임 상태 보고 (GCP, AWS 응답기 공통). 이 파일은 두 응답기 폴더에 status.go 로 복사해서 쓴다.
//
// /status 는 질문 대신 "지금 VM 안에서 무엇이 돌고, 무엇이 열려 있고, 무엇이 붙어 있는가"를 돌려준다.
// 답과 똑같이 d = SHA-256(태그 || 난수 || SHA-256("attestamp-status") || SHA-256(보고서)) 를 증명에 넣으므로,
// 받는 쪽은 이 보고서가 등록된 이미지 안의 코드가 지금 만든 것인지 확인할 수 있다.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const statusPrompt = "attestamp-status"

// 닫힌 VM(첫 프로세스)에서만: 부팅 뒤 커널 안에서 코드를 바꾸는 길을 끈다.
// 모듈 싣기 금지, kexec 금지, 프로세스끼리 붙어 메모리 고치기(ptrace) 금지, 커널 잠금(integrity). 모두 켜면 재부팅 전까지 못 끈다. SysRq 끔.
func lockKernel() {
	for _, kv := range [][2]string{
		{"/proc/sys/kernel/kexec_load_disabled", "1"},
		{"/proc/sys/kernel/sysrq", "0"},
		{"/proc/sys/kernel/modules_disabled", "1"},
		{"/proc/sys/kernel/yama/ptrace_scope", "3"},
		{"/sys/kernel/security/lockdown", "integrity"},
	} {
		if err := os.WriteFile(kv[0], []byte(kv[1]), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "lock %s: %v\n", kv[0], err)
		}
	}
}

type procInfo struct {
	PID     int    `json:"pid"`
	PPID    int    `json:"ppid"`
	Exe     string `json:"exe"`
	Cmdline string `json:"cmdline"`
}

type statusReport struct {
	Processes []procInfo          `json:"processes"`
	Listening []string            `json:"listening"`
	Mounts    []string            `json:"mounts"`
	Block     []string            `json:"block_devices"`
	Kernel    map[string]string   `json:"kernel"`
	Modules   []string            `json:"modules"`
	Extra     map[string][]string `json:"extra,omitempty"`
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(b))
}

func hexIP(h string) string {
	b, _ := hex.DecodeString(h)
	for i := 0; i+4 <= len(b); i += 4 { // /proc/net 은 4바이트씩 뒤집혀 있다
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	return net.IP(b).String()
}

func sockets() []string {
	var out []string
	for _, f := range []string{"tcp", "tcp6", "udp", "udp6", "raw", "raw6"} {
		b, err := os.ReadFile("/proc/net/" + f)
		if err != nil {
			continue
		}
		for i, ln := range strings.Split(string(b), "\n") {
			fs := strings.Fields(ln)
			if i == 0 || len(fs) < 4 {
				continue
			}
			if strings.HasPrefix(f, "tcp") && fs[3] != "0A" { // 0A = LISTEN
				continue
			}
			hp := strings.Split(fs[1], ":")
			port, _ := strconv.ParseInt(hp[1], 16, 32)
			out = append(out, fmt.Sprintf("%s %s:%d", f, hexIP(hp[0]), port))
		}
	}
	sort.Strings(out)
	return out
}

func snapshot() (string, error) {
	r := statusReport{Kernel: map[string]string{}}
	ents, _ := filepath.Glob("/proc/[0-9]*")
	for _, e := range ents {
		pid, err := strconv.Atoi(filepath.Base(e))
		if err != nil {
			continue
		}
		cl, _ := os.ReadFile(e + "/cmdline")
		if len(cl) == 0 {
			continue // 커널 스레드
		}
		exe, _ := os.Readlink(e + "/exe")
		st := readTrim(e + "/stat")
		ppid := 0
		if i := strings.LastIndex(st, ")"); i > 0 {
			if f := strings.Fields(st[i+1:]); len(f) > 1 {
				ppid, _ = strconv.Atoi(f[1])
			}
		}
		r.Processes = append(r.Processes, procInfo{pid, ppid, exe, strings.TrimSpace(strings.ReplaceAll(string(cl), "\x00", " "))})
	}
	sort.Slice(r.Processes, func(i, j int) bool { return r.Processes[i].PID < r.Processes[j].PID })
	r.Listening = sockets()
	for _, ln := range strings.Split(readTrim("/proc/mounts"), "\n") {
		if f := strings.Fields(ln); len(f) >= 3 {
			r.Mounts = append(r.Mounts, f[1]+" "+f[2]+" "+f[0])
		}
	}
	bl, _ := os.ReadDir("/sys/block")
	for _, b := range bl {
		r.Block = append(r.Block, b.Name())
	}
	for _, k := range []string{"modules_disabled", "kexec_load_disabled", "sysrq"} {
		r.Kernel[k] = readTrim("/proc/sys/kernel/" + k)
	}
	r.Kernel["ptrace_scope"] = readTrim("/proc/sys/kernel/yama/ptrace_scope")
	r.Kernel["cmdline"] = readTrim("/proc/cmdline")
	r.Kernel["lockdown"] = readTrim("/sys/kernel/security/lockdown")
	for _, ln := range strings.Split(readTrim("/proc/modules"), "\n") {
		if f := strings.Fields(ln); len(f) > 0 {
			r.Modules = append(r.Modules, f[0])
		}
	}
	sort.Strings(r.Modules)
	b, err := json.MarshalIndent(r, "", " ")
	return string(b), err
}
