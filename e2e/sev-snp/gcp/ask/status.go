package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 증명이 통과한 상태 보고서를 사람이 읽게 출력한다.
func printStatus(s string) {
	var r struct {
		Processes []struct {
			PID, PPID    int
			Exe, Cmdline string
		}
		Listening []string
		Mounts    []string
		Block     []string `json:"block_devices"`
		Kernel    map[string]string
		Modules   []string
	}
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		fmt.Println("  상태 보고서 읽기 실패:", err)
		return
	}
	fmt.Printf("\n[VM 안에서 돌고 있는 프로세스 %d개 (커널 스레드 제외)]\n", len(r.Processes))
	for _, p := range r.Processes {
		c := p.Cmdline
		if len(c) > 90 {
			c = c[:90] + "…"
		}
		fmt.Printf("  pid %-4d 부모 %-4d %s\n", p.PID, p.PPID, c)
	}
	fmt.Println("[연결을 기다리는 포트]")
	for _, l := range r.Listening {
		fmt.Println("  " + l)
	}
	fmt.Println("[붙어 있는 파일 시스템]")
	for _, m := range r.Mounts {
		fmt.Println("  " + m)
	}
	fmt.Printf("[디스크 장치] %s\n", strings.Join(r.Block, " "))
	fmt.Println("[커널 설정]")
	for _, k := range []string{"modules_disabled", "kexec_load_disabled", "sysrq", "ptrace_scope", "lockdown", "cmdline"} {
		fmt.Printf("  %-20s %s\n", k, r.Kernel[k])
	}
	fmt.Printf("[실려 있는 커널 모듈 %d개] %s\n", len(r.Modules), strings.Join(r.Modules, " "))
}
