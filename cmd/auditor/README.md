# att-auditor

att-auditor rebuilds the Vendor's source in the same environment, checks the result against your own
enclave, and registers the hash on the chain. `att-ra-responder` must sit in the same directory as
`att-auditor` on your Nitro instance.

This page shows the Nitro commands. On a TDX or SEV-SNP guest `build` and `launch` take `--platform` and `audit` takes
`--format`; follow "The Auditor's half" in [docs/build/tdx.md](../../docs/build/tdx.md) or
[docs/build/sev-snp.md](../../docs/build/sev-snp.md).

**What you build:** `att-auditor`, `att-ra-responder` and the Vendor's enclave image, rebuilt from its
source. See [Build](../../README.md#build).

## Receive the source

```
att-auditor receive --repo <git url> --into work/src
```

`--ref` picks a commit, branch or tag. `receive` clones the repository, reads `build-config.json` and
runs `CheckBuildEnv`: your OS, CPU architecture and Go version must match the Vendor's `build_env`,
or the command stops with an error. `--ignore-env` turns that error into a warning. A Vendor build
without `-trimpath` only gives a warning. On `aws-nitro` the Amazon Linux release must match too
(`build_env.ami_release`, which the Vendor's `send` records): it decides which docker and nitro-cli
`build` will install. That is a proxy, not a guarantee: each Amazon Linux release has its own package repository, but a `dnf update` with another release, `releasever=latest` in `/etc/dnf/vars`, or a package installed by hand moves the versions without moving the release. On `aws-nitro` a config without `ami_release`, `docker` and `nitro_cli` is invalid (since 2026-11-17), so
neither check can be skipped.

## Build and launch

This is the Vendor's [build and launch](../vendor/README.md#build-and-launch) with `att-auditor` in
place of `att-vendor`:

```
att-auditor build --config work/src/build-config.json --app-dir work/src/<path> --out-dir out
att-auditor launch --eif out/<name>.eif
```

With `--config`, `build` compares the docker and aws-nitro-enclaves-cli versions it has just installed with
the Vendor's (`build_env.docker`, `nitro_cli`) and stops before building anything if either differs
(`--ignore-env` makes it a warning). This is the real check behind `receive`'s release comparison. Without
`--config` the versions are printed and not compared.

Your own enclave's values are what you register. To check the Vendor's running enclave against them, give `audit`
`--vendor-tee <vendor-instance>:5000` (below): it registers your value and then says whether the Vendor's enclave runs
the same; if it does not, that is the finding, and `audit` exits 12.

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

`--config` also sets the document format, from the config's `platform`; without `--config`, `--format` defaults to
`aws-nitro`. A `--format` you give wins over the config, with a warning when the two differ.

`--build-measurements` is required whenever `--tee-addr` is given, on every platform (since 2026-11-16): without a
value you built yourself, `audit` would register whatever the TEE reports. To look at a TEE's report without
registering anything, use `ra-request` and `ra-parse`. With `--vendor-tee`, a Vendor TEE that runs something else
ends `audit` with exit 12 after your value is registered; do not run it again (see [Exit codes](../README.md#exit-codes)).

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
    block       <n>
    expires     <time>
    approver    0x...
    build env   linux/amd64 go1.26.1 trimpath=true …
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
[hand-off section](../vendor/README.md#hand-off-with-the-auditor) of the Vendor guide.
One service and version has one record and one registration per CPU architecture.
Sending content the repository already has commits nothing: `send` prints `already up to date` and exits 0, so re-running it is safe.

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
| `pack` | Nitro: turns a Docker image you built yourself into an enclave image, with the same normalising and responder layering as `build` (`--docker-image`). |
| `ra-request` | Fetches an attestation document from `--tee-addr`. |
| `ra-parse` | Verifies a document and extracts its measurements (Nitro PCR0–2, TDX MRTD and RTMR0–2, SEV-SNP the five or six values); `--format` for a guest. |
| `register` | Signs and registers measurements. `audit` calls it. |
| `lookup` | Finds registrations by `--measurements` or `--record-key`. |
| `compare` | Compares two sets of measurements, or a set of measurements with a registration (`--record-key`). |
| `version` | Prints the pinned Go toolchain and the one in use. |
| `ra-serve` | Answers attestation requests inside the enclave on Nitro (`--extractor nsm`), or serves a recorded document for tests (`fixture=<doc>`). On TDX and SEV-SNP guests the responder is `att-ra-responder ra-serve --extractor <tdx, snp-initrd or snp-verity>`. |
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
