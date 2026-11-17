# Command-line tools

Four binaries: one per role, plus the responder that runs inside the TEE.

| binary | role | runs on |
|---|---|---|
| `att-vendor` | Vendor: build (or pack) the measured image, launch it, send the build config, self-register | for `build`, `pack`, `launch`: a Nitro-enabled EC2 instance (Linux), or the TDX or SEV-SNP guest itself (`--platform intel-tdx \| amd-sev-snp-*`); anywhere for the rest |
| `att-auditor` | Auditor: receive the source, rebuild, launch, deploy a registry, audit, send the result back | the same as `att-vendor` |
| `att-user` | User: `verify`, plus the single steps it wraps (`ra-request`, `ra-parse`, `lookup`, `compare`) | anywhere with network access. No TEE, no Docker, no keys |
| `att-ra-responder` | answers attestation requests from inside the TEE | Nitro: inside the enclave only, layered into the image by `build` and `pack`, so it is measured. TDX and SEV-SNP: started on the guest (`ra-serve --extractor tdx \| snp-initrd \| snp-verity`), not measured (`docs/build/ra-comparison.md`) |

Each role binary contains only its own role's commands. `att-user` has no `register` at all.

Per-role guides: [Vendor](vendor/README.md), [Auditor](auditor/README.md), [User](user/README.md).
The chain side is described in the [registry contract guide](../contracts/README.md).

## Build the binaries

```
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-vendor       ./cmd/vendor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-auditor      ./cmd/auditor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-user         ./cmd/user
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-ra-responder ./cmd/ra-responder
```

Cross-compile for a Nitro instance or a TDX or SEV-SNP guest (Linux) with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` (or
`arm64` for a Graviton Nitro instance; the GCP guests are x86_64). Keep `CGO_ENABLED=0` on every build: on Linux with a C compiler installed Go otherwise links
cgo into `att-ra-responder`, the file differs from a cross-build, and `build` refuses it.

**Pinned toolchain.** The required Go toolchain is exactly the version in
`internal/setup/build/GO_TOOLCHAIN` (currently go1.26.1). A different patch version produces
different `att-ra-responder` bytes and therefore different measurements, so `build` and `pack`
refuse to run under any other toolchain. `att-vendor version` and `att-auditor version` print the
pinned version and the one the binary was built with, and exit 1 if they differ.

**`-trimpath`.** It matters most for `att-ra-responder`. That file is layered straight into the
enclave image, so building identical source from a different folder would otherwise leave a
different absolute path inside the binary, and the measurement (PCR) with it.

**`-buildvcs=false`.** By default Go also stamps the git commit, its time and whether the tree had
uncommitted changes into the binary, and `-trimpath` does not remove them. Identical source built
from two different commits, or from a tree with uncommitted changes, then gives different
`att-ra-responder` bytes (measured; see `docs/build/aws-nitro.md`).

**`-ldflags=-buildid=`.** By default Go writes a build ID into the binary, a hash of every input of the build. Any
change to a package `att-ra-responder` links then moves its bytes, even when no code it can run changed: one unused
function added to `internal/cli` did (`a9a45f2`). With an empty build ID it did not, and neither does a comment that
moves no line of code; code that moved to other lines still changes the bytes (measured 2026-11-17,
`docs/status/build-info-gaps.md`, Q1). Go does not record `-ldflags` in a `-trimpath` binary's build info, so `build`
reads the build ID from the binary itself (`go tool buildid <file>` prints it; empty is right).

### Where each binary runs

- **Vendor's Nitro instance or TDX/SEV-SNP guest:** `att-vendor` and `att-ra-responder`, in the same directory.
- **Auditor's Nitro instance or TDX/SEV-SNP guest:** `att-auditor` and `att-ra-responder`, in the same directory.
- **User:** `att-user`, anywhere.

### One shared `att-ra-responder`

`att-ra-responder` answers attestation requests inside the enclave. On Nitro `build` layers it onto the
software image (on TDX and SEV-SNP it runs from the guest's disk and this section does not apply), so it must sit next to the role binary (or be named with `--responder <file>`).
It is one binary for every role, and the Vendor and the Auditor must use the same file: if they
embed different responders their measurements can never match.

`build` and `pack` do not take the file on trust. They read the build info Go leaves inside every
binary and refuse a responder that was built with a different Go than the pin, without
`-trimpath`, with version control stamping (no `-buildvcs=false`), with cgo, with a Go build ID (no
`-ldflags=-buildid=`), or for another OS or architecture than the instance they run on. The build log prints
the responder's sha256 so both sides can compare it by eye.

`build`, `launch` and the responder work only on a Nitro-enabled EC2 instance (Linux) or on the TDX or SEV-SNP guest
itself (`--platform intel-tdx | amd-sev-snp-*`); `pack` is Nitro only. Everything else runs anywhere.

## Exit codes

0 is success and 2 is a missing or unknown command. A failing command otherwise exits with one of
the codes below. Codes 4 to 6 mean different things depending on where the error came from.

| code | meaning |
|---|---|
| 1 | any other error, including a failed `compare` and a `verify` that does not trust the enclave |
| 3 | network error |
| 4 | attestation in the wrong format; from `register`: the signing key's address is not the registry's approver |
| 5 | attestation signature or certificate chain invalid, or no document could be extracted; from `register`: already registered |
| 6 | nonce mismatch, or a bad or oversized response; from `register`: the expiry is already in the past |
| 7 | attestation document malformed |
| 8 | `git` not found (`send`, `receive`) |
| 9 | repository unreachable, URL or permissions (`send`, `receive`) |
| 10 | commit not found, see `--ref` (`send`, `receive`) |
| 11 | push rejected: no write access, or the remote is ahead, so pull first (`send`) |
| 12 | `audit --vendor-tee`: the Auditor's own value **was registered**, but the Vendor's TEE runs something else. Not a failed audit: do not run it again (a second run ends with 5, already registered). Since 2026-11-16; before, this case exited 0 |

`att-user` uses only 1 and 3 to 7. Codes 8 to 11 exist only in `att-vendor` and `att-auditor`.

## Code layout of cmd/

Each role is one `main` package. Files with the same name in different roles do the same job for that role.

| Files | Job |
|---|---|
| [vendor/main.go](vendor/main.go), [auditor/main.go](auditor/main.go), [user/main.go](user/main.go) | Command dispatch, usage text, exit codes. `ra-request`. The Vendor and Auditor ones also hold the enclave-side commands (`build`, `pack`, `launch`, `ra-serve`, `ra-proxy`, `lo-up`, `send`, `receive`). |
| [vendor/parse.go](vendor/parse.go), [auditor/parse.go](auditor/parse.go), [user/parse.go](user/parse.go) | `ra-parse`: verify an attestation document and extract the measurements. |
| [vendor/chain.go](vendor/chain.go), [auditor/chain.go](auditor/chain.go), [user/chain.go](user/chain.go) | Registry commands: `registry-deploy`, `register`, `lookup` (User: `lookup` only) and measurement file reading. |
| [vendor/compare.go](vendor/compare.go), [auditor/compare.go](auditor/compare.go), [user/compare.go](user/compare.go) | `compare`: two measurement sets, or measurements against a registration. |
| [vendor/self_register.go](vendor/self_register.go) | Vendor `self-register`: register the Vendor's own measurements. |
| [auditor/audit.go](auditor/audit.go) | Auditor `audit`: compare the rebuilt image with the Vendor's measurements and register. |
| [user/verify.go](user/verify.go) | User `verify`: request, verify, look up and decide in one command. |
| [vendor/arch.go](vendor/arch.go), [auditor/arch.go](auditor/arch.go) | CPU architecture handling (`amd64` or `arm64`) for `build`, `pack` and the build environment. |
| [vendor/quiet.go](vendor/quiet.go), [auditor/quiet.go](auditor/quiet.go), [user/quiet.go](user/quiet.go) | The `quiet` flag that silences single-step commands when a bundled command calls them. |
| [ra-responder/main.go](ra-responder/main.go) | `att-ra-responder`: the one binary layered into the enclave image; it only runs `lo-up` and `ra-serve`. |

The `*_e2e_test.go` files in this folder are end-to-end tests that drive the built binaries.
