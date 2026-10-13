# Vendor guide (`tb-vendor`)

The Vendor builds or packs the software into an enclave image, launches it, and either hands the
source to an Auditor or registers the measurement itself (flow 2). Binaries and toolchain:
[Build the binaries](../README.md#build-the-binaries). `tb-vendor` and `tb-ra-responder` must sit in
the same directory on the Nitro instance.

Both ways of building must end with the same three things, because that is all the rest of the
system uses:

| what | where it goes |
|---|---|
| the enclave image (`.eif`) | `launch`, and the hand-off |
| the measurements, nitro-cli's JSON (`{"Measurements":{"PCR0":...,"PCR1":...,"PCR2":...}}`) | `launch --measurements`, `register --measurements`, `compare --b` |
| the address `host:port` the enclave answers on | `--tee` for the Auditor and the User |

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
tb-vendor pack --image app:base --out out
```

Flags: `--image` (required), `--name`, `--port` (default 5000), `--out` (default `out`),
`--responder`.

`launch` starts the enclave, bridges TCP 5000 to the enclave's vsock, asks the new enclave for an
attestation to prove it answers (`out/selfcheck.cbor`), and opens the security-group port. Without
IAM permission on the instance it prints the one `aws ec2` line to run from your account.

```
tb-vendor launch --eif out/app.eif
```

Flags: `--eif` (required), `--measurements`, `--name`, `--port` (default 5000, must match the
build), `--memory` (MiB, default sized from the image), `--cpus` (2), `--cid` (16), `--listen`
(0.0.0.0), `--out`.

Hand `out/app.eif` and `out/measurements.json` to the Auditor by any means (scp, mail). Its
`launch --eif <file> --measurements <file>` measures the image itself and launches it only if the
file measures to what the JSON claims.

### Source rebuilt by the Auditor

You give the Auditor the repository and commit. Both of you run the same two commands on your own
Nitro instance and must get the same PCR0, PCR1 and PCR2. That comparison is the audit.

`build` installs docker and nitro-cli if they are missing, builds the image, layers the responder,
normalizes the layers, makes the image and writes the measurements: `out/<name>.eif` and
`out/measurements.json`.

```
tb-vendor build --app-dir <dir with Dockerfile> --out out
```

It first scans the Dockerfile for things known to break reproducibility (unpinned base image or
packages, `date` or `hostname`, random keys, compiling from source) and prints each with its line.
Add `--verify` to build a second time from scratch and fail unless the measurements come out
identical, listing the files that differ. It is worth doing the first time a source goes to an
Auditor.

```
tb-vendor launch --eif out/<name>.eif
```

`build` takes a directory with a Dockerfile and runs `docker build` itself; `pack` takes an image
you already built. They share `--name`, `--port`, `--out` and `--responder`.

## Self-register (flow 2)

In flow 1 the Auditor audits and registers, and the Vendor does not register. `self-register` is
for flow 2, where the Vendor registers its own enclave:

```
tb-vendor self-register --config build-config.json --tee 127.0.0.1:5000 \
    --rpc <url> --contract <0x...> --key-file <key>
```

`--config` is the same `build-config.json` that `send` pushes: service, version and the build
environment (`build_env`: OS, CPU architecture, Go toolchain, `-trimpath`) are taken from it.
`--service`, `--version` and `--arch` still work and override the file, with a warning when they
differ (`warning: --service nginx2 differs from build-config.json (nginx)`). Without `--config`,
`--service` and `--version` are needed, or are taken from an existing registration of the same
value on that contract.

The confirmation names the CPU architecture and Go toolchain the enclave was built with
(`registered nginx 1.31.6 (amd64, go1.26.1 -trimpath)`). The architecture also goes on-chain in the
entry's `arch` field, so `lookup` prints it back. It is display only and not part of the lookup key.

Further flags: `--measurements <file>` (compare the attestation to your build value first and
refuse to register on a mismatch, printing the differing PCRs), `--expires` (default 2160h, that
is 90 days), `--platform` (default `aws-nitro`), `--format`, `--timeout`, `--nonce`, `--trusted`,
`--out`.

Any value you leave out is asked for interactively. `--rpc` is special: when empty, it asks for
`y` to use Ethereum **mainnet**, and any other text is taken as the RPC URL (a testnet, for
example).

A registry for flow 2 is deployed once with:

```
tb-vendor registry-deploy --rpc <url> --key-file <deployer.key> --approver <0x...>
```

See the [registry guide](../../contracts/README.md) for what the contract accepts.

### The steps it wraps

Send a fresh nonce to the enclave and get back the signed attestation document:

```
tb-vendor ra-request --tee 127.0.0.1:5000 --out doc.cbor
```

Verify the signature chain up to the platform root and the nonce, and extract PCR0, PCR1, PCR2:

```
tb-vendor ra-parse --doc doc.cbor --out parsed.json
```

Sign those measurements with your key and register them. The `key` it prints is the lookup
coordinate:

```
tb-vendor register --rpc <url> --contract <0x...> --key-file <key> \
    --measurements parsed.json --config build-config.json
```

Read the registration back by measurement alone:

```
tb-vendor lookup --rpc <url> --contract <0x...> --measurements parsed.json
```

Self-check: does what is on-chain match what the enclave attested, is it unexpired, and is the
signer the contract's approver:

```
tb-vendor compare --a parsed.json --rpc <url> --contract <0x...> --key <key register printed>
```

`compare --a <file> --b <file>` instead compares two sets of measurements directly.

## Hand-off with the Auditor

Simplest: tell the Auditor the repository and the commit. That is all it needs to rebuild.

Or hand the build declaration over as a file. You write `build-config.json`; `send` commits it to
the root of the repository and pushes:

```
tb-vendor send --repo <git url> --config build-config.json
```

```json
{
  "schema": "tb.build-config/1",
  "service": "nginx",
  "version": "1.31.6",
  "platform": "aws",
  "source":   { "repo": "https://github.com/<you>/<app>", "commit": "<git commit>", "path": "mainline/alpine-slim" },
  "tee_info": { "instance_id": "i-...", "region": "eu-north-1", "ami_id": "ami-..." },
  "build_env": { "os": "linux", "arch": "amd64", "go": "go1.26.1", "trimpath": true }
}
```

`build_env` is the environment the image was built in: OS, CPU architecture (`amd64` or `arm64`),
Go toolchain, and whether `-trimpath` was used. If you built with tb, leave it out and `send`
fills it in from the running binary. If you built outside tb, write it yourself. The Auditor
builds to that environment, not to whatever it happens to have, because PCR0, PCR1 and PCR2 differ
between x86_64 and arm64 and between Go versions.

The Auditor must be able to read the repository. For a private one, give it read access
(collaborator or deploy key); tb does not manage permissions. `send` and `receive` fail with exit
code 8 (git not found), 9 (repository unreachable), 10 (commit not found, see `--ref`) or 11 (push
rejected: write access, or the remote is ahead, so pull first). By hand: commit `build-config.json`
at the repository root and push; the Auditor clones it and reads the file. `--workdir` makes `send`
work in an existing clone instead of a temporary one.

Later, when the Auditor has published its result, fetch it. `measurement-record.json` and the image
arrive with their hashes verified:

```
tb-vendor receive --repo <git url> --into from-auditor
```

`--ref` picks a commit, branch or tag; the default is the default branch. A received image is
launched with `tb-vendor launch --eif <file> --measurements <file>`.

## Other commands

`ra-serve`, `lo-up` and `ra-proxy` run the responder and the TCP bridge by hand. `build`, `pack`
and `launch` already do this for you. `tb-vendor version` prints the pinned and the actual Go
toolchain.
