# Project structure

This page lists every folder in the repository, then explains the one idea that shapes most of
`internal/`: **role packages** and **platform modules**. The [README](README.md) gives the overview;
[CONTRIBUTING](CONTRIBUTING.md) has the build and commit rules.

## Tree

```
.
├── go.mod, go.sum          Go module "attestamp"; the toolchain is pinned to go1.26.1
├── README.md               overview, install, quick start
├── CONTRIBUTING.md         build, test, layout and commit conventions
├── STRUCTURE.md            this file
├── .gitattributes          line-ending and diff rules
├── .gitmodules             pins OpenZeppelin under contracts/evm/lib
├── .gitignore              keys, e2e work files, stray build outputs
│
├── cmd/                    one folder per binary, plus the end-to-end Go tests
│   ├── vendor/             att-vendor:       build, launch, send, self-register
│   ├── auditor/            att-auditor:      receive, build, launch, audit, registry-deploy, send
│   ├── user/               att-user:         verify (no key, no register)
│   ├── ra-responder/       att-ra-responder: runs inside the enclave and answers attestation requests
│   └── *_e2e_test.go       build the real binaries and drive them against a local anvil chain
│
├── internal/               all library code; nothing here is importable from outside the module
│   ├── tee/                talking to the enclave and reading what it says
│   │   ├── request/        send a nonce, receive an attestation document
│   │   └── parse/          verify the document's signature and nonce, extract the measurements
│   ├── setup/              getting an enclave image built and running
│   │   ├── build/          reproducible image build, reproducibility scan, toolchain and responder checks
│   │   └── launch/         start the enclave, bridge the port, measure it
│   ├── chain/              the Ethereum side: register and look up measurements
│   │   ├── register/       sign a measurement (EIP-712) and write it to the registry
│   │   └── lookup/         read registrations back (read only)
│   ├── core/               logic that is about measurements, not about a platform or a chain
│   │   ├── compare/        compare values and judge a registration (match, expiry, approver)
│   │   └── transfer/       the build-config and measurement-record files that travel between Vendor and Auditor
│   └── cli/                the few things every command line shares (colour, progress lines)
│
├── contracts/
│   └── evm/                Registry.sol (Solidity) and the OpenZeppelin submodule under lib/
│
├── e2e/                    shell scripts for runs against real Nitro instances
│   ├── env.sh              shared settings (instance addresses come from an untracked env.local.sh)
│   ├── 0-Setup-*.sh        install or upload the binaries to the instances, and stop them again
│   ├── flow1-auditor-registers/       Auditor rebuilds and registers; tamper steps included
│   ├── flow2-vendor-self-registers/   Vendor registers itself; tamper steps included
│   └── *-check.sh, *-compare.sh       single-purpose checks (toolchain pin, build environment,
│                                      architecture, responder)
│
└── docs/
    └── img/                the diagrams used by the README (English and Korean SVGs)
```


### Inside `internal/<feature>/`

Every feature folder under `internal/` (`tee/request`, `tee/parse`, `setup/build`, `setup/launch`,
`chain/register`, `chain/lookup`, `core/compare`, `core/transfer`) has the same two kinds of
sub-package.

```
internal/tee/request/
├── Vendor/          role package  (package vendor)
│   ├── aws/         platform module: AWS Nitro, implemented
│   ├── gcp/         platform module: GCP Confidential Space, partial
│   └── azure/       platform module: Azure, empty
├── auditor/         role package  (package auditor), same shape
│   └── aws/ gcp/ azure/
└── user/            role package  (package user), no platform sub-folders
```

Not every feature has every role. The table under "Which binary uses what" shows who exists where.

## The module concept

### Role packages: vendor, auditor, user

A role package holds the code one role's binary needs, and the types that belong to that role. The
three roles are Vendor (ships the software), Auditor (rebuilds it and registers it) and User
(verifies it). A role package is named after the role in lowercase (`package vendor`), and
`cmd/<role>/main.go` imports only the matching role packages.

Known inconsistency to fix: on disk the Vendor folders are capitalised (`Vendor`) while `auditor`
and `user` are lowercase, and the Go package inside is lowercase `vendor` in all three cases. The
capitalised folders are these eight:

- `internal/tee/request/Vendor`
- `internal/tee/parse/Vendor`
- `internal/setup/build/Vendor`
- `internal/setup/launch/Vendor`
- `internal/chain/register/Vendor`
- `internal/chain/lookup/Vendor`
- `internal/core/compare/Vendor`
- `internal/core/transfer/Vendor`

Renaming them to `vendor` means changing the import paths in `cmd/vendor`, in the other role
packages that import them, and in the tests. On Windows and macOS (case-insensitive by default) the
difference is invisible, which is how it went unnoticed.

### Platform modules: aws, gcp, azure

A platform module is the code that is specific to one TEE platform. It lives one level below a role
package (`internal/tee/request/Vendor/aws`) or, for logic that does not depend on the role, next to
the role packages (`internal/tee/parse/aws`). A role package picks the module by the attestation
document's format name (`aws-nitro`, `gcp-cs-jwt`, `azure-maa-jwt`).

What is implemented, checked against the code:

| Platform | Implemented end to end | State in the tree |
| --- | --- | --- |
| aws (Nitro Enclaves) | Yes. | Request (NSM, vsock, proxy), document verification against the AWS Nitro root, measurement extraction, image build, enclave launch. |
| gcp (Confidential Space) | No. | `tee/request/<role>/gcp` can fetch a token from the launcher socket and has a test. `tee/parse/<role>/gcp.go` returns "not implemented" for every method. Nothing in `cmd/*` constructs either. There is no gcp folder under `setup/`. |
| azure (Confidential VM) | No. | `tee/request/<role>/azure` returns "not implemented" (it needs a vTPM library and MAA). `tee/parse/<role>/azure.go` returns "not implemented". Nothing in `cmd/*` constructs either. No azure folder under `setup/`. |

Only **aws** is a working platform. gcp and azure exist so the format names, the Parser interface
and the folder layout are already in place when someone adds them.

`ether` plays the same part for chains: `chain/register/<role>/ether` and `chain/lookup/<role>/ether`
are the only chain modules, and they talk to any Ethereum-compatible chain.

### The shared pieces

Two pieces of logic do not depend on the role and exist once:

- `internal/tee/parse/aws` holds the real Nitro parsing (CBOR decoding, certificate chain and
  signature check, PCR extraction, the embedded AWS root certificate). Each role's
  `tee/parse/<role>/aws.go` is a thin adapter that converts to and from that role's own types.
- `internal/core/compare/compare.go`, `check.go` and `types.go` hold the comparison and the trust
  decision. Each role's `core/compare/<role>` is a thin delegate that converts its own
  `Measurements` type into the shared `Items`.

`internal/setup/build` (root files: architecture, toolchain pin, responder build check) is shared by
all binaries that build; `internal/cli` is shared by every command.

### Why role code is copied per role

Each binary must contain only its own role's commands, and the User's binary has no `register` at
all. Go types are identified by package: `tee/parse/vendor.Measurements` and
`tee/parse/user.Measurements` are different types even when their fields match. So one shared
implementation cannot be imported by three roles without every binary linking every role's
commands and types. Instead, each role package is a copy (or a close copy) with its own types, and
the role-neutral logic is pulled out into the shared pieces above wherever Go allows it.

The copies were compared after renaming `Vendor`, `auditor` and `user` to one placeholder, so only
real differences show.

**Identical across roles (apart from the role name):**

| Folder | Identical between |
| --- | --- |
| `chain/register` (all of it, including the generated `registry.go`) | Vendor, auditor |
| `setup/build/<role>/aws` and `setup/launch/<role>/aws` | Vendor, auditor |
| `tee/request` (`request.go`, `nonce.go`, `tcp.go`, `wire.go`, `aws/*`, `gcp/*`, `azure/*`) | Vendor, auditor; the user copy of `request.go`, `nonce.go`, `tcp.go`, `wire.go` is the same too |
| `tee/parse` (`parse.go`, `aws.go`, `gcp.go`, `azure.go`) | Vendor, auditor, user |
| `chain/lookup` (`lookup.go`, `ether/reader.go`) | Vendor, auditor |
| `core/compare/<role>` | Vendor, auditor |
| `core/transfer`: `build_config.go`, `codec.go`, `digest.go`, `measurement_record.go`, `registration.go`, `git_common.go` | Vendor, auditor |

**Different, because the roles do different things:**

| File | How it differs |
| --- | --- |
| `core/transfer/<role>/transport.go` and `git.go` | Opposite directions. The Vendor sends the build config and receives the measurement record; the Auditor receives the build config (clone) and sends the measurement record. |
| `core/transfer/auditor/build_env_check.go` | Only the Auditor has it: the OS, CPU architecture and Go version check. |
| `chain/lookup/user` (`lookup.go`, `ether/reader.go`) | The User's reader differs from the Vendor and Auditor ones. |
| `core/compare/user/compare.go` | The User has no register package, so it recomputes the measurement hash itself. |
| Tests in `core/transfer`, `chain/lookup/*/ether`, `core/compare/user` | Follow the code above. |

The User has no `setup`, no `chain/register` and no `core/transfer`. It also has no platform
sub-folders, because it only needs to ask and verify.

Three tests sit directly in a feature folder because they cross roles:
`chain/register/ether_test.go`, `core/transfer/vendor_auditor_test.go` and
`tee/request/user_to_vendor_test.go`.

### Which binary uses what

Verified with `go list -deps` on each `cmd/*` package.

| Module | Job | att-vendor | att-auditor | att-user | att-ra-responder |
| --- | --- | :-: | :-: | :-: | :-: |
| `tee/request/<role>` and `<role>/aws` | Request an attestation document; the responder side runs inside the enclave | yes | yes | yes (role only) | yes (Vendor copy, with aws) |
| `tee/parse/aws` (shared) | Verify a Nitro document and extract measurements | yes | yes | yes | no |
| `tee/parse/<role>` | Role-typed adapter over the shared parser | yes | yes | yes | no |
| `setup/build` (root) | Architecture, toolchain pin, responder build check | yes | yes | no | no |
| `setup/build/<role>/aws` | Reproducible image build, scan, verify | yes | yes | no | no |
| `setup/launch/<role>/aws` | Start the enclave, bridge the port, measure | yes | yes | no | no |
| `core/transfer/<role>` | Build-config and measurement-record exchange, over git or files | yes | yes | no | no |
| `chain/register/<role>` and `/ether` | Sign and register a measurement; deploy the registry | yes | yes | no | no |
| `chain/lookup/<role>` and `/ether` | Read registrations | yes | yes | yes | no |
| `core/compare` (shared) and `core/compare/<role>` | Compare values, judge expiry and approver | yes | yes | yes | no |
| `cli` | Colour and progress output | yes | yes | yes | yes |

Two details the table hides:

- `att-user` does not link any role's commands, but it does link `chain/register/auditor/ether`
  (the generated Registry binding) and, through it, `chain/lookup/auditor`, `tee/parse/auditor` and
  `tee/request/auditor`. `chain/lookup/user/ether/reader.go` imports that binding directly.
- `att-ra-responder` is built from the Vendor request package only, and `build` layers it into the
  enclave image.

## Folder reference

| Path | What lives there |
| --- | --- |
| `cmd/<role>/` | `main.go` (flags and dispatch), plus `arch.go`, `chain.go`, `compare.go`, `parse.go`, `quiet.go` wiring for that role; vendor adds `self_register.go`, user adds `verify.go`, auditor adds `audit.go` |
| `internal/setup/build/GO_TOOLCHAIN` | The pinned Go version, read by `build` and by CONTRIBUTING |
| `internal/tee/parse/aws/` | Shared Nitro parser, `root.pem`, and a recorded document under `testdata/` |
| `internal/tee/request/testdata/` | A recorded Nitro attestation document used by tests |
| `internal/chain/*/<role>/ether/` | Go side of the registry contract; `registry.go` is generated by `go generate` from `contracts/evm/Registry.sol` |
| `contracts/evm/` | `Registry.sol`; `lib/openzeppelin-contracts` is a git submodule |
| `e2e/` | Shell scripts, not run by `go test`; the Go end-to-end tests are in `cmd/` |
| `docs/img/` | `roles.svg`, `verify.svg` and Korean variants (`-ko`) |
