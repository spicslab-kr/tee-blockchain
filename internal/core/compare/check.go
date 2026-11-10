package compare

import "bytes"

// RegInfo is the part of an on-chain registration that Check reads: the measurement hash, the expiry
// and the approver. It is filled in from chain/lookup's Registration.
type RegInfo struct {
	MeasurementHash []byte
	Expires         int64 // unix seconds, on the chain clock
	Approver        string
}

// Check is the trust decision for a registration. It is used wherever a registration is on one side: an
// Auditor or Vendor checking its own lookup right after registering, and the User's final check. It passes
// when the measurement hash matches, the registration has not expired at now (the chain's clock), and the
// approver is in trustedApprovers (normally the registry's AUDITOR and VENDOR addresses).
func Check(measurementHash []byte, reg RegInfo, now int64, trustedApprovers []string) Verdict {
	var reasons []string
	if reg.Expires <= now {
		reasons = append(reasons, "registration expired")
	}
	if !contains(trustedApprovers, reg.Approver) {
		reasons = append(reasons, "approver "+reg.Approver+" is not in the trusted approvers")
	}
	if !bytes.Equal(measurementHash, reg.MeasurementHash) {
		reasons = append(reasons, "measurement hash differs from the chain registration")
	}
	return Verdict{Pass: len(reasons) == 0, Reason: reasons}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if equalAddress(v, s) {
			return true
		}
	}
	return false
}

// equalAddress compares Ethereum addresses case-insensitively, since checksum casing varies.
func equalAddress(a, b string) bool {
	return toLower(a) == toLower(b)
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
