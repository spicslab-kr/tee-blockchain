# Contributing

This is a private, shared working repository. These are the rules we keep so the next person
can pick it up without asking.

## Build

The Go toolchain is pinned to exactly the version in `internal/setup/build/GO_TOOLCHAIN`
(currently go1.26.1). `build` refuses to run under any other version, because a different patch
version produces different `att-ra-responder` bytes and therefore different measurements.

```
go build -trimpath -buildvcs=false -o att-vendor       ./cmd/vendor
go build -trimpath -buildvcs=false -o att-auditor      ./cmd/auditor
go build -trimpath -buildvcs=false -o att-user         ./cmd/user
go build -trimpath -buildvcs=false -o att-ra-responder ./cmd/ra-responder
```

For the Nitro instances: `GOOS=linux GOARCH=amd64` (or `arm64`). Always `-trimpath -buildvcs=false`;
the responder is layered into the enclave image, so the build path, or the git commit and its time,
would otherwise end up in the PCRs. `build` refuses a responder built without either flag.

Running `go build ./cmd/<role>/...` without `-o` drops a binary named after the package into the
repo root; those names are in `.gitignore`, don't commit them.

## Toolchain

| tool | version | used for | how it was fixed |
|---|---|---|---|
| Go | `internal/setup/build/GO_TOOLCHAIN` (go1.26.1) | everything | `build` refuses any other version |
| solc | 0.8.28 | compiling `contracts/evm/Registry.sol` | `--use 0.8.28` in `generate.go`; also in the bytecode's metadata (`64736f6c6343 00081c`) |
| forge (Foundry) | 1.8.0 | running solc for `generate.go` | see below |
| OpenZeppelin Contracts | v5.7.0, `cab19933c33c2ad1d4c7a84864a3601dddfd16f3` | imported by `Registry.sol` | the submodule |
| Python | 3 (3.14.6 checked) | `generate.go`'s second step: splits forge's JSON into `.abi` and `.bin`, standard library only (`json`, `io`) | see below |
| abigen | from go-ethereum in `go.mod` (v1.17.5) | the Go binding | `go run`, so the version is the module's |
| anvil (Foundry) | 1.8.0 | the chain-backed tests | `README.md`, "Testing" |

The forge that produced the committed `registry.go` was not recorded. Checked on 2026-11-12: forge 1.8.0
(`foundry_v1.8.0_win32_amd64.zip`, sha256 matching the release's `.sha256` file), with OpenZeppelin at the commit above
and Python 3.14.6, runs the three `go:generate` steps to a `registry.go` that is **byte-identical** to the committed
one in both `chain/register/auditor/ether` and `chain/register/Vendor/ether`, metadata hash included. So forge 1.8.0
is a version known to reproduce it. A different forge may pass different default settings to solc and change the
metadata hash at the end of the bytecode, and with it the deployed code.

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
