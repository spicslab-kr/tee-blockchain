# Command-line tools

Four binaries: one per role, plus the responder that runs inside the enclave.

| binary | role | runs on |
|---|---|---|
| `att-vendor` | Vendor: build or pack the enclave image, launch it, send the build config, self-register | a Nitro-enabled EC2 instance (Linux) for `build`, `pack`, `launch`; anywhere for the rest |
| `att-auditor` | Auditor: receive the source, rebuild, launch, deploy a registry, audit, send the result back | a Nitro-enabled EC2 instance (Linux) for `build`, `pack`, `launch`; anywhere for the rest |
| `att-user` | User: `verify`, plus the single steps it wraps (`ra-request`, `ra-parse`, `lookup`, `compare`) | anywhere with network access. No Nitro, no Docker, no keys |
| `att-ra-responder` | answers attestation requests inside the enclave | inside the enclave only. `build` and `pack` layer it into the image |

Each role binary contains only its own role's commands. `att-user` has no `register` at all.

Per-role guides: [Vendor](vendor/README.md), [Auditor](auditor/README.md), [User](user/README.md).
The chain side is described in the [registry contract guide](../contracts/README.md).

## Build the binaries

```
go build -trimpath -o att-vendor       ./cmd/vendor
go build -trimpath -o att-auditor      ./cmd/auditor
go build -trimpath -o att-user         ./cmd/user
go build -trimpath -o att-ra-responder ./cmd/ra-responder
```

Cross-compile for the Nitro instances (Linux) with `GOOS=linux GOARCH=amd64` (or `arm64` for a
Graviton instance).

**Pinned toolchain.** The required Go toolchain is exactly the version in
`internal/setup/build/GO_TOOLCHAIN` (currently go1.26.1). A different patch version produces
different `att-ra-responder` bytes and therefore different measurements, so `build` and `pack`
refuse to run under any other toolchain. `att-vendor version` and `att-auditor version` print the
pinned version and the one the binary was built with, and exit 1 if they differ.

**`-trimpath`.** It matters most for `att-ra-responder`. That file is layered straight into the
enclave image, so building identical source from a different folder would otherwise leave a
different absolute path inside the binary, and the measurement (PCR) with it.

### Where each binary runs

- **Vendor's Nitro instance:** `att-vendor` and `att-ra-responder`, in the same directory.
- **Auditor's Nitro instance:** `att-auditor` and `att-ra-responder`, in the same directory.
- **User:** `att-user`, anywhere.

### One shared `att-ra-responder`

`att-ra-responder` answers attestation requests inside the enclave. `build` layers it onto the
software image, so it must sit next to the role binary (or be named with `--responder <file>`).
It is one binary for every role, and the Vendor and the Auditor must use the same file: if they
embed different responders their measurements can never match.

`build` and `pack` do not take the file on trust. They read the build info Go leaves inside every
binary and refuse a responder that was built with a different Go than the pin, without
`-trimpath`, or for another OS or architecture than the instance they run on. The build log prints
the responder's sha256 so both sides can compare it by eye.

`build`, `pack`, `launch` and the responder only work on a Nitro-enabled EC2 instance (Linux).
Everything else runs anywhere.

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
