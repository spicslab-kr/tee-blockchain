package snp

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// What in a GCP SEV-SNP report could tell one VM from another, read from the eight reports recorded on one VM
// (attestamp-snp-vendor, 2026-11-10 and 2026-11-11, initrdless, distribution initrd, and six boots with an
// application). This is half of why test-matrix row T10 (another VM's vTPM evidence bound by this VM's report) goes
// unnoticed: docs/build/sev-snp.md, "Same VM".
//
//   - MEASUREMENT is the same on every boot, with or without an initrd, with or without an application: it identifies
//     the firmware GCP launched, which every VM of the same machine type shares, not the VM.
//   - FAMILY_ID, IMAGE_ID, HOST_DATA, ID_KEY_DIGEST and AUTHOR_KEY_DIGEST are all zero: GCP sets no launch identity
//     a verifier could check.
//   - REPORT_ID changes on every boot of the same VM, and CHIP_ID changes when the VM lands on another host. Both
//     exist, but neither stays with a VM, and neither appears in the vTPM evidence, so there is nothing to compare
//     them with.
//
// Offsets are those of the SNP attestation report (AMD SEV-SNP ABI, report version 2 and later; these are version 5).
var snpReportCells = []string{"", "initrdless-", "opt1-v1-", "opt1-v2-", "opt1p-v1-", "opt1p-v2-", "opt2-v1-", "opt2-v2-"}

func reportField(r []byte, off, n int) []byte { return r[off : off+n] }

func TestReports_IdentifyFirmwareNotVM(t *testing.T) {
	const (
		familyID     = 0x10
		imageID      = 0x20
		measurement  = 0x90
		hostData     = 0xC0
		idKeyDigest  = 0xE0
		authorDigest = 0x110
		reportID     = 0x140
		chipID       = 0x1A0
	)
	var firstMeasurement []byte
	reportIDs := map[string]string{}
	chipIDs := map[string]bool{}
	for _, cell := range snpReportCells {
		name := "gcp-n2d-" + cell + "report.bin"
		r, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if len(r) != 1184 {
			t.Fatalf("%s: %d bytes, want 1184", name, len(r))
		}
		m := reportField(r, measurement, 48)
		if firstMeasurement == nil {
			firstMeasurement = m
		} else if !bytes.Equal(m, firstMeasurement) {
			t.Errorf("%s: MEASUREMENT %x differs from the other boots' %x", name, m, firstMeasurement)
		}
		for _, f := range []struct {
			field string
			off   int
			n     int
		}{{"FAMILY_ID", familyID, 16}, {"IMAGE_ID", imageID, 16}, {"HOST_DATA", hostData, 32}, {"ID_KEY_DIGEST", idKeyDigest, 48}, {"AUTHOR_KEY_DIGEST", authorDigest, 48}} {
			if v := reportField(r, f.off, f.n); !bytes.Equal(v, make([]byte, f.n)) {
				t.Errorf("%s: %s is %x, expected all zero on GCP", name, f.field, v)
			}
		}
		id := hex.EncodeToString(reportField(r, reportID, 32))
		if other, dup := reportIDs[id]; dup {
			t.Errorf("%s and %s share REPORT_ID %s; expected a new one per boot", name, other, id)
		}
		reportIDs[id] = name
		chipIDs[hex.EncodeToString(reportField(r, chipID, 64))] = true
	}
	if len(chipIDs) < 2 {
		t.Errorf("one VM's recordings show %d CHIP_ID; expected it to change with the host", len(chipIDs))
	}
}
