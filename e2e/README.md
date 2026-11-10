# End-to-end scripts

These scripts run both flows against real Nitro instances and a public chain. Everything runs from
your own machine, in Git Bash, macOS or Linux; steps that must build or launch inside an instance
connect over SSH and stream the script, so nothing is uploaded except the binaries. Run every
script from the repository root.

For tests that need no Nitro hardware, see `go test ./...` in the main README.

## Prerequisites

- Two Nitro-enabled EC2 instances (Linux), one for the Vendor and one for the Auditor. An optional
  third, arm64 (Graviton), is used only by the architecture comparison.
- `git`, an `ssh` client and the pinned Go version on your machine. `0-Setup-local-install.sh`
  installs what is missing.
- The SSH key (`PEM` in `env.sh`, default `pem2.pem`) in the repository root, and two funded key
  files in the repository root: `auditor.key` (signing key, approver in flow 1) and `deployer.key` (deployer key, pays gas,
  signing key and approver in flow 2). Key files are gitignored. Never commit them.
- Test ETH on the chosen network for both keys.

## env.sh and env.local.sh

`e2e/env.sh` holds the shared settings: SSH user and key, chain and RPC, signing and deployer key files, service name and
version, and the helper functions the scripts use. It does not hold instance addresses. Those go in
`e2e/env.local.sh`, which is gitignored: copy `e2e/env.local.example.sh` to `e2e/env.local.sh` and
fill in

```
VENDOR_HOST=<vendor instance public IP or DNS>
AUDITOR_HOST=<auditor instance public IP or DNS>
ARM64_HOST=<arm64 instance, or leave empty>
```

`env.sh` reads it automatically. A script that needs a host that is not set stops with a message
telling you to create the file. `ARM64_HOST` may stay empty: the architecture scripts then only
print guidance.

### Choosing the chain

`NET=sepolia` is the default. Mainnet spends real ETH (deployment about 1.1M gas, one registration
about 89k gas) and needs an explicit confirmation:

```
bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh
NET=mainnet CONFIRM_MAINNET=1 bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh
```

Without `CONFIRM_MAINNET=1`, `NET=mainnet` exits at once. Any other `NET` value is rejected.

## Setup, once

```
bash e2e/0-Setup-local-install.sh   # git, ssh, and the pinned Go on your machine
bash e2e/0-Setup-local-upload.sh    # build Linux binaries and upload them to both instances
```

Stop everything when you are done (the instances themselves are stopped from the AWS console):

```
bash e2e/99-Setup-local-stop.sh     # terminate the enclaves and TCP bridges on both instances
```

## Flow 1: the Auditor rebuilds and registers

Run in order. Steps 1 to 3 are the normal path; 4 and 5 show a tampering attempt being caught.

| script | who | what it does |
|---|---|---|
| `flow1-auditor-registers/1-Auditor-build-audit-handoff.sh` | Auditor | rebuilds the source on its instance, launches it, deploys a registry, runs `audit`, hands the image and record to the Vendor. Options `-c 0x...` (use an existing registry), `-v` (also ask the Vendor's enclave) |
| `flow1-auditor-registers/2-Vendor-launch-check-record.sh` | Vendor | launches the image the Auditor handed over and checks its own enclave against both the received value and the registered one |
| `flow1-auditor-registers/3-User-verify.sh` | User | `att-user verify` against the Vendor's enclave and the Auditor's registry. Exit 0 means trusted |
| `flow1-auditor-registers/4-Vendor-tamper-swap-enclave.sh` | Vendor | quietly runs a different image instead |
| `flow1-auditor-registers/5-User-tamper-verify.sh` | User | the same `verify` now fails with exit 1: nobody registered what is running |

The registry address is saved in `e2e-work/registry.txt` for the later steps.

## Flow 2: the Vendor registers its own enclave

| script | who | what it does |
|---|---|---|
| `flow2-vendor-self-registers/1-Vendor-build-self-register.sh` | Vendor | builds and launches, deploys its own registry, runs `self-register`. Option `-c 0x...` |
| `flow2-vendor-self-registers/2-User-verify.sh` | User | verifies against the Vendor's registry. It passes, but the approver is the Vendor itself |
| `flow2-vendor-self-registers/3-Vendor-tamper-swap-self-register.sh` | Vendor | runs a different image and registers that value in its own registry |
| `flow2-vendor-self-registers/4-Auditor-tamper-audit-check-vendor.sh` | Auditor | audits with `--vendor-tee`, so it reports that the Vendor's enclave runs something else. Needs the Auditor's build value from flow 1 step 1 |
| `flow2-vendor-self-registers/5-User-tamper-verify-both-registries.sh` | User | verifies against both registries. The Vendor's says yes, the Auditor's says no, so the result is not trusted |

The Vendor's registry address is saved in `e2e-work/registry-flow2.txt`.

## Checks outside the two flows

| script | what it checks |
|---|---|
| `toolchain-pin-check.sh` | an att binary built with another Go patch version refuses to `build` |
| `responder-check.sh` | `build --responder` refuses a responder built without `-trimpath`, or for the wrong architecture |
| `build-env-check.sh` | `receive` rejects a `build-config.json` whose `build_env` does not match the instance, and `--ignore-env` only warns |
| `arch-record-check.sh` | the architecture in records and registrations; sends no transaction |
| `arch-measurement-compare.sh` | the same source measures the same on two x86_64 hosts and differently on arm64 (needs `ARM64_HOST`) |

Intermediate files go in `e2e-work/`.

## Settings and variables

Set in `e2e/env.sh` unless noted.

| variable | meaning |
|---|---|
| `VENDOR_HOST`, `AUDITOR_HOST`, `ARM64_HOST` | instance addresses, set in `env.local.sh` and empty by default; `need_host` stops a script if a required one is empty |
| `SSH_USER` | login user on the instances, `ec2-user` |
| `PEM` | SSH key file, relative to the repository root, default `pem2.pem` |
| `SSH`, `SCP` | the ssh and scp command lines built from `PEM` |
| `V`, `A`, `R` | functions that run a command on the Vendor, Auditor or arm64 instance |
| `NET` | `sepolia` (default) or `mainnet`; mainnet also needs `CONFIRM_MAINNET=1` |
| `RPC` | the public RPC URL chosen by `NET` |
| `AUDITOR_SIGNING_KEY` | key file `auditor.key`, the approver in flow 1 |
| `DEPLOYER_KEY`, `VENDOR_SIGNING_KEY` | key file `deployer.key`; pays gas and is the approver in flow 2 |
| `AUDITOR_ADDR`, `VENDOR_ADDR` | the addresses of those two keys, used as `--approver` and in records |
| `SERVICE`, `VERSION` | service name and version written into records, `nginx` and `1.31.6` |
| `WORK` | work directory for intermediate files, `e2e-work` |
| `BIN` | where local binaries are built, `e2e-work/bin` |
| `EXE` | `.exe` in Git Bash, empty elsewhere |
| `VENDOR_BIN`, `AUDITOR_BIN`, `USER_BIN` | paths of the local `att-vendor`, `att-auditor` and `att-user` binaries under `BIN` |

## What each script leaves behind

All paths are under `e2e-work/` (gitignored).

| file | written by | read by |
|---|---|---|
| `registry.txt` | flow 1 step 1; flow 2 step 4 | flow 1 steps 3 and 5; flow 2 step 5 |
| `registry-flow2.txt` | flow 2 steps 1 and 3 | flow 2 steps 2 and 5 |
| `auditor-measured.json` (the Auditor's measurements) | flow 1 step 1; flow 2 step 4 | the `audit` command in the same script |
| `auditor-build-config.json` (copy of the build config) | flow 1 step 1 | the `audit` command in the same script |
| `vendor-build.json`, `vendor-build-config.json` | flow 2 step 1 (`vendor-build.json` again in step 3) | `self-register` in the same script |
| `from-auditor/alpine-slim.eif`, `measurements.json` | flow 1 step 1 | uploaded to the Vendor instance for flow 1 step 2 |
| `from-auditor/measurement-record.json` | flow 1 step 1 | flow 1 step 2 (registry address, record key) |
| `vendor-check/doc.cbor`, `parsed.json` | flow 1 step 2 | `compare` in the same script |
| `bin/att-*` | `0-Setup-local-upload.sh` | every script that runs a local `att` command |
| `arch/`, `responder/`, `toolchain/` | the matching check script | the same script |
| `build-env-check/` | `arch-record-check.sh` here, and `build-env-check.sh` on the instance | the same script |
