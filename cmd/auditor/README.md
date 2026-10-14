# Auditor guide (`att-auditor`)

The Auditor rebuilds the Vendor's software, runs its own copy in an enclave, checks that the
attested value equals its own build, and only then registers that value on the chain. Binaries and
toolchain: [Build the binaries](../README.md#build-the-binaries). `att-auditor` and `att-ra-responder`
must sit in the same directory on the Auditor's Nitro instance, and the responder must be the same
file the Vendor uses.

## Receive the source

Simplest: `git clone` the repository and `git checkout <commit>` the Vendor named.

Or, if the Vendor used `send`, this clones the repository, reads `build-config.json`, and checks
that the commit it names is really in the history:

```
att-auditor receive --repo <git url> --into work/src
```

`--ref` picks a commit, branch or tag. `receive` also compares the Vendor's `build_env` with this
instance and refuses to go on (exit other than 0) if the OS, CPU architecture or Go version
differ: a build here would not reproduce the Vendor's measurements, and the mismatch would look
like a finding. Move to a matching instance, or pass `--ignore-env` to only warn. A Vendor that
built without `-trimpath` gets a warning only.

## Build and launch

This is the Vendor's [build and launch](../vendor/README.md#build-pack-and-launch) with
`att-auditor` in place of `att-vendor`; the flags are the same. Reproducibility is the point: the
same source, the same `att-ra-responder` and normalized layers give the PCR0, PCR1 and PCR2 the
Vendor got. If they differ, that is the finding, and nothing should be registered.

Source rebuilt by the Auditor:

```
att-auditor build --app-dir work/src/<path> --out-dir out
```

```
att-auditor launch --eif out/<name>.eif
```

Image handed over as a file: the Vendor sends the image and its measurements, and `launch` measures
the image itself and refuses it if it does not match.

```
att-auditor launch --eif <received.eif> --build-measurements <received measurements.json>
```

## Deploy the registry (once, first time only)

One registry has one approver: only registrations signed by that address are accepted, forever. The
deployer key only pays gas and can be anyone's. The command prints the registry address, which
everyone then uses:

```
att-auditor registry-deploy --rpc <url> --deployer-key-file <deployer.key> --approver <my 0x...>
```

A registry deployed before registrations carried the CPU architecture (`arch`) does not accept the
current `register`; deploy a new one. See the [registry guide](../../contracts/README.md).

## Audit

```
att-auditor audit --config work/src/build-config.json --tee-addr <my-instance>:5000 \
    --build-measurements out/measurements.json --rpc <url> --registry <0x...> --signing-key-file <key>
```

`--tee-addr` is **your own** enclave, running **your own** build of the Vendor's source. What goes
on-chain is what you built from the source and ran yourself, checked against your build value, and
not what the Vendor told you.

`--config` is the Vendor's `build-config.json`; `receive` left it in the clone root. Service,
version and build environment (`build_env`) come from it, so they are not typed again. `--service`,
`--version` and `--arch` still work and override the file, with a warning when they differ.
Without `--config`, `--service` and `--version` are needed (or are taken from an existing
registration of the same value) and `--arch` defaults to this machine's.

Further flags: `--vendor-tee <host:port>` (also ask the Vendor's running enclave and report whether
it runs the same value; what is registered is still your value), `--expires` (default 2160h, that
is 90 days), `--platform` (default `aws-nitro`), `--format`, `--timeout`, `--nonce`, `--trusted-approvers`,
`--out-dir`. Without `--tee-addr`, `audit` registers `--build-measurements` as is, with no audit. Values you leave
out are asked for interactively; an empty `--rpc` asks for `y` to use Ethereum mainnet, and any
other text is taken as the RPC URL.

### The steps it wraps

Attestation document from your enclave:

```
att-auditor ra-request --tee-addr <my-instance>:5000 --out-file doc.cbor
```

Verify it and extract PCR0, PCR1, PCR2:

```
att-auditor ra-parse --doc doc.cbor --out-file parsed.json
```

The audit gate: the enclave's attested value must equal your build value. FAIL means the enclave is
not running what you built, and nothing gets registered:

```
att-auditor compare --attested parsed.json --expected out/measurements.json
```

Register the **build value** with your signing key:

```
att-auditor register --rpc <url> --registry <0x...> --signing-key-file <key> \
    --measurements out/measurements.json --config work/src/build-config.json
```

Read it back by measurement. The chain says which architecture the registration is for
(`service nginx 1.31.6 (aws-nitro, amd64)`). `arch` is in the signed registration but not in the record key, so
nobody needs to know it to look the value up:

```
att-auditor lookup --rpc <url> --registry <0x...> --measurements out/measurements.json
```

Self-check against the chain:

```
att-auditor compare --attested parsed.json --rpc <url> --registry <0x...> --record-key <record key register printed>
```

Pointing `--tee-addr` at the **Vendor's** enclave is the "something looks off" check from flow 2: attest
what the Vendor is really running and compare it to your build. `--vendor-tee` does that as an
extra step, but registration always comes from your own enclave.

## Send the result back to the Vendor

`measurement-record.json` carries the service and version, the PCRs, the build tool version and
the `build_env` the image was built in (OS, CPU architecture as `amd64` or `arm64`, Go toolchain,
`-trimpath`), the same shape as in `build-config.json`. Everything except the PCRs and the build
tool is already in the Vendor's `build-config.json`, and the PCRs are in `out/measurements.json`,
so `send` builds the record from those two, fills in the image's sha256 and pushes the record and
the image to the repository:

```
att-auditor send --repo <git url> --config work/src/build-config.json \
    --measurements out/measurements.json --eif out/<name>.eif
```

The build tool is nitro-cli. Its version is read from `nitro-cli --version` on this instance, or
given with `--build-tool-version`. The record is also left next to the measurements
(`out/measurement-record.json`). `--workdir` uses an existing clone.

Or write the record by hand and send that:

```
att-auditor send --repo <git url> --record measurement-record.json --eif out/<name>.eif
```

The same source measures differently on x86_64 and arm64 (and under a different Go), so one
service and version has one record, and one registration, per CPU architecture. `send` refuses a
record that does not say which, and warns when the record's Go is not the pinned one. Exit codes
8 to 11 are described in [Exit codes](../README.md#exit-codes).

## Key types, fields and functions

A short map of the code behind `att-auditor`, for someone who wants to read or change it. It lists the pieces worth knowing, not every identifier.

### Command dispatch

`main` in [main.go](main.go) switches on the first argument, runs the function for that command and turns its error into an exit code with `exitCode`.

| Name | Where | What it is or does |
|---|---|---|
| `main`, `usage` | [main.go](main.go) | Picks the command from `os.Args[1]`; prints usage and exits 2 when it is missing or unknown. |
| `receive` | [main.go](main.go) | Clones the source repository, reads `build-config.json`, runs the environment check. |
| `buildCmd` | [main.go](main.go) | `build`: builds the image and writes the EIF and measurements, via `Build.Run` in [build.go](../../internal/setup/build/auditor/aws/build.go). |
| `packCmd` | [main.go](main.go) | `pack`: the same from an image you already built. |
| `launchCmd` | [main.go](main.go) | `launch`: runs the EIF as an enclave via `Launch.Run` in [launch.go](../../internal/setup/launch/auditor/aws/launch.go); with `--build-measurements` it first measures the EIF ([measure.go](../../internal/setup/launch/auditor/aws/measure.go)) and refuses a mismatch. |
| `auditCmd` | [audit.go](audit.go) | `audit`: the whole audit flow, below. |
| `send` | [main.go](main.go) | `send`: builds the record with `buildRecord` (or reads `--record`), writes it with `writeRecordBeside`, then pushes with `GitTransport.Send`. |
| `registryDeploy` | [chain.go](chain.go) | `registry-deploy`: deploys the registry with its one approver. |
| `register` | [chain.go](chain.go) | `register`: signs the measurements and registers them; returns the record key. |
| `lookup` | [chain.go](chain.go) | `lookup`: reads by record key, or finds the record keys for given measurements first. |
| `compareCmd`, `checkMode` | [compare.go](compare.go) | `compare`: plain mode (`--expected`) or registry mode (`--record-key`); `checkMode` rejects mixed flags. |
| `raRequest` | [main.go](main.go) | `ra-request`: sends a nonce, saves the attestation document. |
| `raParse` | [parse.go](parse.go) | `ra-parse`: verifies the document and extracts the measurements. |
| `exitCode` | [main.go](main.go) | Maps error values to exit codes. |

### Audit flow

`auditCmd` in [audit.go](audit.go) runs these steps in order and stops at the first failure.

| Step | Where | What it does |
|---|---|---|
| Attest | `raRequest` in [main.go](main.go), then `raParse` in [parse.go](parse.go) | Asks your own enclave for an attestation document and verifies signature, certificate chain and nonce. |
| Compare to the build value | `Impl.Compare` in [compare.go](../../internal/core/compare/auditor/compare.go) | Checks the attested measurements equal `--build-measurements`; on a difference it stops and registers nothing. |
| Vendor check (optional) | same functions, on `--vendor-tee` | Reports whether the Vendor's running enclave has the same value; it never changes what is registered. |
| Register | `register` in [chain.go](chain.go) | Signs your value with your signing key and registers it; prints the record key, transaction, block and expiry. |
| Read back | `Reader.Lookup` in [reader.go](../../internal/chain/lookup/auditor/ether/reader.go) | Reads the registration from the chain under the record key. |
| Self-check | `Impl.Check` and `Clock` in [compare.go](../../internal/core/compare/auditor/compare.go) | Value, expiry (on the block's time) and approver must all pass; otherwise exit 1. |

### Environment check in receive

`receive` calls `CurrentBuildEnv` and `CheckBuildEnv` in [build_env_check.go](../../internal/core/transfer/auditor/build_env_check.go) to compare the Vendor's `build_env` with this instance.

| Field | Where | Behavior |
|---|---|---|
| `os`, `arch`, `go` | [build_env_check.go](../../internal/core/transfer/auditor/build_env_check.go) | Each must match this instance, or `receive` fails: a rebuild here could not reproduce the Vendor's measurements. |
| `trimpath` | [build_env_check.go](../../internal/core/transfer/auditor/build_env_check.go) | Never an error; `false` only gives a warning. |
| `--ignore-env` | [main.go](main.go) | Turns the `os`, `arch` and `go` errors into warnings. |

### measurement-record.json

`MeasurementRecord` in [measurement_record.go](../../internal/core/transfer/auditor/measurement_record.go); `Validate` checks it, with extra requirements on `aws-nitro`.

| Field | What it is |
|---|---|
| `schema` | Always `attestamp.measurement-record/1`. |
| `service`, `version` | The build this record is for. |
| `platform` | `aws-nitro` here; decides which values and image fields are required. |
| `values` | The measurements by name; on `aws-nitro`, `PCR0`, `PCR1` and `PCR2`. |
| `build_tool` | `name` (`nitro-cli`) and `version` of the tool that built the image. |
| `build_env` | `os`, `arch` (`amd64` or `arm64`), `go`, `trimpath`; required in full on `aws-nitro`. |
| `image` | `path` of the EIF and its `sha256` (64 hex characters); `send` fills the hash. |
| `registration` | Optional: `chain` and `address` of the registry it was registered on. |

`BuildConfig` in [build_config.go](../../internal/core/transfer/auditor/build_config.go) is what the Vendor hands over before the build: `schema`, `service`, `version`, `platform`, `source` (repo, commit, path), `tee_info` and `build_env`. It has no measurements, image or build tool. The record is what the Auditor sends back after the build: the same service, version, platform and `build_env`, plus the measured `values`, the `image` and the `build_tool`. `buildRecord` in [main.go](main.go) copies the first group from the config.

### Registration types

Used when you register ([register.go](../../internal/chain/register/auditor/register.go)) and when you read back ([lookup.go](../../internal/chain/lookup/auditor/lookup.go)).

| Name | Where | What it is |
|---|---|---|
| `Meta` (register) | [register.go](../../internal/chain/register/auditor/register.go) | What is signed besides the values: the lookup `Meta` plus `Expires`, after which the registration no longer counts. There is no revocation. |
| `Meta` (lookup) | [lookup.go](../../internal/chain/lookup/auditor/lookup.go) | `Platform` (for example `aws-nitro`), `Arch` (`amd64` or `arm64`, informational, not part of the record key), `Service`, `Version`. |
| `Signed` | [register.go](../../internal/chain/register/auditor/register.go) | A registration ready to send: `Values`, `Meta`, `Approver` (address of the signing key), `Digest` (the hash that was signed) and the 65-byte `Signature`. |
| `Receipt` | [register.go](../../internal/chain/register/auditor/register.go) | The result of `Register`: `RecordKey`, `TxHash`, `Block`. |
| `Registration` | [lookup.go](../../internal/chain/lookup/auditor/lookup.go) | What a lookup returns: `RecordKey`, `Values`, `Meta`, `Expires` and `ChainTime` (both on the chain's clock), `Approver`, `Signature`. |
| `MeasurementHash` | [register.go](../../internal/chain/register/auditor/register.go) | The single measurement hash stored on chain: keccak256 over `name=hex` lines in name order. |
| `Registrar`, `Reader` | [register.go](../../internal/chain/register/auditor/register.go), [lookup.go](../../internal/chain/lookup/auditor/lookup.go) | Interfaces for `Sign` and `Register`, and for `FindRecordKeys` and `Lookup`; the Ethereum versions are in the `ether` folders. |

The contract takes `platform`, `arch`, `service`, `version`, the measurement hash and `expires`, signed by the approver. The approver is the signing key's address and must be the one fixed in the registry at deploy time.

### Record key

The record key is `keccak256(abi.encode(approver, keccak256(platform), measurement hash))`, computed by `recordKeyOf` in [Registry.sol](../../contracts/evm/Registry.sol). It is not derived from service, version or arch. `Reader.FindRecordKeys` finds keys from measurements alone, so a User needs no one's word for which key to read.

### Exit codes

`exitCode` in [main.go](main.go) is the only place codes are assigned; the full table is in [Exit codes](../README.md#exit-codes). 0 is success, 2 a missing or unknown command, 1 any other error (including a failed `compare` or `audit` check) and 3 a network error. 4 to 7 come from the attestation, and from `register` where they mean not the approver (4), already registered (5) and expiry in the past (6). 8 to 11 come from the repository steps of `send` and `receive`.
