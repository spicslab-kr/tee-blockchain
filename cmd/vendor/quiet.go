package main

import tr "attestamp/internal/core/transfer/Vendor"

// quiet is turned on when a command that bundles several steps (self-register) calls raRequest, raParse, register and lookup.
// Those functions then print nothing themselves, and the caller prints one line saying what it is doing.
var quiet bool

// lastRegistered: the registration that register last submitted. Kept so self-register can print it in their own sentence.
var lastRegistered struct {
	RecordKey, Tx, Expires, Approver string
	BuildEnv                         tr.BuildEnv // architecture and Go toolchain of the measurements; shown on screen and belongs in measurement-record.json (the chain holds only the arch)
	Block                            uint64
}
