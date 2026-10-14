// Package vendor reads registrations from the chain.
//
// The auditor, vendor and user roles each have an identical copy of this file. Each copy uses its own role's
// tee/parse/vendor.Measurements, which Go treats as a different type from the other roles' Measurements even though the
// fields match. The reader in ether/reader.go is also per role, so a logic change goes into every copy.
package vendor

import (
	"context"

	"attestamp/internal/tee/parse/Vendor"
)

// Meta is the descriptive part of a registration, read from the Registered event.
type Meta struct {
	Service  string
	Version  string
	Platform string
	Arch     string // CPU architecture the image was built for (amd64 | arm64); informational and not part of the record key
}

// Registration is a registration as stored in the registry (approver, expiry) plus its event data.
// Values holds the single measurement hash the contract stores.
type Registration struct {
	RecordKey string
	Values    vendor.Measurements
	Meta      Meta
	Expires   int64 // unix seconds; the registration counts until the chain's time reaches this
	ChainTime int64 // unix time of the block that served the lookup; expiry is judged against this value
	Approver  string
	Signature []byte
}

// Reader looks up registrations in one registry.
type Reader interface {
	// FindRecordKeys returns the record keys registered for the measurements, empty if none.
	FindRecordKeys(ctx context.Context, values vendor.Measurements) ([]string, error)
	// Lookup returns the registration stored under a record key.
	Lookup(ctx context.Context, recordKey string) (Registration, error)
}
