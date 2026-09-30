package main

import (
	"fmt"
	"runtime"
	"strings"

	"tb/internal/setup/build"
)

// archNames — 이 프로젝트가 빌드하는 아키텍처의 GOARCH 이름 (amd64 · arm64). build.ArchTargets 가 유일한 출처다.
func archNames() []string {
	out := make([]string, 0, len(build.ArchTargets))
	for _, t := range build.ArchTargets {
		out = append(out, t.GOARCH)
	}
	return out
}

// defaultArch — --arch 를 안 줬을 때: 이 바이너리가 도는 기계의 GOARCH. 측정값 파일을 다른 기계(인스턴스)에서
// 가져와 여기서 register 한다면 그 기계의 아키텍처를 --arch 로 직접 줘야 한다 — 같은 소스도 아키텍처마다 PCR 이 다르다.
func defaultArch() string { return runtime.GOARCH }

// checkArch — GOARCH 철자만 받는다. uname 이 내는 x86_64 · aarch64 는 여기서 걸린다.
func checkArch(s string) error {
	for _, n := range archNames() {
		if s == n {
			return nil
		}
	}
	return fmt.Errorf("--arch: want one of %s (GOARCH spelling: x86_64 is amd64, aarch64 is arm64), got %q", strings.Join(archNames(), "|"), s)
}
