package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// imagePCRs are the values that come from the image. PCR3, 4 and 8 say where it ran, not what it is.
var imagePCRs = []string{"PCR0", "PCR1", "PCR2"}

// measureEIF measures the EIF file directly with nitro-cli. When the image arrives as a file (pack), this is the step where the
// Auditor measures the file itself and compares the result with the Vendor's measurements.json. For an EIF built here, it gives the same values build measured.
func measureEIF(ctx context.Context, r runner, eif string) (map[string]string, error) {
	out, err := r.output(ctx, "nitro-cli", "describe-eif", "--eif-path", eif)
	if err != nil {
		return nil, err
	}
	return readPCR(out)
}

// readPCR extracts PCR0, PCR1 and PCR2 from nitro-cli output (both build-enclave and describe-eif give {"Measurements":{"PCR0":...}}).
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

// writeMeasurements writes the measured values in the same shape as the measurements.json that build produces. If a file already holding the same values
// exists (one build wrote), it is left alone.
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

// checkAgainst compares the values measured from the file with the received measurements.json. If they differ, it says which PCR.
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

var errMeasurementMismatch = errors.New("the EIF does not measure to what --build-measurements claims; not launching")
