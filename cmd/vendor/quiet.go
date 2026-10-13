package main

import tr "tb/internal/core/transfer/Vendor"

// quiet is turned on when a command that bundles several steps (audit, verify) calls raRequest, raParse, register and lookup.
// Those functions then print nothing themselves, and the caller prints one line saying what it is doing.
var quiet bool

// lastRegistered: the registration that register last submitted. Kept so audit / self-register can print it in their own sentence.
var lastRegistered struct {
	Key, Tx, Expires, Signer string
	BuildEnv                 tr.BuildEnv // the architecture and Go toolchain of the measurements; not put on the chain, only shown on screen and in measurement-record.json
	Block                    uint64
}
