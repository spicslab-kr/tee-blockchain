package main

import tr "attestamp/internal/core/transfer/auditor"

// quiet is turned on when a command that bundles several steps (audit, verify) calls raRequest, raParse, register and lookup.
// Those functions then print nothing themselves, and the caller prints one line saying what it is doing.
var quiet bool

// lastRegistered: the registration that register last submitted. Kept so audit / self-register can print it in their own sentence.
var lastRegistered struct {
	RecordKey, Tx, Expires, Approver string
	BuildEnv                         tr.BuildEnv // the architecture and Go toolchain of the measurements; not put on the chain, only shown on screen and in measurement-record.json
	Block                            uint64
}
