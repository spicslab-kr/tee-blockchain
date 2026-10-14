# Vendor guide (`att-vendor`)

As the Vendor you build or pack your software into an enclave image, launch it, and either hand the
source to an Auditor or register the measurement yourself (flow 2). For the binaries and the pinned
toolchain, see [Build the binaries](../README.md#build-the-binaries). `att-vendor` and
`att-ra-responder` must sit in the same directory on the Nitro instance.

Both ways of building end with the same three things, and the rest of the system uses only these:

| what | where it goes |
|---|---|
| the enclave image (`.eif`) | `launch`, and the hand-off |
| the measurements, nitro-cli's JSON (`{"Measurements":{"PCR0":...,"PCR1":...,"PCR2":...}}`) | `launch --build-measurements`, `register --measurements`, `compare --expected` |
| the address `host:port` the enclave answers on | `--tee-addr` for the Auditor and the User |

Building is optional. You can launch an image received from the Auditor directly (see
[Hand-off](#hand-off-with-the-auditor)).

## Build, pack and launch

### Image handed over as a file

You build the software image with any tool, `pack` turns it into an enclave image, and you give
the Auditor the image and its measurements. The Auditor measures the file it receives and does not rebuild.

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
attestation to confirm it answers (`out/selfcheck.cbor`), and opens the security-group port. If the
instance has no IAM permission to do that, `launch` prints the one `aws ec2` line to run from your account.

```
att-vendor launch --eif out/app.eif
```

Flags: `--eif` (required), `--build-measurements`, `--enclave-name`, `--port` (default 5000, must match the
build), `--memory` (MiB, default sized from the image), `--cpus` (2), `--cid` (16), `--listen`
(0.0.0.0), `--out-dir`.

Send `out/app.eif` and `out/measurements.json` to the Auditor by any means (scp, mail). The Auditor's
`launch --eif <file> --build-measurements <file>` measures the image and launches it only if the
file measures to what the JSON claims.

### Source rebuilt by the Auditor

You give the Auditor the repository and commit. You each run the same two commands on your own
Nitro instance and must get the same PCR0, PCR1 and PCR2. That comparison is the audit.

`build` installs docker and nitro-cli if they are missing, builds the image, layers the responder,
normalizes the layers, and writes `out/<name>.eif` and `out/measurements.json`.

```
att-vendor build --app-dir <dir with Dockerfile> --out-dir out
```

`build` first scans the Dockerfile for known causes of unreproducible images (unpinned base image or
packages, `date` or `hostname`, random keys, compiling from source) and prints each with its line.
Add `--verify` to build a second time from scratch; it fails unless the measurements come out
identical and lists the files that differ. Use it the first time you send a source to an Auditor.

```
att-vendor launch --eif out/<name>.eif
```

`build` takes a directory with a Dockerfile and runs `docker build` for you. `pack` takes an image
you already built. They share `--image-name`, `--port`, `--out-dir` and `--responder`.

## Self-register (flow 2)

In flow 1 the Auditor audits and registers. In flow 2 you register your own enclave with
`self-register`:

```
att-vendor self-register --config build-config.json --tee-addr 127.0.0.1:5000 \
    --rpc <url> --registry <0x...> --signing-key-file <key>
```

`--config` is the same `build-config.json` that `send` pushes. Service, version and the build
environment (`build_env`: OS, CPU architecture, Go toolchain, `-trimpath`) come from it.
`--service`, `--version` and `--arch` override the file and print a warning when they
differ (`warning: --service nginx2 differs from build-config.json (nginx)`). Without `--config`,
you give `--service` and `--version`, or `self-register` takes them from an existing registration of the same
value on that registry.

The confirmation names the CPU architecture and Go toolchain the enclave was built with
(`registered nginx 1.31.6 (amd64, go1.26.1 -trimpath)`). The architecture is also stored on-chain in the
registration's `arch` field, and `lookup` prints it back. It is display only and is no part of the record key.

Further flags: `--build-measurements <file>` (compares the attestation to your build value first and
refuses to register on a mismatch, printing the differing PCRs), `--expires` (default 2160h, that
is 90 days), `--platform` (default `aws-nitro`), `--format`, `--timeout`, `--nonce`, `--trusted-approvers`,
`--out-dir`.

`self-register` asks interactively for any value you leave out. For `--rpc` it asks for
`y` to use Ethereum mainnet, and it takes any other text as the RPC URL (a testnet, for
example).

You deploy a registry for flow 2 once with:

```
att-vendor registry-deploy --rpc <url> --deployer-key-file <deployer.key> --approver <0x...>
```

The [registry guide](../../contracts/README.md) describes what the contract accepts.

### The steps it wraps

Send a fresh nonce to the enclave and receive the signed attestation document:

```
att-vendor ra-request --tee-addr 127.0.0.1:5000 --out-file doc.cbor
```

Verify the signature chain up to the platform root and the nonce, and extract PCR0, PCR1 and PCR2:

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

Check that the on-chain value matches what the enclave attested, that it has not expired, and that the
approver is the registry's approver:

```
att-vendor compare --attested parsed.json --rpc <url> --registry <0x...> --record-key <record key register printed>
```

`compare --attested <file> --expected <file>` compares two sets of measurements directly.

## Hand-off with the Auditor

The simplest hand-off is to tell the Auditor the repository and the commit, which is all it needs to rebuild.

You can also hand over the build declaration as a file. You write `build-config.json`, and `send` commits it to
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

`build_env` is the environment you built the image in: OS, CPU architecture (`amd64` or `arm64`),
Go toolchain, and whether you used `-trimpath`. If you built with Attestamp, leave it out and `send`
fills it in from the running binary. If you built elsewhere, write it yourself. The Auditor
builds to the environment you declare, because PCR0, PCR1 and PCR2 differ
between x86_64 and arm64 and between Go versions.

The Auditor must be able to read the repository. For a private one, give it read access
(collaborator or deploy key), because Attestamp does not manage permissions. To do the same by hand, commit
`build-config.json` at the repository root and push, and the Auditor clones it and reads the file.
`--workdir` makes `send` work in an existing clone. Without it, `send` uses a temporary clone.

When the Auditor has published its result, fetch it. `measurement-record.json` and the image
arrive with their hashes verified:

```
att-vendor receive --repo <git url> --into from-auditor
```

`--ref` picks a commit, branch or tag, and the default is the default branch. You launch a received
image with `att-vendor launch --eif <file> --build-measurements <file>`.

## Other commands

`ra-serve`, `lo-up` and `ra-proxy` run the responder and the TCP bridge by hand. `build`, `pack`
and `launch` already run them for you. `att-vendor version` prints the pinned and the actual Go
toolchain.

## Troubleshooting

Errors print as `att-vendor: <message>`. The exit codes are listed in [Exit codes](../README.md#exit-codes).
Codes 8 to 11 are returned by `send` and `receive` for git failures, and code 4 from `register` and
`self-register` means your key is not the registry's approver.

`att was built with go... but the project pins go...`: your binary was built with the wrong Go
toolchain. Rebuild it as described in [Build the binaries](../README.md#build-the-binaries).

`<path>: attestation responder not found (build cmd/ra-responder and put it there, or point ResponderPath at it)`:
`build` or `pack` cannot find `att-ra-responder`. Put it next to `att-vendor` or pass `--responder <path>`.

`the EIF does not measure to what --build-measurements claims; not launching`: the image file and its
measurements JSON do not belong together. Ask the sender for a matching pair.

`enclave is not running: the software may have exited at once; run nitro-cli run-enclave with --debug-mode --attach-console to see why`:
your software exited at start. Run the command the message names and read the console output.

`the enclave is not running what I built, so this audit is not registering anything`: with
`--build-measurements`, the attested PCRs differ from your build, and `self-register` lists the differing
ones above this line. Rebuild or relaunch so the running enclave matches your build, then run
`self-register` again.

`this measurement is already registered on this platform` (exit 5): that value is on the registry. Run
`att-vendor lookup --measurements <file>` with the same `--rpc` and `--registry` to read the existing registration.

### What a successful self-register prints

The run prints the lines below, with placeholders for the values. On a terminal each step shows a
spinner first and then ends with the same text and a check mark.

```
Asking my enclave at 127.0.0.1:5000 for an attestation... ✓ aws-nitro document, <n> bytes
Checking the attestation (signature chain to the platform root, nonce)... ✓ genuine
    PCR0  <hex>
    PCR1  <hex>
    PCR2  <hex>
Comparing what the enclave attested with my own value (out/measurements.json)... ✓ identical: the enclave runs exactly what I built
Registering nginx 1.31.6 (amd64) on the chain, signed with my signing key... ✓ registered
    record key  0x...
    tx          0x...
    block       <n>
    expires     <RFC 3339 time>
    approver    0x...
    build env   <build_env>
Reading the registration back from the chain and checking it... ✓ value, expiry and approver all check out
PASS: registered by myself (no Auditor). Users can verify against registry 0x...
```

The comparison step appears only with `--build-measurements`. After each step a line starting with `·`
explains why the step is needed.

## Reading the code

Execution starts in `main` in [main.go](main.go), which switches on the first argument and calls one
function per command. This folder holds the flag parsing and the chain commands. The build, launch,
transfer, registration and comparison logic lives under [internal](../../internal), in packages named
`Vendor` or `Vendor/aws`.

- [selfRegisterCmd](self_register.go) runs the flow 2 sequence: request, parse, compare, register, read back, check.
- [Build.Run](../../internal/setup/build/Vendor/aws/build.go) turns a Dockerfile or an existing image into an EIF and its PCR values.
- [Launch.Run](../../internal/setup/launch/Vendor/aws/launch.go) starts the enclave and the TCP bridge to its vsock.
- [OpenPort](../../internal/setup/launch/Vendor/aws/port.go) opens the security-group port, or returns the command to run when permission is missing.
- [BuildConfig](../../internal/core/transfer/Vendor/build_config.go) defines `build-config.json` and validates it.
- [GitTransport](../../internal/core/transfer/Vendor/git.go) implements `Send` and `Receive` for the hand-off.
- [Registrar](../../internal/chain/register/Vendor/register.go) signs the measurements and sends the registration to the contract.
- [Compare and Check](../../internal/core/compare/Vendor/compare.go) compare two sets of measurements, or a set against a registration (value, expiry on the chain's clock, approver).

### build-config.json fields

`Validate` in [build_config.go](../../internal/core/transfer/Vendor/build_config.go) enforces these.

| field | meaning |
|---|---|
| `schema` | Must be `attestamp.build-config/1`. |
| `service`, `version` | What you register. Both are required and they name the registration. |
| `platform` | `aws-nitro`. It decides which `tee_info` keys are required. |
| `source` | `repo` and `commit` (both required) that the Auditor clones, and an optional `path` to the app folder. |
| `tee_info` | Facts about your instance. For `aws-nitro`: `instance_id`, `region`, `ami_id`. |
| `build_env.os` | GOOS of the build, for example `linux`. Required for `aws-nitro`. |
| `build_env.arch` | `amd64` or `arm64`. Required for `aws-nitro`, since measurements differ per architecture. |
| `build_env.go` | Go toolchain that compiled `att-ra-responder`, for example `go1.26.1`. Required for `aws-nitro`. |
| `build_env.trimpath` | Whether you used `-trimpath`. `false` is allowed and the Auditor warns about it. |
