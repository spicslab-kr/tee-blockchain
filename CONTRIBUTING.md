# Contributing

This is a private, shared working repository. These are the rules we keep so the next person
can pick it up without asking.

## Build

The Go toolchain is pinned to exactly the version in `internal/setup/build/GO_TOOLCHAIN`
(currently go1.26.1). `build` refuses to run under any other version, because a different patch
version produces different `att-ra-responder` bytes and therefore different measurements.

```
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-vendor       ./cmd/vendor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-auditor      ./cmd/auditor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-user         ./cmd/user
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-ra-responder ./cmd/ra-responder
```

For the instances: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` (or `arm64`). Always `-trimpath -buildvcs=false`
and `CGO_ENABLED=0`; the responder is layered into the enclave image, so the build path, or the git commit and
its time, would otherwise end up in the PCRs, and without `CGO_ENABLED=0` a native Linux build with gcc
installed links cgo and gives other bytes than a cross-build. `build` refuses a responder built without any of
the three.

Running `go build ./cmd/<role>/...` without `-o` drops a binary named after the package into the
repo root; those names are in `.gitignore`, don't commit them.

## Toolchain

Every tool a build or a run calls, which step calls it, what to install on an empty machine, and the versions actually
used (printed with `--version`, never guessed) are in [docs/build/toolchain.md](docs/build/toolchain.md). In short:
Go at exactly `internal/setup/build/GO_TOOLCHAIN`, git, and anvil 1.8.0 for the chain tests; forge 1.8.0 (solc
0.8.28), Python 3 and the OpenZeppelin submodule only to regenerate the contract binding, which they reproduce byte
for byte.

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
| `contracts/evm` | the Registry contract (`Registry.sol`); OpenZeppelin as a submodule |

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
  run the measurement check once on an instance (`e2e/arch-measurement-compare.sh`): the
  point of this project is that the bytes come out identical.
- If you changed a command's flags or output, update the README section for that role.
- Unless you say otherwise, code you contribute is licensed as MIT and
  documentation as CC BY 4.0, the same as the rest of the project.

## Releases

Tags are `vMAJOR.MINOR.PATCH`. A release attaches the four Linux binaries for `amd64` and
`arm64`, built with the pinned toolchain and `-trimpath -buildvcs=false`, plus a `SHA256SUMS` file. The
`att-ra-responder` sha256 in a release is the one both Vendor and Auditor should see in their
build logs.
