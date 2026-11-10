package vendor

import (
	"reflect"
	"testing"

	"attestamp/internal/tee/parse/snp"
)

// A record's names must be the ones the SEV-SNP parser returns, or a Vendor's record and a User's parse would name
// different measurements and hash differently on chain.
func TestMeasurementNames_MatchSNPParser(t *testing.T) {
	for _, f := range []string{snp.FormatInitrd, snp.FormatVerity} {
		if got, want := MeasurementNames[f], snp.MeasurementNames(f); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: transfer lists %v, the parser returns %v", f, got, want)
		}
	}
}
