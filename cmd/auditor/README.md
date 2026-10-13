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
