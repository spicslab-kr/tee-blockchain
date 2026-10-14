# Auditor guide (`att-auditor`)

You rebuild the Vendor's software, run your own copy in an enclave, check that the attested value
equals your own build, and only then register that value on the chain. For the binaries and the
pinned toolchain, see [Build the binaries](../README.md#build-the-binaries). `att-auditor` and
`att-ra-responder` must sit in the same directory on your Nitro instance, and the responder must be
the same file the Vendor uses.

## Receive the source

You can `git clone` the repository and `git checkout <commit>` the Vendor named.

If the Vendor used `send`, this command clones the repository, reads `build-config.json`, and checks
that the commit it names is in the history:

```
att-auditor receive --repo <git url> --into work/src
```

`--ref` picks a commit, branch or tag. `receive` also compares the Vendor's `build_env` with your
instance and exits with an error if the OS, CPU architecture or Go version differ, because a build
on your instance would not reproduce the Vendor's measurements and the mismatch would look like a
finding. Move to a matching instance, or pass `--ignore-env` to turn the error into a warning. If
the Vendor built without `-trimpath`, you get a warning only.

## Build and launch

This is the Vendor's [build and launch](../vendor/README.md#build-pack-and-launch) with
`att-auditor` in place of `att-vendor`, and the flags are the same. The same source, the same
`att-ra-responder` and the same normalized layers give you the PCR0, PCR1 and PCR2 the Vendor got.
If they differ, that is a finding, and you register nothing.

Rebuild from source:

```
att-auditor build --app-dir work/src/<path> --out-dir out
```

```
att-auditor launch --eif out/<name>.eif
```

If the Vendor hands over an image file, the Vendor sends the image and its measurements. `launch`
measures the image itself and refuses it if the result does not match:

```
att-auditor launch --eif <received.eif> --build-measurements <received measurements.json>
```

## Deploy the registry (once, first time only)

A registry has one approver, and it accepts only registrations signed by that address. The deployer
key pays gas only and can belong to anyone. The command prints the registry address, which everyone
then uses:

```
att-auditor registry-deploy --rpc <url> --deployer-key-file <deployer.key> --approver <my 0x...>
```

A registry deployed before registrations carried the CPU architecture (`arch`) does not accept the
current `register`, so deploy a new one. See the [registry guide](../../contracts/README.md).

## Audit

```
att-auditor audit --config work/src/build-config.json --tee-addr <my-instance>:5000 \
    --build-measurements out/measurements.json --rpc <url> --registry <0x...> --signing-key-file <key>
```

`--tee-addr` is your own enclave, running your own build of the Vendor's source. The value that goes
on the chain is the one you built from the source and ran yourself, checked against your build
value. Nothing the Vendor reports enters the registration.

`--config` is the Vendor's `build-config.json`, which `receive` left in the clone root. Service,
version and build environment (`build_env`) come from it, so you do not type them again. `--service`,
`--version` and `--arch` still work and override the file, with a warning when they differ. Without
`--config` you need `--service` and `--version` (or an existing registration of the same value
supplies them), and `--arch` defaults to your machine's.

Further flags:

- `--vendor-tee <host:port>` also asks the Vendor's running enclave and reports whether it runs the
  same value. What you register is still your value.
- `--expires` sets the lifetime of the registration (default 2160h, which is 90 days).
- `--platform` (default `aws-nitro`), `--format`, `--timeout`, `--nonce`, `--trusted-approvers` and
  `--out-dir`.

Without `--tee-addr`, `audit` registers `--build-measurements` as is, with no audit. Values you leave
out are asked for interactively. An empty `--rpc` asks for `y` to use Ethereum mainnet, and any other
text is taken as the RPC URL.

### What a successful audit prints

The audit ends with the registration details and a PASS line, here with placeholder values:

```
    record key  0x...
    tx          0x...
    block       <number>
    expires     <time>
    approver    0x...
    build env   <os, arch, go>
PASS: audited and registered. Users can now verify against registry 0x...
```

### The steps it wraps

Get an attestation document from your enclave:

```
att-auditor ra-request --tee-addr <my-instance>:5000 --out-file doc.cbor
```

Verify it and extract PCR0, PCR1 and PCR2:

```
att-auditor ra-parse --doc doc.cbor --out-file parsed.json
```

The audit gate: the enclave's attested value must equal your build value. FAIL means the enclave is
not running what you built, and nothing gets registered:

```
att-auditor compare --attested parsed.json --expected out/measurements.json
```

Register the build value with your signing key:

```
att-auditor register --rpc <url> --registry <0x...> --signing-key-file <key> \
    --measurements out/measurements.json --config work/src/build-config.json
```

Read it back by measurement. The chain reports which architecture the registration is for (for
example `service nginx 1.31.6 (aws-nitro, amd64)`). The `arch` field is in the signed registration
but not in the record key, so anyone can look the value up without knowing it:

```
att-auditor lookup --rpc <url> --registry <0x...> --measurements out/measurements.json
```

Check your own registration against the chain:

```
att-auditor compare --attested parsed.json --rpc <url> --registry <0x...> --record-key <record key register printed>
```

If you point `--tee-addr` at the Vendor's enclave, you attest what the Vendor is running and compare
it to your build. This is the "something looks off" check from flow 2. `--vendor-tee` runs it as an
extra step, and the registration always comes from your own enclave.

## Send the result back to the Vendor

`measurement-record.json` carries the service and version, the PCRs, the build tool version and the
`build_env` the image was built in (OS, CPU architecture as `amd64` or `arm64`, Go toolchain,
`-trimpath`). Everything except the PCRs and the build tool is in the Vendor's `build-config.json`,
and the PCRs are in `out/measurements.json`. `send` builds the record from those two files, fills in
the image's sha256, and pushes the record and the image to the repository:

```
att-auditor send --repo <git url> --config work/src/build-config.json \
    --measurements out/measurements.json --eif out/<name>.eif
```

The build tool is nitro-cli. `send` reads its version from `nitro-cli --version` on your instance,
or you give it with `--build-tool-version`. The record is also left next to the measurements, as
`out/measurement-record.json`. `--workdir` uses an existing clone.

You can also write the record by hand and send that:

```
att-auditor send --repo <git url> --record measurement-record.json --eif out/<name>.eif
```

The same source measures differently on x86_64 and arm64, and under a different Go, so one service
and version has one record and one registration per CPU architecture. `send` refuses a record that
does not name the architecture, and warns when the record's Go is not the pinned one.

Exit codes are listed in [Exit codes](../README.md#exit-codes). `register` and `audit` also return 4
when the signing key's address is not the registry's approver, 5 when the value is already
registered, and 6 when the expiry is already past.

## Troubleshooting

`receive` stops with `build env differs from the Vendor's (...): this instance cannot produce the Vendor's measurements`, followed by `(rerun with --ignore-env to proceed anyway)`.
Your OS, CPU architecture or Go version differs from the Vendor's `build_env`. Move to a matching
instance. Use `--ignore-env` only if you accept that your measurements may differ.

`audit` prints `the enclave is not running what I built, so this audit is not registering anything`.
The attested PCRs differ from `--build-measurements`. Check that `--tee-addr` is your own enclave and
that you launched the EIF you built. If both are right, the difference is a finding, and you register
nothing.

`register` or `audit` fails with `the signer is not the approver fixed in the registry` (exit 4). The
address of your signing key is not the approver set when the registry was deployed. Use the key whose
address you passed as `--approver` to `registry-deploy`, or deploy a new registry.

`register` or `audit` fails with `this measurement is already registered on this platform` (exit 5).
This value is already on the registry under your approver. Read it with `lookup`; there is
nothing more to register.

`register` or `audit` fails with `expiry is not in the future by the chain's clock` (exit 6). The
chain's clock decides expiry, so a short `--expires` or a lagging RPC can trip it. Pass a longer
`--expires`.

`audit` ends with `but note: the Vendor's enclave at <address> does NOT run this value`. You passed
`--vendor-tee`, and the Vendor's running enclave attests a different value from your build. Your
registration is still valid, and a User who verifies the Vendor's enclave against it is told not to
trust it. Ask the Vendor why.

## Reading the code

Execution starts in `main` in [main.go](main.go), which switches on the first argument and maps the
returned error to an exit code. `audit.go` holds the audit flow, `chain.go` the registry commands,
`compare.go` and `parse.go` the comparison and attestation parsing. The packages under
[internal](../../internal) hold the logic: `core/transfer` the build config, the record and the git
exchange, `core/compare` the comparison, `chain/register` and `chain/lookup` the registry client, and
`setup` the image build and launch.

- [`auditCmd`](audit.go) runs the whole audit and stops at the first failure.
- [`raRequest`](main.go) and [`raParse`](parse.go) fetch your enclave's attestation document and verify its signature, chain and nonce.
- [`Impl.Compare`](../../internal/core/compare/auditor/compare.go) is the audit gate, which checks the attested values equal your build.
- [`register`](chain.go) signs your value with your signing key, registers it and returns the record key.
- [`Reader.Lookup`](../../internal/chain/lookup/auditor/ether/reader.go) reads the registration back under the record key.
- [`Impl.Check`](../../internal/core/compare/auditor/compare.go) checks value, expiry (on the chain's clock) and approver together.
- [`CheckBuildEnv`](../../internal/core/transfer/auditor/build_env_check.go) is the environment check `receive` runs.
- [`buildRecord`](main.go) builds the measurement record that `send` pushes.

`measurement-record.json` is the type [`MeasurementRecord`](../../internal/core/transfer/auditor/measurement_record.go):

| Field | Content |
|---|---|
| `schema` | `attestamp.measurement-record/1` |
| `service`, `version` | The build the record is for. |
| `platform` | `aws-nitro`. It decides which values and image fields are required. |
| `values` | The measurements by name: `PCR0`, `PCR1`, `PCR2`. |
| `build_tool` | `name` (`nitro-cli`) and `version`. |
| `build_env` | `os`, `arch` (`amd64` or `arm64`), `go`, `trimpath`. |
| `image` | `path` of the EIF and its `sha256`. |
| `registration` | Optional: `chain` and `address` of the registry. |

[`BuildConfig`](../../internal/core/transfer/auditor/build_config.go) is what the Vendor hands over before the build. It has no `values`, `image` or `build_tool`, and the record adds those after the build.
