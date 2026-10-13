# Contributing

This is a private, shared working repository. These are the rules we keep so the next person
can pick it up without asking.

## Build

The Go toolchain is pinned to exactly the version in `internal/setup/build/GO_TOOLCHAIN`
(currently go1.26.1). `build` refuses to run under any other version, because a different patch
version produces different `att-ra-responder` bytes and therefore different measurements.

```
go build -trimpath -o att-vendor       ./cmd/vendor
go build -trimpath -o att-auditor      ./cmd/auditor
go build -trimpath -o att-user         ./cmd/user
go build -trimpath -o att-ra-responder ./cmd/ra-responder
```

For the Nitro instances: `GOOS=linux GOARCH=amd64` (or `arm64`). Always `-trimpath`; the
responder is layered into the enclave image, so the build path would otherwise end up in the PCRs.

Running `go build ./cmd/<role>/...` without `-o` drops a binary named after the package into the
repo root; those names are in `.gitignore`, don't commit them.

## Test

```
go test ./...
```

Unit tests run anywhere. `cmd/*_e2e_test.go` build the binaries and run them against a local
`anvil` and a recorded attestation document; the chain-backed ones skip themselves when `anvil`
is not on `PATH`. No Nitro hardware is needed for any test.
Files the `e2e/` scripts fetch from instances land in `e2e-work/`, which is ignored.

## Layout

| path | what |
|---|---|
| `cmd/<role>` | one `main` per role: `vendor`, `auditor`, `user`, `ra-responder`. A role's binary only has that role's commands. |
| `internal/tee` | attestation request / parse, per cloud (`aws`, `azure`, `gcp` stubs) |
| `internal/setup` | `build` (image → EIF, normalized layers) and `launch` (enclave + vsock bridge) |
| `internal/chain` | `register` and `lookup` against `Registry.sol` |
| `internal/core` | `compare` (the trust decision) and `transfer` (hand-off records) |
| `contracts/evm` | the Registry contract; OpenZeppelin as a submodule |

Role-specific code sits in a sub-package named after the role (`.../Vendor`, `.../auditor`,
`.../user`). Keep it that way so a role's binary cannot link another role's commands.

## Commit messages

```
type(scope): what changed, in the imperative
```

- `type`: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`.
- `scope`: the package or area, e.g. `tee/parse`, `chain`, `cmd/user`, `e2e`, `contracts`.
- Test-only changes, including the e2e scripts, are `test(e2e)` / `test(cmd)`, not `fix`.
- One change per commit. The body says *why* when the diff does not.

## Pull requests

- Branch from `main`, open a PR, get one review from the other person.
- `go vet ./...` and `go test ./...` must pass. If you touched `build` or the responder, also
  run the measurement check once on an instance (`e2e/arch-measurement-compare.sh`) — the
  point of this project is that the bytes come out identical.
- If you changed a command's flags or output, update the README section for that role.

## Releases

Tags are `vMAJOR.MINOR.PATCH`. A release attaches the four Linux binaries for `amd64` and
`arm64`, built with the pinned toolchain and `-trimpath`, plus a `SHA256SUMS` file. The
`att-ra-responder` sha256 in a release is the one both Vendor and Auditor should see in their
build logs.
