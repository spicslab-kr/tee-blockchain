// Package vendor reads registrations from the chain. It only reads.
//
// The auditor, vendor and user roles each have an identical copy of this file. Registration.Values and
// Reader.FindRecordKeys use this role's tee/parse/vendor.Measurements. The other roles' Measurements have the same fields
// (Name, Value) but live in different packages, so Go treats them as different types and one interface
// cannot be shared by the three roles. Meta has no role-specific type and could have been shared, but it sits more
// naturally in the same file as Registration and Reader, so it was left here.
//
// The implementation (ether/reader.go) is per role for the same reason, so a logic change has to be made in all three places.
package vendor

import (
	"context"

	"attestamp/internal/tee/parse/Vendor"
)

type Meta struct {
	Service  string
	Version  string
	Platform string
	Arch     string // CPU architecture the image was built for (amd64 | arm64); informational, not in the record key
}

type Registration struct {
	RecordKey string
	Values    vendor.Measurements
	Meta      Meta
	Expires   int64 // by the chain's clock
	ChainTime int64 // unix time of the block that served the lookup; expiry is judged against this, not the verifier's clock
	Approver  string
	Signature []byte
}

type Reader interface {
	FindRecordKeys(ctx context.Context, values vendor.Measurements) ([]string, error)
	Lookup(ctx context.Context, recordKey string) (Registration, error)
}
