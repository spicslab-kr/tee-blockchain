# Contributing

These are the rules we keep so that anyone can pick the work up without asking.

## Build

The Go toolchain is pinned to exactly the version in `internal/setup/build/GO_TOOLCHAIN`
(currently go1.26.1). `build` refuses to run under any other version, because a different patch
version produces different `att-ra-responder` bytes and therefore different measurements.

```
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-vendor       ./cmd/vendor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-auditor      ./cmd/auditor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-user         ./cmd/user
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-ra-responder ./cmd/ra-responder
```

For the instances: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` (or `arm64`). Always `-trimpath -buildvcs=false -ldflags=-buildid=`
and `CGO_ENABLED=0`; on Nitro the responder is layered into the enclave image, so the build path, or the git commit
and its time, would otherwise end up in the PCRs; without `-ldflags=-buildid=` Go's build ID, a hash of every input
of the build, moves the responder's bytes whenever any package it links changes, even code it never runs; and without
`CGO_ENABLED=0` a native Linux build with gcc installed links cgo and gives other bytes than a cross-build. `build`
refuses a responder built without any of the four.

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
`anvil` and recorded Nitro, TDX and SEV-SNP evidence; the chain-backed ones skip themselves when `anvil`
is neither on `PATH` nor in `~/.foundry/bin`. No TEE hardware is needed for any test.
Files the `e2e/` scripts fetch from instances land in `e2e-work/`, which is ignored.

`internal/doccheck` checks every document reference in the tracked files: Markdown links and their anchors, file
paths written in Markdown, Go comments and scripts, and every commit named in backticks (`1967749d`), which must be a
commit of this repository and unambiguous (it skips in a shallow clone). When you move or rename a file, it lists
every reference left pointing at the old place; if the history is ever rewritten, it lists every commit the documents
still name. Fix the reference; its few exceptions are not a place to silence a failure (the test file
says why).

## Layout

| path | what |
|---|---|
| `cmd/<role>` | one `main` per role: `vendor`, `auditor`, `user`, `ra-responder`. A role's binary only has that role's commands. |
| `internal/tee` | attestation request / parse, per platform (`aws`, `tdx`, `snp`; `azure`, `gcp` stubs) |
| `internal/setup` | `build` (Nitro: image → EIF, normalized layers; TDX/SEV-SNP: the application initrd and Option 2 image) and `launch` (Nitro: enclave + vsock bridge; TDX/SEV-SNP: GRUB entry, reboot, `--check`) |
| `internal/chain` | `register` and `lookup` against `Registry.sol` |
| `internal/core` | `compare` (the trust decision) and `transfer` (hand-off records) |
| `contracts/evm` | the Registry contract (`Registry.sol`); OpenZeppelin as a submodule |

Role-specific code sits in a sub-package named after the role (`.../Vendor`, `.../auditor`,
`.../user`). Keep it that way so a role's binary cannot link another role's commands.

## Language

Everything in the repository is written in English: code, comments, documents, commit messages and command output.

## The measurement hash

What goes on the chain is one hash over all of a platform's measurements:

```
measurementHash = keccak256( for each measurement, sorted by name: "<name>=<hex value>\n" )
```

It is computed in **four** places, and **all four must stay identical**, or a value registered by one binary is not
found by another:

| file | used by |
|---|---|
| `internal/chain/register/Vendor/register.go` | the Vendor's `register`, `self-register` |
| `internal/chain/register/auditor/register.go` | the Auditor's `register`, `audit` |
| `internal/core/compare/user/compare.go` | the User's comparison |
| `internal/chain/lookup/user/ether/reader.go` | the User's lookup (the User has no register package) |

Each has a `hash_golden_test.go` next to it with the same expected hashes for one real recording per format. Change all
four implementations and all four golden tests together, in one commit.

**Consequence: changing a measurement's name, or how many measurements a format has, changes the hash on the chain**,
so registrations made before no longer match. That is why one format string always has a fixed list of values. SEV-SNP
Option 1 registers five values and Option 2 six (PCR12 added), so they are two format strings,
`amd-sev-snp-initrd` and `amd-sev-snp-verity`, not one format with a variable count.

## Adding a platform

A platform is identified by a format string, carried in `RADoc.Format`; one platform may have more than one (SEV-SNP
has `amd-sev-snp-initrd` and `amd-sev-snp-verity`). The wire protocol sends it as a length-prefixed string, so a new
platform needs no protocol change. `contracts/evm/Registry.sol` does not change either: `recordKey =
keccak256(approver, platform, measurement)`, so each format string is already its own namespace on the chain. Below,
`<plat>` is the folder name (`aws`, `tdx`, `snp`) and `<format>` a format string. The files to touch:

| # | path | what |
|---|---|---|
| 1 | `internal/tee/parse/<plat>/` | the real parser: document, verification, nonce, measurements, embedded root certificate, and a constant per format string (`Format`; SEV-SNP has `FormatInitrd` and `FormatVerity`) |
| 2 | `internal/tee/parse/{Vendor,auditor,user}/<plat>.go` | thin type-converting adapters, one per role |
| 3 | `internal/tee/request/<plat>/` | getting the evidence inside the TEE, once for all roles |
| 4 | `internal/tee/request/{Vendor,auditor}/<plat>/` | the role's `Extractor`, a thin adapter over 3 |
| 5 | `cmd/ra-responder/main.go` | one more `--extractor` case |
| 6 | `cmd/{vendor,auditor,user}/parse.go` | one more `case` in the parser switch |
| 7 | `internal/core/transfer/{Vendor,auditor}/build_config.go` | `TEEInfoKeys["<format>"]`, one entry per format string |
| 8 | `internal/core/transfer/{Vendor,auditor}/measurement_record.go` | `MeasurementNames["<format>"]` per format string, and the format in a `case` of `Validate` |
| 9 | `internal/setup/build/<plat>/` and `internal/setup/build/{Vendor,auditor}/<plat>/` | producing the measured image: the shared code once, a thin adapter per role |
| 10 | `internal/setup/launch/<plat>/` and `internal/setup/launch/{Vendor,auditor}/<plat>/` | starting it and measuring it, in the same two layers |
| 11 | `cmd/{vendor,auditor}/<plat>.go`, and the `--platform` cases in `buildCmd` and `launchCmd` of `cmd/{vendor,auditor}/main.go` | the `build` and `launch` commands for the platform |
| 12 | the four `hash_golden_test.go` (below) | the expected hash of one real recording per new format string, the same in all four |
| 13 | `.gitignore` | an exception line for the embedded root certificate (below) |
| 14 | `docs/build/<guide>.md` (today `aws-nitro.md`, `tdx.md`, `sev-snp.md`) | what changes the measurements on this platform |

Follow the two-layer shape the existing platforms use: the role-independent logic once, next to the role folders
(`internal/tee/parse/aws` holds the real Nitro parser, `internal/setup/build/tdx` the TDX build), and per role only a
thin adapter. Record one real attestation
document, commit it under `testdata/`, and write the parser against it, so that parse, adapters, CLI wiring and the
trust decision can be tested with no instance running. What a parser must drop and check is in `STRUCTURE.md`,
"What every platform's parser must do".

## Secrets and certificates

Never commit a secret: no private keys, no real instance addresses, no instance IDs, no account numbers. Instance
addresses go in `e2e/env.local.sh`, which is ignored. `.gitignore` blocks every `*.pem` and `*.key`, with one exception
line per embedded platform root certificate (`internal/tee/parse/{aws,tdx,snp}/root.pem`). **A new platform's root
certificate needs its own exception line**, or it is silently left out of the commit.

## Commit messages

```
type(scope): what changed, in the imperative
```

- `type`: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`.
- `scope`: the package or area, e.g. `tee/parse`, `chain`, `cmd/user`, `e2e`, `contracts`.
- Test-only changes, including the e2e scripts, are `test(e2e)` / `test(cmd)`, not `fix`.
- One change per commit. The body says *why* when the diff does not.

## Pull requests

- Branch from `main`, open a PR, and get one review from a maintainer other than the author.
- `go vet ./...` and `go test ./...` must pass. If you touched `build` or the responder, also
  run the measurement check once on an instance (`e2e/arch-measurement-compare.sh`): the
  point of this project is that the bytes come out identical. That script is Nitro only, where the responder is
  inside the measured image. On TDX and SEV-SNP the responder is not measured, and what `build --platform` writes is
  pinned by the golden files of `internal/setup/build/cpio` and the tests of `internal/setup/build/{tdx,snp}`, which
  `go test ./...` runs. If you changed what `build --platform` writes, run flow 1 on two guests,
  `e2e/cvm/0-Setup-guests.sh` and then `e2e/cvm/flow1-auditor-registers/` 1 and 2 (e2e/README.md, "Flow 1 on TDX and
  SEV-SNP"): step 1 builds with `--verify` on the Auditor's guest, and step 2 requires the Vendor's guest to attest the
  Auditor's values. They have run on TDX guests, Option 1 (`docs/results/cvm-flow-rehearsal-2026-11-18.md`); for Option 2
  and SEV-SNP, where they have not run yet, "The Auditor's half" of `docs/build/tdx.md` and `docs/build/sev-snp.md` is the
  procedure that has.
- If you changed a command's flags or output, update the README section for that role.
- Unless you say otherwise, code you contribute is licensed as MIT and
  documentation as CC BY 4.0, the same as the rest of the project.

## Releases

There are no releases yet. When there are: tags are `vMAJOR.MINOR.PATCH`, and a release attaches the four Linux binaries for `amd64` and
`arm64`, built with the pinned toolchain, `CGO_ENABLED=0` and `-trimpath -buildvcs=false -ldflags=-buildid=`, plus a `SHA256SUMS` file. The
`att-ra-responder` sha256 in a release is the one both Vendor and Auditor should see in their
build logs.
