# att-vendor

`att-vendor` is the Vendor's tool. It packs or builds your software into an enclave image, launches it
on a Nitro instance, and registers its measurements, either through an Auditor or yourself.
`att-ra-responder` must sit in the same directory as `att-vendor`. To get both binaries, see
[Build the binaries](../README.md#build-the-binaries).

## Build and launch

If you built the software image yourself, `pack` turns it into an enclave image (`Build` with
`DockerImage` set). It layers the attestation responder on top (`ResponderPath`, `att-ra-responder` next
to the binary by default) and writes `out/app.eif` and `out/measurements.json` (`OutDir`).
`--image-name` sets the name and `--port` the port (default 5000).

```
att-vendor pack --docker-image app:base --out-dir out
```

If you have the source, `build` takes a directory with a Dockerfile (`AppDir`), runs `docker build`
and does the same layering. Before building it scans the Dockerfile for known causes of unreproducible
images and prints each with its line. `--verify` (`Verify`) builds a second time from scratch and fails
if the measurements differ.

```
att-vendor build --app-dir <dir with Dockerfile> --out-dir out
att-vendor launch --eif out/app.eif
```

`launch` (`Launch`, `EIF`) starts the enclave, bridges TCP port 5000 to its vsock (`ProxyBinary`), asks the enclave for
an attestation to check that it answers, and opens the security-group port. If the instance may not
change the security group, it prints the one `aws ec2` command to run yourself. `--enclave-name`
(`EnclaveName`), `--memory`, `--cpus`, `--cid`, `--listen` and `--port` adjust the run. For an image
you received, add `--build-measurements <file>` (`BuildMeasurements`). `launch` then measures the image
and starts it only if the result matches the file.

## Self-register (flow 2)

In flow 2 you register your own enclave, with no Auditor.

```
att-vendor self-register --config build-config.json --tee-addr 127.0.0.1:5000 \
    --rpc <url> --registry <0x...> --signing-key-file <key>
```

`self-register` ([`selfRegisterCmd`](self_register.go)) does this, in order:

- asks your enclave for an attestation and verifies it up to the platform root and the nonce
- with `--build-measurements <file>`, compares the attested PCRs with your build and stops on a mismatch
- signs the measurements with your signing key and registers them on the registry
- reads the registration back and checks value, expiry and approver

`--config` supplies service, version and build environment. `--service`, `--version` and `--arch`
override it and print a warning. `--expires` sets the lifetime (default 2160h, 90 days). Any value you
leave out is asked for. For `--rpc`, answering `y` selects Ethereum mainnet and any other text is the
RPC URL, for example a testnet.

```
Asking my enclave at 127.0.0.1:5000 for an attestation... ✓ aws-nitro document, <n> bytes
Checking the attestation (signature chain to the platform root, nonce)... ✓ genuine
Registering nginx 1.31.6 (amd64) on the chain, signed with my signing key... ✓ registered
    record key  0x...
Reading the registration back from the chain and checking it... ✓ value, expiry and approver all check out
PASS: registered by myself (no Auditor). Users can verify against registry 0x...
```

## Hand-off with the Auditor

In flow 1 the Auditor rebuilds your source and registers the result. The exchange runs in this order:

1. You send `build-config.json` to your source repository (`send`).
2. The Auditor receives it, rebuilds the image in the environment you declared and compares the PCRs.
3. The Auditor sends `measurement-record.json` and the EIF back through its own repository.
4. You receive them, hashes verified (`receive`), and launch the EIF.

```
att-vendor send --repo <git url> --config build-config.json
att-vendor receive --repo <auditor git url> --into from-auditor
```

`send` commits the file to the repository root and pushes it. `--workdir` reuses an existing clone.
If you leave `build_env` out, `send` fills it in
from the running binary.

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

| field | meaning |
|---|---|
| `schema` | Must be `attestamp.build-config/1`. |
| `service`, `version` | Required. They name the registration. |
| `platform` | `aws-nitro`. It decides which `tee_info` keys are required. |
| `source` | `repo` and `commit` are required. `path` is the optional app folder. |
| `tee_info` | For `aws-nitro`: `instance_id`, `region`, `ami_id`. |
| `build_env` | `os`, `arch` (`amd64` or `arm64`) and `go` are required. `trimpath` may be false, and the Auditor warns about it. |

The Auditor needs read access to the repository. Exit codes 8 to 11 report git failures from `send` and
`receive` (see [Exit codes](../README.md#exit-codes)).

## Other commands

| command | what it does |
|---|---|
| `ra-request` | Sends a fresh nonce to an enclave and saves its attestation document. |
| `ra-parse` | Verifies a document and extracts PCR0, PCR1 and PCR2. |
| `register` | Signs measurements with your key and registers them. |
| `lookup` | Reads a registration by record key or by measurements. |
| `compare` | Checks two measurement sets, or a set against a registration on the chain. |
| `registry-deploy` | Deploys a registry. See the [registry guide](../../contracts/README.md). |
| `version` | Prints the pinned and the actual Go toolchain. |
| `ra-serve`, `ra-proxy`, `lo-up` | Run the responder, the TCP bridge and loopback by hand. `launch` already runs them. |

## Troubleshooting

- `att was built with go... but the project pins go...`: rebuild the binary as described in
  [Build the binaries](../README.md#build-the-binaries).
- `attestation responder not found (build cmd/ra-responder and put it there, or point ResponderPath at it)`:
  put `att-ra-responder` next to `att-vendor` or pass `--responder <path>`.
- `the EIF does not measure to what --build-measurements claims; not launching`: the image and the
  measurements file do not belong together. Ask the sender for a matching pair.
- `enclave is not running: the software may have exited at once; ...`: run the `nitro-cli` command the message names.
- `this measurement is already registered on this platform` (exit 5): run `lookup --measurements <file>`
  with the same `--rpc` and `--registry` to read the existing registration.

## Reading the code

- [main](main.go) switches on the first argument and calls one function per command. `exitCode` maps errors to exit codes.
- [selfRegisterCmd](self_register.go) runs the flow 2 sequence.
- [Build.Run](../../internal/setup/build/Vendor/aws/build.go) and [Launch.Run](../../internal/setup/launch/Vendor/aws/launch.go) build and start the enclave.
- [BuildConfig](../../internal/core/transfer/Vendor/build_config.go) defines `build-config.json`, and `Validate` checks it.
- [GitTransport](../../internal/core/transfer/Vendor/git.go) implements `Send` and `Receive`.
- [Registrar](../../internal/chain/register/Vendor/register.go) registers a `Signed` with its `Meta` and returns a `Receipt`.
