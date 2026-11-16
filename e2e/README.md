# End-to-end scripts

These scripts run both flows against real Nitro instances and a public chain, or a local one
([without a public testnet](#running-without-a-public-testnet)). Everything runs from
your own machine, in Git Bash, macOS or Linux; steps that must build or launch inside an instance
connect over SSH and stream the script, so nothing is uploaded except the binaries. Run every
script from the repository root.

For tests that need no Nitro hardware, see `go test ./...` in the main README.

## Prerequisites

- Two Nitro-enabled EC2 instances (Linux), one for the Vendor and one for the Auditor. An optional
  third, arm64 (Graviton), is used only by the architecture comparison. `bash e2e/cloud/aws-nitro.sh create --yes` makes
  the two x86_64 instances; see [`cloud/README.md`](cloud/README.md) for the settings and cost. If the account
  already has the key pair `attestamp-e2e` (made from another machine, so its `.pem` is not on yours), `create` stops
  with `could not create key pair`: give this machine names of its own,
  `KEY_NAME=<name> SG_NAME=<name> TAG=<name> bash e2e/cloud/aws-nitro.sh create --yes`, and the same `TAG=` to `status`,
  `stop`, `start` and `delete`. The instances have no automatic stop; `sudo shutdown -h +360` on each, right after
  `create`, is a safety net if a run is abandoned. **If SSH times out**, from another computer or after your network
  changed: the security group allows only the addresses added to it, starting with the creating machine's. Run
  `bash e2e/cloud/aws-nitro.sh status`; it says whether this machine is allowed and prints the `/32` commands to add it
  ([`cloud/README.md`](cloud/README.md), "From another computer, or after your address changed").
- `git`, an `ssh` client and the pinned Go version on your machine. `0-Setup-local-install.sh`
  installs what is missing.
- On Windows, run the scripts in Git Bash. Git Bash (MSYS) rewrites any argument that starts with `/` (or holds
  `=/…`) into a Windows path before a native program such as `aws.exe` sees it, even inside quotes; the scripts put
  `MSYS_NO_PATHCONV=1` in front of the calls that pass such values (`/dev/xvda`, `/dev/sda1`). Do the same when you
  type such a command yourself.
- The SSH key (`PEM` in `env.sh`, default `pem2.pem`) in the repository root, and two funded key
  files in the repository root: `auditor.key` (signing key, approver in flow 1) and `deployer.key` (deployer key, pays gas,
  signing key and approver in flow 2). Key files are gitignored. Never commit them.
- Test ETH on the chosen network for both keys. A key file holds one private key in hex. Set `AUDITOR_ADDR` and
  `VENDOR_ADDR` (their addresses) in `e2e/env.local.sh`. There is no default: every script that deploys or registers
  stops with an error until both are set. The checks that only build (`arch-*`, `toolchain-pin-check.sh`,
  `build-env-check.sh`, `responder-check.sh`) do not need them. **No ETH?** Run on a local chain instead: see
  [Running without a public testnet](#running-without-a-public-testnet); it needs neither the key files nor the
  addresses.

### Key files

A fresh clone has no `auditor.key` and no `deployer.key`; they are gitignored and must never be committed.

- **With `NET=local` you need neither.** The scripts sign with anvil's development accounts, which every anvil start
  funds ([Running without a public testnet](#running-without-a-public-testnet)). The CVM getting-started procedures do
  the same (`docs/build/tdx.md` and `docs/build/sev-snp.md`, step 11).
- **Only to deploy and register on a public network** (`NET=sepolia`, the default, or `NET=mainnet`) do you need two
  keys of your own, each with a balance on that network:

| file | role | pays gas for |
|---|---|---|
| `auditor.key` | the Auditor's signing key: the approver of flow 1's registry | nothing; it only signs registrations |
| `deployer.key` | deploys every registry, and is the Vendor's signing key and approver in flow 2 | deployment (about 1.1M gas) and flow 2's registrations (about 89k gas each) |

A key file is one line: a 32-byte private key in hex, with or without `0x` (the commands read it with
`--signing-key-file` and `--deployer-key-file`). To make one, in the repository root:

```
openssl rand -hex 32 > auditor.key
openssl rand -hex 32 > deployer.key
```

The addresses, for `AUDITOR_ADDR` and `VENDOR_ADDR` in `e2e/env.local.sh`, come from `cast`, which Foundry installs
together with `anvil`:

```
cast wallet address --private-key 0x$(cat auditor.key)
cast wallet address --private-key 0x$(cat deployer.key)
```

Then send test ETH to both addresses on Sepolia. These files hold private keys: keep them out of every commit, message
and screenshot, and use them for nothing but these tests.

## env.sh and env.local.sh

`e2e/env.sh` holds the shared settings: SSH user and key, chain and RPC, signing and deployer key files, service name and
version, and the helper functions the scripts use. It does not hold instance addresses. Those go in
`e2e/env.local.sh`, which is gitignored: copy `e2e/env.local.example.sh` to `e2e/env.local.sh` and
fill in

```
VENDOR_HOST=<vendor instance public IP or DNS>
AUDITOR_HOST=<auditor instance public IP or DNS>
ARM64_HOST=<arm64 instance, or leave empty>
AUDITOR_ADDR=<address of auditor.key>
VENDOR_ADDR=<address of deployer.key>
```

`env.sh` reads it automatically. A value set in the environment wins over the file, so a one-off run can pass
the addresses on the command line. An instance without a fixed address gets a new public IP each time it is started
(`bash e2e/cloud/aws-nitro.sh status` prints them):

```
VENDOR_HOST=<new vendor IP> AUDITOR_HOST=<new auditor IP> bash e2e/replay-check.sh
```

A script that needs a host that is not set stops with a message telling you to create the file. `ARM64_HOST` may stay empty: the architecture scripts then only
print guidance.

### Choosing the chain

`NET=sepolia` is the default. Mainnet spends real ETH (deployment about 1.1M gas, one registration
about 89k gas) and needs an explicit confirmation:

```
bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh
NET=mainnet CONFIRM_MAINNET=1 bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh
```

Without `CONFIRM_MAINNET=1`, `NET=mainnet` exits at once. `NET=local` is described next. Any other `NET` value is
rejected.

### Running without a public testnet

Both flows run end to end on a local chain, so you can try the whole thing with real enclaves and no ETH at all.
What the flows check (registered means trusted, anything else is refused) does not depend on which chain holds the
registry; the contract is the same bytecode.

1. Install `anvil` 1.8.0 from [Foundry](https://getfoundry.sh), the local chain the Go tests also use. On Linux,
   macOS or WSL:

   ```
   curl -fsSL https://foundry.paradigm.xyz | bash
   source ~/.bashrc
   foundryup --install v1.8.0
   ```

   The release archive without an installer script, and Windows without WSL: `docs/build/tdx.md`, step 1.
2. Start it in a terminal of its own and leave it running until the last script of the flow has finished. A later
   script reads the registry an earlier one deployed, and an anvil that is restarted starts empty:

   ```
   anvil --port 8545
   ```

3. Run the scripts with `NET=local`, for example:

   ```
   NET=local bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh
   NET=local bash e2e/flow1-auditor-registers/2-Vendor-launch-check-record.sh
   ```

   `LOCAL_RPC` points them at another address (default `http://127.0.0.1:8545`).

What changes with `NET=local`:

- **Where the chain must be reachable.** Every chain call (`registry-deploy`, `audit`, `self-register`, `lookup`,
  `compare`, `verify`) runs on your machine; the instances only build and launch. anvil on `127.0.0.1` is therefore
  enough: no port is opened, and the instances never reach the chain.
- **Keys.** The scripts sign with anvil's development accounts 0 (deployer, and the Vendor in flow 2) and 1 (the
  Auditor in flow 1), which every anvil start funds, instead of `auditor.key` and `deployer.key`; the Go tests do the
  same. They are written to `e2e-work/anvil-keys/` and override `AUDITOR_ADDR` and `VENDOR_ADDR` for that run. These
  keys are public knowledge: never use them on a public network.
- **Results.** Every line in `e2e-work/results/*.jsonl` carries `"chain":"local"`, so these runs are never confused
  with runs on Sepolia.
- **Records.** The measurement record handed to the Vendor in flow 1 names `http://127.0.0.1:8545` as its RPC. That
  record is only good on the machine that ran anvil.

A script that needs the chain stops at once with a message if nothing answers at `LOCAL_RPC`.

## Setup, once

```
bash e2e/0-Setup-local-install.sh   # git, ssh, and the pinned Go on your machine
bash e2e/0-Setup-local-upload.sh    # build Linux binaries, upload them to both instances, install git there if missing
```

On Linux the install script adds Go to `PATH` in `~/.bashrc`, which only a new interactive shell reads: open a new
terminal (or `source ~/.bashrc`) before the upload script. A shell that does not read `~/.bashrc` (a script, `bash -c`,
an IDE's runner) needs `export PATH=$PATH:/usr/local/go/bin` first.

**The test application** is nginx's official image source, `https://github.com/nginx/docker-nginx` at commit
`ef5a25a6314e652a2dcfbce6074bef729f4d9639` (2026-09-21), folder `mainline/alpine-slim`. The flow and arch scripts clone
it on the instance and check out that commit. Until 2026-11-12 they cloned a personal fork; the pinned commit is an
unmodified upstream commit (checked: it is an ancestor of upstream `master`), so the files, and the measurement, are the
same from either URL.

Stop everything when you are done (the instances themselves with `bash e2e/cloud/aws-nitro.sh stop`):

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
| `responder-check.sh` | `build --responder` refuses a responder built without `-trimpath`, with version control stamping, or for the wrong architecture |
| `build-env-check.sh` | `receive` rejects a `build-config.json` whose `build_env` does not match the instance, and `--ignore-env` only warns |
| `arch-record-check.sh` | the architecture in records and registrations; sends no transaction |
| `replay-check.sh` | T9: two fresh documents from the running enclave differ, and document 1 against request 2's nonce is refused with exit 6. Needs a running enclave, no chain. Against a TDX or SEV-SNP guest with its responder running (`docs/build/tdx.md` step 9): `TEE_ADDR=<guest-ip>:5000 RESULTS_PLATFORM=intel-tdx NET=local bash e2e/replay-check.sh` (or `amd-sev-snp-initrd`, `amd-sev-snp-verity`); `NET=local` only labels the result lines. It uses `e2e-work/bin/att-user` (`att-user.exe` on Windows), which `0-Setup-local-upload.sh` builds; without the Nitro setup, build it with `CGO_ENABLED=0 go build -trimpath -buildvcs=false -o e2e-work/bin/att-user ./cmd/user` (`-o e2e-work/bin/att-user.exe` on Windows). A SEV-SNP guest on the stock boot fails `own-nonce-a/b` with exit 7 (`docs/build/sev-snp.md` step 9) |
| `debug-mode-check.sh` | T8: the same EIF started with `nitro-cli run-enclave --debug-mode` gives a document that `ra-parse` refuses (exit 7, every PCR is zero); keeps the document as a fixture. No chain |
| `arch-measurement-compare.sh` | the same source measures the same on two x86_64 hosts checked out under two different paths (T1, T3), and differently on arm64 (only with `ARM64_HOST`) |

Intermediate files go in `e2e-work/`. Every verdict is also appended to `e2e-work/results/aws-nitro.jsonl`, one JSON line
each (`result()` in `env.sh`; fields in `docs/status/test-matrix.md`, "Making the results machine-readable").
`arch-measurement-compare.sh` runs the x86_64 pair without `ARM64_HOST` and records the arm64 side as skipped.

## Settings and variables

Set in `e2e/env.sh` unless noted.

| variable | meaning |
|---|---|
| `VENDOR_HOST`, `AUDITOR_HOST`, `ARM64_HOST` | instance addresses, set in `env.local.sh` and empty by default; `need_host` stops a script if a required one is empty |
| `SSH_USER` | login user on the instances, `ec2-user` |
| `PEM` | SSH key file, relative to the repository root, default `pem2.pem` |
| `SSH`, `SCP` | the ssh and scp command lines built from `PEM`; every connection goes through `net_retry` |
| `SSH_TRIES` | attempts per ssh/scp when the connection itself cannot be set up (default 3). Some networks abort a burst of new connections ("kex_exchange_identification … Software caused connection abort"); such an attempt is retried after 3 s, then 6 s. A remote command that fails is never retried, and after the last attempt the failure stands |
| `V`, `A`, `R` | functions that run a command on the Vendor, Auditor or arm64 instance |
| `NET` | `sepolia` (default), `mainnet` (also needs `CONFIRM_MAINNET=1`) or `local` |
| `RPC` | the RPC URL chosen by `NET`: a public endpoint, or `LOCAL_RPC` for `local` |
| `LOCAL_RPC` | the local chain for `NET=local`, default `http://127.0.0.1:8545` |
| `AUDITOR_SIGNING_KEY` | key file `auditor.key`, the approver in flow 1 (anvil account 1 with `NET=local`) |
| `DEPLOYER_KEY`, `VENDOR_SIGNING_KEY` | key file `deployer.key`; pays gas and is the approver in flow 2 (anvil account 0 with `NET=local`) |
| `AUDITOR_ADDR`, `VENDOR_ADDR` | the addresses of those two keys, used as `--approver` and in records; set from env.local.sh, or to the anvil accounts with `NET=local` |
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
