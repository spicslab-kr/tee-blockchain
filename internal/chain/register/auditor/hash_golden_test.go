package auditor

import (
	"encoding/hex"
	"testing"

	p "attestamp/internal/tee/parse/auditor"
)

// The measurement hash is computed in four places: chain/register/{Vendor,auditor}, core/compare/user and
// chain/lookup/user/ether. They must give the same 32 bytes, or a User looks up and compares against a value nobody
// registered. Each of the four has this same test with the same golden values: real measurements of one recording per
// format, and the hash computed independently with Foundry's `cast keccak` (1.8.0) over the "name=hex\n" lines in
// name order. A change to the hash has to change all four tests, and the on-chain records with them.
var hashGolden = []struct {
	format string
	values map[string]string
	want   string
}{
	{"aws-nitro", map[string]string{ // internal/tee/request/testdata/nitro-attestation.cbor
		"PCR0": "af229a81a0d7f55ff47d6b06bda729f6680bd42220ad31c4f5ecd952084dade9cab1f08b4e7f5962352f3c9d452514fc",
		"PCR1": "3b4a7e1b5f13c5a1000b3ed32ef8995ee13e9876329f9bc72650b918329ef9cf4e2e4d1e1e37375dab0ba56ba0974d03",
		"PCR2": "bd1cddbb73cd08a147ac41e19fcedca0b602d24b0432562a726ea4cdabb783e1ddaee8c0ff4000752460a68575f2580f",
	}, "3d73c2f1111621b5a883c824445a803348decfa1a8513ecf039da30ca78377a5"},
	{"intel-tdx", map[string]string{ // internal/tee/parse/tdx/testdata/gcp-c3-opt2m-v1-quote.bin
		"MRTD":  "f21e6b48743fe14c2d090078ca1197f09ae4c99438677f2c79901ef80e2aeefe2f9a7eb6e3eb3b5047c4f1511731e5d3",
		"RTMR0": "8847ec5d4441a3e41856f9de32c21676a788060c2746c565aee58b90a882fd427e9e45487dd992cd87eaf0b2fcdfee68",
		"RTMR1": "6e1a3f8856f3119c3036497985d35f79c1744ad141eed42a7fe75aea0de87ee9437d9c71e7a04bbc3af59d7d7842734a",
		"RTMR2": "fc0d75eb1f8f152bf73ac4a7a778d2802efbc675050016a0fbf77c5ab7f7efa2f8b2c4b4aa5ae7c100cd362e6c6a0fb9",
	}, "dc9a25c06864bb75bdfbdfce3c70365c2599b6a165dce27328525d0397a4cb5f"},
	{"amd-sev-snp-initrd", map[string]string{ // live evidence of the stock-boot SEV-SNP guest, 2026-11-12
		"MEASUREMENT": "f67900851e25a378f04b4d4b5f1ea4410c11deb74e97ddcc7e3363a33f6e1dbb7880d304005387bc98e9cbdceec4ea28",
		"PCR4":        "5b65d2d390d40a5dffb29675a90939615b25aaf164a8b493743242e6f5e84389",
		"KERNEL":      "3cf8c2c34cd30720d3f37c8b5327ae7d10f24073e73a34edb1e4aaea4dd39adb",
		"CMDLINE":     "80dfd7d3a1f7ba936075eb6666f5e72c865fa2c11295ba0f35cfbdb276cc3de0",
		"INITRD":      "4703ec07684196e8616d384ac585be54b6345ca8c6ea5544aab8445e0cd4b0a2",
	}, "2a571358a50ecb6176439b886332ad5c867701ec5ebfcd582c8de219e2609957"},
	{"amd-sev-snp-verity", map[string]string{ // internal/tee/parse/snp/testdata/gcp-n2d-opt2-v1-*
		"MEASUREMENT": "f67900851e25a378f04b4d4b5f1ea4410c11deb74e97ddcc7e3363a33f6e1dbb7880d304005387bc98e9cbdceec4ea28",
		"PCR4":        "5b65d2d390d40a5dffb29675a90939615b25aaf164a8b493743242e6f5e84389",
		"KERNEL":      "3cf8c2c34cd30720d3f37c8b5327ae7d10f24073e73a34edb1e4aaea4dd39adb",
		"CMDLINE":     "2c2f65293fceae9eed803d5fd46996ad8f1d20b945c252c266873d3a0b0b8b52",
		"INITRD":      "b0fd172dff60ce1ba307d0318ae92d49d2dad8296ae7de8c912e9ab79dabf33a",
		"PCR12":       "470379c79f539f0ec838951311fc92d43782ff3eb138cd6575188ca4b18a4689",
	}, "53477d041ced776650e2652aef4efd502138bf4e2b6bec088d220d67607b5331"},
}

func TestMeasurementHash_GoldenPerFormat(t *testing.T) {
	for _, c := range hashGolden {
		var ms p.Measurements
		for name, v := range c.values {
			b, err := hex.DecodeString(v)
			if err != nil {
				t.Fatal(err)
			}
			ms = append(ms, p.Measurement{Name: name, Value: b})
		}
		h, err := MeasurementHash(ms)
		if err != nil {
			t.Fatalf("%s: %v", c.format, err)
		}
		if got := hex.EncodeToString(h[:]); got != c.want {
			t.Errorf("%s: measurement hash %s, want %s", c.format, got, c.want)
		}
	}
}
