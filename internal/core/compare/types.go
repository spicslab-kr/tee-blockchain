// Package compare holds the actual comparison and verdict logic, independent of role. The per-role packages
// core/compare/<role> (auditor, Vendor, user) are thin delegates that only convert their own
// tee/parse.Measurements type into the shape used here (Items). To change the logic, edit only this package
// (compare.go and check.go).
//
// compare.go is the comparison with no chain value (parsed vs build values, etc.). check.go is used when a chain
// registration is on either side; it checks expiry and the approver (trustedApprovers) as well as value equality.
package compare

// Item is one named measurement and Items is a set of them. The fields match the per-role Measurement/Measurements.
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

// Verdict is the result and its reasons. It is a pass if Reason is empty.
type Verdict struct {
	Pass   bool
	Reason []string
}
