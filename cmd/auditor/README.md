# att-auditor

att-auditor rebuilds the Vendor's source in the same environment, checks the result against your own
enclave, and registers the hash on the chain. `att-ra-responder` must sit in the same directory as
`att-auditor` on your Nitro instance.

**What you build:** `att-auditor`, `att-ra-responder` and the Vendor's enclave image, rebuilt from its
source. See [Build](../../README.md#build).

## Receive the source

```
att-auditor receive --repo <git url> --into work/src
```

`--ref` picks a commit, branch or tag. `receive` clones the repository, reads `build-config.json` and
runs `CheckBuildEnv`: your OS, CPU architecture and Go version must match the Vendor's `build_env`,
or the command stops with an error. `--ignore-env` turns that error into a warning. A Vendor build
without `-trimpath` only gives a warning.

## Build and launch

This is the Vendor's [build and launch](../vendor/README.md#build-and-launch) with `att-auditor` in
place of `att-vendor`:

```
att-auditor build --app-dir work/src/<path> --out-dir out
att-auditor launch --eif out/<name>.eif
```

If your PCR0, PCR1 and PCR2 differ from the Vendor's, that is the finding, and you do not register.

## Deploy the registry
```
att-auditor registry-deploy --rpc <url> --deployer-key-file <deployer.key> --approver <my 0x...>
```

One registry has one approver, fixed at deployment, and it accepts only registrations signed by that
address. A registry deployed before the `arch` field (`Meta.Arch`) does not accept the current
`register`, so deploy a new one.

## Audit
```
att-auditor audit --config work/src/build-config.json --tee-addr <my-instance>:5000 \
    --build-measurements out/measurements.json --rpc <url> --registry <0x...> --signing-key-file <key>
```

`audit` ([audit.go](audit.go)) does these in order:

- Requests and verifies the attestation document from your own enclave.
- Checks that the attested value equals your build value, and stops if it does not.
- Registers the build value with your signing key.
- Reads the registration back from the chain.
- Checks value, expiry and approver, as a User would.

The flags that matter:

- `--config` is the Vendor's `build-config.json`, which `receive` left in the clone root. It supplies service, version and build environment.
- `--build-measurements` is the `measurements.json` from your build, and `--tee-addr` is your own enclave.
- `--registry` and `--rpc` name the registry and the chain, and `--signing-key-file` holds the approver key.
- `--expires` sets the lifetime of the registration (`Meta.Expires`, default 2160h, which is 90 days).

On success you see:

```
    record key  0x...
    tx          0x...
    expires     <time>
    approver    0x...
PASS: audited and registered. Users can now verify against registry 0x...
```

## Send the result back
```
att-auditor send --repo <git url> --config work/src/build-config.json \
    --measurements out/measurements.json --eif out/<name>.eif
```

`send` builds `measurement-record.json` from `build-config.json` and the measurements, adds the EIF's
sha256, and pushes the record and the image. It reads the build tool version from `nitro-cli --version`,
or you give it with `--build-tool-version`. The full exchange is in the
[hand-off section](../vendor/README.md#hand-off-with-the-auditor) of the vendor guide.
One service and version has one record and one registration per CPU architecture.

| Field | Content |
|---|---|
| `schema`, `service`, `version`, `platform` | `attestamp.measurement-record/1`, the build, `aws-nitro` |
| `values` | `PCR0`, `PCR1`, `PCR2` |
| `build_tool` | `name` (`nitro-cli`) and `version` |
| `build_env` | `os`, `arch` (`amd64` or `arm64`), `go`, `trimpath` |
| `image`, `registration` | EIF `path` and `sha256`; optional registry `chain` and `address` |

## Other commands

| Command | What it does |
|---|---|
| `ra-request` | Fetches an attestation document from `--tee-addr`. |
| `ra-parse` | Verifies a document and extracts PCR0, PCR1 and PCR2. |
| `register` | Signs and registers measurements. `audit` calls it. |
| `lookup` | Finds registrations by `--measurements` or `--record-key`. |
| `compare` | Compares two sets of measurements, or a document with a registration. |
| `version` | Prints the pinned Go toolchain and the one in use. |
| `ra-serve` | Runs inside the enclave and answers attestation requests. |
| `ra-proxy` | Forwards TCP connections to the enclave's vsock. |
| `lo-up` | Brings up loopback inside the enclave. |

## Troubleshooting

- `build env differs from the Vendor's (...): this instance cannot produce the Vendor's measurements`: move to a matching instance, or pass `--ignore-env` if you accept that your measurements may differ.
- `the enclave is not running what I built, so this audit is not registering anything`: check that `--tee-addr` is your own enclave and that you launched the EIF you built. If both are right, this is the finding.
- `the signer is not the approver fixed in the registry`: use the key whose address you gave to `registry-deploy --approver`, or deploy a new registry.
- `this measurement is already registered on this platform`: the value is registered. Read it with `lookup`.
- `expiry is not in the future by the chain's clock`: pass a longer `--expires`.

## Reading the code

- `main` in [main.go](main.go) switches on the first argument, and `exitCode` there maps an error to the exit code.
- `auditCmd` in [audit.go](audit.go) runs the audit steps.
- [CheckBuildEnv](../../internal/core/transfer/auditor/build_env_check.go) is the check that `receive` runs.
- [MeasurementRecord](../../internal/core/transfer/auditor/measurement_record.go) is the record that `send` pushes.
- [Registrar](../../internal/chain/register/auditor/register.go) signs and sends a registration (`Meta`), and returns a `Receipt` with the `RecordKey`.
- [Reader.FindRecordKeys and Reader.Lookup](../../internal/chain/lookup/auditor/ether/reader.go) find and read registrations.
