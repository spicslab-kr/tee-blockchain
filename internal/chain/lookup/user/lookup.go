// Package user reads registrations from the chain. It only reads.
//
// The auditor, vendor and user roles each have an identical copy of this file. Registration.Values and
// Reader.FindAddress use this role's tee/parse/user.Measurements. The other roles' Measurements have the same fields
// (Name, Value) but live in different packages, so Go treats them as different types and one interface
// cannot be shared by the three roles. Meta has no role-specific type and could have been shared, but it sits more
// naturally in the same file as Registration and Reader, so it was left here.
//
// The implementation (ether/reader.go) is per role for the same reason. The user role does not register, so there is
// no chain/register/user; the reason ether/reader.go borrows the auditor's contract binding is in the comment of that file.
package user

import (
	"context"

	"tb/internal/tee/parse/user"
)

type Meta struct {
	Service  string
	Version  string
	Platform string
	Arch     string // CPU architecture the image was built for (amd64 | arm64); informational, not part of the key
}

type Registration struct {
	Address   string
	Values    user.Measurements
	Meta      Meta
	Expires   int64 // by the chain's clock
	ChainTime int64 // unix time of the block that served the lookup; expiry is judged against this, not the verifier's clock
	Signer    string
	Signature []byte
}

type Reader interface {
	FindAddress(ctx context.Context, values user.Measurements) ([]string, error)
	Lookup(ctx context.Context, address string) (Registration, error)
}
