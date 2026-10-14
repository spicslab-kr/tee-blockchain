package main

// quiet is turned on when a command that bundles several steps (verify) calls raRequest, raParse, register and lookup.
// Those functions then print nothing themselves, and the caller prints one line saying what it is doing.
var quiet bool
