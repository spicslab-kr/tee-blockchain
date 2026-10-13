package compare

import "bytes"

// Compare is the comparison with no chain value involved. It matches a and b by name; a name present on only
// one side, or with different values, is recorded as a reason and the result is a mismatch. This covers parsed
// values vs build values, and parsed values vs a measurement record (or another TEE's values).
func Compare(a, b Items) Verdict {
	av := map[string][]byte{}
	for _, it := range a {
		av[it.Name] = it.Value
	}
	bv := map[string][]byte{}
	for _, it := range b {
		bv[it.Name] = it.Value
	}
	names := map[string]bool{}
	for n := range av {
		names[n] = true
	}
	for n := range bv {
		names[n] = true
	}
	var reasons []string
	for n := range names {
		va, okA := av[n]
		vb, okB := bv[n]
		switch {
		case !okA:
			reasons = append(reasons, n+": missing in a")
		case !okB:
			reasons = append(reasons, n+": missing in b")
		case !bytes.Equal(va, vb):
			reasons = append(reasons, n+": values differ")
		}
	}
	return Verdict{Pass: len(reasons) == 0, Reason: reasons}
}
