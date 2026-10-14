# Vendor guide (`att-vendor`)

The Vendor builds or packs the software into an enclave image, launches it, and either hands the
source to an Auditor or registers the measurement itself (flow 2). Binaries and toolchain:
[Build the binaries](../README.md#build-the-binaries). `att-vendor` and `att-ra-responder` must sit in
the same directory on the Nitro instance.

Both ways of building must end with the same three things, because that is all the rest of the
system uses:

| what | where it goes |
|---|---|
| the enclave image (`.eif`) | `launch`, and the hand-off |
| the measurements, nitro-cli's JSON (`{"Measurements":{"PCR0":...,"PCR1":...,"PCR2":...}}`) | `launch --build-measurements`, `register --measurements`, `compare --expected` |
| the address `host:port` the enclave answers on | `--tee-addr` for the Auditor and the User |

Building is optional: an image received from the Auditor can be launched directly (see
[Hand-off](#hand-off-with-the-auditor)).

## Build, pack and launch

### Image handed over as a file

You build the software image however you like, `pack` turns it into an enclave image, and you give
the Auditor the image and its measurements. The Auditor does not rebuild; it measures the file it
received.

```
docker build -t app:base <dir>
```

`pack` reads the image's original command, layers the responder and its entrypoint on top,
normalizes the layers, and writes `out/app.eif` and `out/measurements.json`:

```
att-vendor pack --docker-image app:base --out-dir out
```

Flags: `--docker-image` (required), `--image-name`, `--port` (default 5000), `--out-dir` (default `out`),
`--responder`.

`launch` starts the enclave, bridges TCP 5000 to the enclave's vsock, asks the new enclave for an
attestation to prove it answers (`out/selfcheck.cbor`), and opens the security-group port. Without
IAM permission on the instance it prints the one `aws ec2` line to run from your account.

```
att-vendor launch --eif out/app.eif
```

Flags: `--eif` (required), `--build-measurements`, `--enclave-name`, `--port` (default 5000, must match the
build), `--memory` (MiB, default sized from the image), `--cpus` (2), `--cid` (16), `--listen`
(0.0.0.0), `--out-dir`.

Hand `out/app.eif` and `out/measurements.json` to the Auditor by any means (scp, mail). Its
`launch --eif <file> --build-measurements <file>` measures the image itself and launches it only if the
file measures to what the JSON claims.

### Source rebuilt by the Auditor

You give the Auditor the repository and commit. Both of you run the same two commands on your own
Nitro instance and must get the same PCR0, PCR1 and PCR2. That comparison is the audit.

`build` installs docker and nitro-cli if they are missing, builds the image, layers the responder,
normalizes the layers, makes the image and writes the measurements: `out/<name>.eif` and
`out/measurements.json`.

```
att-vendor build --app-dir <dir with Dockerfile> --out-dir out
```

It first scans the Dockerfile for things known to break reproducibility (unpinned base image or
packages, `date` or `hostname`, random keys, compiling from source) and prints each with its line.
Add `--verify` to build a second time from scratch and fail unless the measurements come out
identical, listing the files that differ. It is worth doing the first time a source goes to an
Auditor.

```
att-vendor launch --eif out/<name>.eif
```

`build` takes a directory with a Dockerfile and runs `docker build` itself; `pack` takes an image
you already built. They share `--image-name`, `--port`, `--out-dir` and `--responder`.

## Self-register (flow 2)

In flow 1 the Auditor audits and registers, and the Vendor does not register. `self-register` is
for flow 2, where the Vendor registers its own enclave:

```
att-vendor self-register --config build-config.json --tee-addr 127.0.0.1:5000 \
    --rpc <url> --registry <0x...> --signing-key-file <key>
```

`--config` is the same `build-config.json` that `send` pushes: service, version and the build
environment (`build_env`: OS, CPU architecture, Go toolchain, `-trimpath`) are taken from it.
`--service`, `--version` and `--arch` still work and override the file, with a warning when they
differ (`warning: --service nginx2 differs from build-config.json (nginx)`). Without `--config`,
`--service` and `--version` are needed, or are taken from an existing registration of the same
value on that registry.

The confirmation names the CPU architecture and Go toolchain the enclave was built with
(`registered nginx 1.31.6 (amd64, go1.26.1 -trimpath)`). The architecture also goes on-chain in the
registration's `arch` field, so `lookup` prints it back. It is display only and not part of the record key.

Further flags: `--build-measurements <file>` (compare the attestation to your build value first and
refuse to register on a mismatch, printing the differing PCRs), `--expires` (default 2160h, that
is 90 days), `--platform` (default `aws-nitro`), `--format`, `--timeout`, `--nonce`, `--trusted-approvers`,
`--out-dir`.

Any value you leave out is asked for interactively. `--rpc` is special: when empty, it asks for
`y` to use Ethereum **mainnet**, and any other text is taken as the RPC URL (a testnet, for
example).

A registry for flow 2 is deployed once with:

```
att-vendor registry-deploy --rpc <url> --deployer-key-file <deployer.key> --approver <0x...>
```

See the [registry guide](../../contracts/README.md) for what the contract accepts.

### The steps it wraps

Send a fresh nonce to the enclave and get back the signed attestation document:

```
att-vendor ra-request --tee-addr 127.0.0.1:5000 --out-file doc.cbor
```

Verify the signature chain up to the platform root and the nonce, and extract PCR0, PCR1, PCR2:

```
att-vendor ra-parse --doc doc.cbor --out-file parsed.json
```

Sign those measurements with your signing key and register them. The `record key` it prints is the lookup
coordinate:

```
att-vendor register --rpc <url> --registry <0x...> --signing-key-file <key> \
    --measurements parsed.json --config build-config.json
```

Read the registration back by measurement alone:

```
att-vendor lookup --rpc <url> --registry <0x...> --measurements parsed.json
```

Self-check: does what is on-chain match what the enclave attested, is it unexpired, and is the
approver the registry's approver:

```
att-vendor compare --attested parsed.json --rpc <url> --registry <0x...> --record-key <record key register printed>
```

`compare --attested <file> --expected <file>` instead compares two sets of measurements directly.

## Hand-off with the Auditor

Simplest: tell the Auditor the repository and the commit. That is all it needs to rebuild.

Or hand the build declaration over as a file. You write `build-config.json`; `send` commits it to
the root of the repository and pushes:

```
att-vendor send --repo <git url> --config build-config.json
```

```json
{
  "schema": "attestamp.build-config/1",
  "service": "nginx",
  "version": "1.31.6",
  "platform": "aws-nitro",
  "source":   { "repo": "https://github.com/<you>/<app>", "commit": "<git commit>", "path": "mainline/alpine-slim" },
  "tee_info": { "instance_id": "i-...", "region": "eu-north-1", "ami_id": "ami-..." },
  "build_env": { "os": "linux", "arch": "amd64", "go": "go1.26.1", "trimpath": true }
}
```

`build_env` is the environment the image was built in: OS, CPU architecture (`amd64` or `arm64`),
Go toolchain, and whether `-trimpath` was used. If you built with Attestamp, leave it out and `send`
fills it in from the running binary. If you built outside Attestamp, write it yourself. The Auditor
builds to that environment, not to whatever it happens to have, because PCR0, PCR1 and PCR2 differ
between x86_64 and arm64 and between Go versions.

The Auditor must be able to read the repository. For a private one, give it read access
(collaborator or deploy key); Attestamp does not manage permissions. `send` and `receive` fail with exit
code 8 (git not found), 9 (repository unreachable), 10 (commit not found, see `--ref`) or 11 (push
rejected: write access, or the remote is ahead, so pull first). By hand: commit `build-config.json`
at the repository root and push; the Auditor clones it and reads the file. `--workdir` makes `send`
work in an existing clone instead of a temporary one.

Later, when the Auditor has published its result, fetch it. `measurement-record.json` and the image
arrive with their hashes verified:

```
att-vendor receive --repo <git url> --into from-auditor
```

`--ref` picks a commit, branch or tag; the default is the default branch. A received image is
launched with `att-vendor launch --eif <file> --build-measurements <file>`.

## Other commands

`ra-serve`, `lo-up` and `ra-proxy` run the responder and the TCP bridge by hand. `build`, `pack`
and `launch` already do this for you. `att-vendor version` prints the pinned and the actual Go
toolchain.

## Key types, fields and functions

A reading map of the code behind `att-vendor`, limited to the pieces a new contributor meets first. Links are relative to this folder.

### Command dispatch

`main` in [main.go](main.go) switches on the first argument, runs the matching function, and turns the returned error into an exit code with `exitCode`.

| Name | Where | What it is or does |
| --- | --- | --- |
| `build` | [main.go](main.go) `buildCmd` | Parses flags into a `Build` and calls `Run`: software Dockerfile to EIF and PCRs. |
| `pack` | [main.go](main.go) `packCmd` | Same `Build`, but starts from `--docker-image` instead of a Dockerfile. |
| `launch` | [main.go](main.go) `launchCmd` | Runs `Launch.Run`, asks the new enclave for a self-check attestation, then calls `OpenPort`. |
| `send` | [main.go](main.go) `send` | Fills `build_env` if empty, then `GitTransport.Send` pushes `build-config.json`. |
| `receive` | [main.go](main.go) `receive` | `GitTransport.Receive` clones the Auditor's record and image and verifies their hashes. |
| `self-register` | [self_register.go](self_register.go) `selfRegisterCmd` | Attest your own enclave, register, read back and check (flow below). |
| `register` | [chain.go](chain.go) `register` | Signs the measurements with your signing key and registers them; returns the record key. |
| `lookup` | [chain.go](chain.go) `lookup` | Reads one record key, or finds the record keys for a set of measurements first. |
| `compare` | [compare.go](compare.go) `compareCmd` | Two sets of measurements, or a set against a registration (`checkMode`); prints PASS or FAIL. |
| `registry-deploy` | [chain.go](chain.go) `registryDeploy` | Deploys the registry contract with a fixed approver. |
| `ra-request` | [main.go](main.go) `raRequest` | Sends a fresh nonce to a TEE address and saves the document and `.nonce`. |
| `ra-parse` | [parse.go](parse.go) `raParse` | Verifies signature, chain and nonce, writes the measurements. |
| `ra-serve` | [main.go](main.go) `raServe` | Responder inside the enclave: `Serve` with the `nsm` or fixture extractor, on tcp or vsock. |
| `ra-proxy` | [main.go](main.go) `raProxy` | Parent-instance bridge from TCP to the enclave's vsock. |
| `lo-up` | [main.go](main.go) | Calls `LoopbackUp` in [internal/tee/request/Vendor/aws](../../internal/tee/request/Vendor/aws) to bring up loopback in the enclave. |
| `version` | [main.go](main.go) | Prints the pinned and actual Go toolchain; exit 1 if they differ. |

### Build

| Name | Where | What it is or does |
| --- | --- | --- |
| `Build` | [build.go](../../internal/setup/build/Vendor/aws/build.go) | Input to one build; `Run` goes tools, software image, responder layer, EIF, PCRs. |
| `AppDir` | same | Directory holding the software's Dockerfile (`build`). Exactly one of `AppDir` and `DockerImage`. |
| `DockerImage` | same | An image you already built (`pack`); att only layers, normalizes and makes the EIF. |
| `ImageName` | same | Image and EIF name; defaults to the directory name or the image name without its tag. |
| `ResponderPath` | same | The `att-ra-responder` to layer in; default is the file of that name next to the executable. |
| `OutDir` | same | Result directory, default `out`. |
| `Port` | same | vsock port the responder listens on inside the enclave; 0 means 5000. |
| `Verify` | same, [verify.go](../../internal/setup/build/Vendor/aws/verify.go) | Rebuild once more without cache and fail unless PCR0, PCR1 and PCR2 match. |
| `Result` | same | Output: `Name`, `Image` tag, `EIF` path, `Measurements` file, `PCR` map, and `Responder` info of the layered-in binary. |

### Launch

| Name | Where | What it is or does |
| --- | --- | --- |
| `Launch` | [launch.go](../../internal/setup/launch/Vendor/aws/launch.go) | Input to one launch; `Run` starts the enclave and the bridge. |
| `EIF`, `EnclaveName` | same | The image file to run, and the enclave name (default: the file name). |
| `Port`, `Listen` | same | RA port (default 5000, must match build) and the address the bridge accepts on. |
| `Memory`, `CPUs`, `CID` | same | Enclave MiB (0 sizes it from the EIF), CPU count (2), vsock address (16). |
| `OutDir` | same | Log and self-check directory, default `out`. |
| `BuildMeasurements` | same | For a received EIF: measure it directly ([measure.go](../../internal/setup/launch/Vendor/aws/measure.go)) and refuse to launch if it differs. |
| `Result` | same | `Name`, `Measurements` file, `PCR` map, `Memory` given and `Address` to check. |
| `OpenPort` | [port.go](../../internal/setup/launch/Vendor/aws/port.go) | Opens the security group port, or returns the command to run when permission is missing. |

### build-config.json

Defined by `BuildConfig` in [build_config.go](../../internal/core/transfer/Vendor/build_config.go); `Validate` enforces it.

| Name | Where | What it is or does |
| --- | --- | --- |
| `schema` | [build_config.go](../../internal/core/transfer/Vendor/build_config.go) | Must be `attestamp.build-config/1`. |
| `service`, `version` | same | What you are registering; required, and they name the registration. |
| `platform` | same | `aws-nitro` here; decides which `tee_info` keys are required. |
| `source` | same | `repo` and `commit` (both required) the Auditor clones, and an optional `path` to the app folder. |
| `tee_info` | same | Facts about your instance; for `aws-nitro`: `instance_id`, `region`, `ami_id`. |
| `build_env` | same | `BuildEnv`: the environment the image was built in; required for `aws-nitro`. |
| `build_env.os` | same | GOOS of the build, for example `linux`. |
| `build_env.arch` | same | `amd64` or `arm64`; measurements differ per architecture. |
| `build_env.go` | same | Go toolchain that compiled `att-ra-responder`, for example `go1.26.1`. |
| `build_env.trimpath` | same | Whether `-trimpath` was used. `false` is allowed but the Auditor warns. |

### Measurements

| Name | Where | What it is or does |
| --- | --- | --- |
| `Measurement`, `Measurements` | [parse.go](../../internal/tee/parse/Vendor/parse.go) | A named value (`Name`, `Value` bytes) and the list of them; `Get` finds one by name. |
| PCR0 | [build.go](../../internal/setup/build/Vendor/aws/build.go) `ReadPCR` | Hash of the whole enclave image file (EIF). |
| PCR1 | same | Hash of the kernel and boot ramdisk. |
| PCR2 | same | Hash of the application, that is the software and the layered-in responder. |
| `AWSParser` | [aws.go](../../internal/tee/parse/Vendor/aws.go) | Nitro parser: `VerifySig`, `CheckNonce`, `Parse`, `Info`. |
| `MeasurementHash` | [register.go](../../internal/chain/register/Vendor/register.go) | Reduces a set to the one 32-byte value the registry stores. |
| `RADoc`, `Nonce` | [request.go](../../internal/tee/request/Vendor/request.go), [nonce.go](../../internal/tee/request/Vendor/nonce.go) | The raw attestation document with its format name, and the fresh value that binds it to now. |

### Registration and lookup

| Name | Where | What it is or does |
| --- | --- | --- |
| `Meta` | [register.go](../../internal/chain/register/Vendor/register.go) | Service, version, platform, arch and `Expires`; no revocation exists. |
| `Registrar` | same | `Sign` makes the signature that says who registered; `Register` sends it to the contract. |
| `Signed`, `Receipt` | same | The signed value ready to send, and the result with `RecordKey`, `TxHash`, `Block`. |
| `Reader` | [lookup.go](../../internal/chain/lookup/Vendor/lookup.go) | `FindRecordKeys` by measurements, `Lookup` by record key; the chain-backed one is in [reader.go](../../internal/chain/lookup/Vendor/ether/reader.go). |
| `Registration` | same | What the chain returns: values, meta, `Expires`, `ChainTime`, `Approver`, `Signature`. Expiry is judged on `ChainTime`. |
| `ResolveRegistration` | [registration.go](../../internal/core/transfer/Vendor/registration.go) | Takes service, version and arch from the config; a differing flag wins with a warning. |

### Exit codes

`exitCode` in [main.go](main.go) maps the error to a code; anything unmatched is 1.

| Name | Where | What it is or does |
| --- | --- | --- |
| 1 | [main.go](main.go) | Any other failure, including a failed `compare` and a toolchain mismatch. |
| 2 | [main.go](main.go) | No command or an unknown command (usage). |
| 3 | [main.go](main.go) | Network error. |
| 4 | [main.go](main.go) | Not the registry's approver, or the document is not the expected format. |
| 5 | [main.go](main.go) | Already registered; bad document signature or certificate chain; extraction failed in the TEE. |
| 6 | [main.go](main.go) | Expiry not in the future on the chain's clock; nonce mismatch; malformed or oversized response. |
| 7 | [main.go](main.go) | The attestation document is malformed. |
| 8 to 11 | [main.go](main.go) | `send` and `receive`: git missing (8), repository unreachable (9), commit not found (10), push rejected (11). |

### The self-register flow

| Name | Where | What it is or does |
| --- | --- | --- |
| `selfRegisterCmd` | [self_register.go](self_register.go) | Drives the whole flow; resolves service, version and arch from `--config` first. |
| `raRequest` | [main.go](main.go) | Asks your own enclave for an attestation with a fresh nonce. |
| `raParse` | [parse.go](parse.go) | Checks signature chain and nonce, and writes the attested PCRs. |
| `Compare` | [compare.go](../../internal/core/compare/Vendor/compare.go) | Attested value against your `--build-measurements`; a difference stops the run. |
| `register` | [chain.go](chain.go) | Signs with your signing key and registers; returns the record key. |
| `Check` | [compare.go](../../internal/core/compare/Vendor/compare.go) | After `Lookup`, checks value, expiry (chain clock) and approver; fails with exit 1. |
