package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// imagePCRs — 이미지에서 나오는 것. PCR3·4·8 은 어디서 돌았느냐지 무엇이냐가 아니다.
var imagePCRs = []string{"PCR0", "PCR1", "PCR2"}

// measureEIF — EIF 파일을 nitro-cli 로 직접 잰다. Case A(이미지를 파일로 넘겨받는 흐름)에서 Auditor 가 Vendor 의
// measurements.json 을 믿는 대신 파일 자체를 재는 단계. Case B 에서는 build 가 잰 값과 같은 값이 나온다.
func measureEIF(ctx context.Context, r runner, eif string) (map[string]string, error) {
	out, err := r.output(ctx, "nitro-cli", "describe-eif", "--eif-path", eif)
	if err != nil {
		return nil, err
	}
	return readPCR(out)
}

// readPCR — nitro-cli 출력(build-enclave · describe-eif 모두 {"Measurements":{"PCR0":…}})에서 PCR0·1·2 를 꺼낸다.
func readPCR(b []byte) (map[string]string, error) {
	var v struct {
		Measurements map[string]string
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("measurements: %w", err)
	}
	pcr := map[string]string{}
	for _, k := range imagePCRs {
		if v.Measurements[k] == "" {
			return nil, fmt.Errorf("measurements: no %s", k)
		}
		pcr[k] = strings.ToLower(v.Measurements[k])
	}
	return pcr, nil
}

// writeMeasurements — 잰 값을 build 가 내는 measurements.json 과 같은 모양으로 쓴다. 이미 같은 값을 담은 파일이
// 있으면(Case B: build 가 쓴 것) 건드리지 않는다.
func writeMeasurements(path string, pcr map[string]string) (bool, error) {
	if b, err := os.ReadFile(path); err == nil {
		if have, err := readPCR(b); err == nil && samePCR(have, pcr) {
			return false, nil
		}
	}
	b, _ := json.MarshalIndent(struct {
		Measurements map[string]string
	}{pcr}, "", "  ")
	return true, os.WriteFile(path, append(b, '\n'), 0o644)
}

func samePCR(a, b map[string]string) bool {
	for _, k := range imagePCRs {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}

// checkAgainst — 파일을 잰 값 vs 보내온 measurements.json. 다르면 어느 PCR 인지 돌려준다.
func checkAgainst(measured map[string]string, claimedPath string) ([]string, error) {
	b, err := os.ReadFile(claimedPath)
	if err != nil {
		return nil, err
	}
	claimed, err := readPCR(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", claimedPath, err)
	}
	var reasons []string
	for _, k := range imagePCRs {
		if measured[k] != claimed[k] {
			reasons = append(reasons, fmt.Sprintf("%s: the EIF measures %s…, the file claims %s…", k, head(measured[k]), head(claimed[k])))
		}
	}
	return reasons, nil
}

func head(s string) string {
	if len(s) > 16 {
		return s[:16]
	}
	return s
}

var errMeasurementMismatch = errors.New("the EIF does not measure to what --measurements claims — not launching")
