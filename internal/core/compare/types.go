// Package compare holds the comparison and verdict logic shared by all roles. The per-role packages
// core/compare/<role> (auditor, Vendor, user) are thin delegates that convert their tee/parse.Measurements
// type into Items. The logic lives in this package (compare.go and check.go).
//
// Compare (compare.go) matches two sets of values by name. Check (check.go) is the trust decision for a
// registration: the measurement matches, the registration has not expired on the chain's clock, and the
// approver is in trustedApprovers.
package compare

// Item is one named measurement and Items is a set of them, mirroring the per-role Measurement and Measurements.
type Item struct {
	Name  string
	Value []byte
}

type Items []Item

func (items Items) Get(name string) []byte {
	for _, it := range items {
		if it.Name == name {
			return it.Value
		}
	}
	return nil
}

// Verdict is the result of a comparison. Pass is true exactly when Reason is empty.
type Verdict struct {
	Pass   bool
	Reason []string
}
