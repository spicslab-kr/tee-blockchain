package compare

import "bytes"

// RegInfo is a role-independent summary of the on-chain registration that Check needs. It is filled in
// from chain/lookup's Registration (measurement hash, expiry, approver).
type RegInfo struct {
	MeasurementHash []byte
	Expires         int64 // unix seconds, on the chain clock
	Approver        string
}

// Check compares when one side is an on-chain registration. Such a comparison must look at expiry and the
// approving signature as well as the values. It is used whenever a chain value is on one side, whatever the
// role (an Auditor or Vendor checking its own lookup right after registering, or the User's final check).
// It passes only if the values match, the registration has not expired, and the approver is in trustedApprovers
// (normally the contract's AUDITOR and VENDOR addresses).
func Check(measurementHash []byte, reg RegInfo, now int64, trustedApprovers []string) Verdict {
	var reasons []string
	if reg.Expires <= now {
		reasons = append(reasons, "registration expired")
	}
	if !contains(trustedApprovers, reg.Approver) {
		reasons = append(reasons, "approver "+reg.Approver+" is not in the trusted list")
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

// equalAddress compares Ethereum addresses case-insensitively: they may differ only in case (checksum) and still be the same address.
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
