// Package doccheck has no code: its test checks that every document reference in the repository still points at
// something. This repository's documents carry its evidence (links between guides, results and status pages, and
// file paths in prose, Go comments and scripts), so a reference that stops resolving is a defect, found by
// `go test ./...` with no network. See references_test.go for what is checked and the few deliberate exceptions.
package doccheck
