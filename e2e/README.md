# End-to-end scripts

These scripts run both flows against real Nitro instances and a public chain, or a local one
([without a public testnet](#running-without-a-public-testnet)). Everything runs from
your own machine, in Git Bash, macOS or Linux; steps that must build or launch inside an instance
connect over SSH and stream the script, so nothing is uploaded except the binaries. Run every
script from the repository root. Both flows also run on TDX and SEV-SNP guests on GCP:
[Flow 1 on TDX and SEV-SNP](#flow-1-on-tdx-and-sev-snp) and [Flow 2 on TDX and SEV-SNP](#flow-2-on-tdx-and-sev-snp).

For tests that need no Nitro hardware, see `go test -timeout 30m ./...` in the main README.

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

The two addresses are those of your key files ([Key files](#key-files), below); with `NET=local` they are not needed.
`env.sh` reads the file automatically. A value set in the environment wins over the file, so a one-off run can pass
the addresses on the command line. An instance without a fixed address gets a new public IP each time it is started
(`bash e2e/cloud/aws-nitro.sh status` prints them):

```
VENDOR_HOST=<new vendor IP> AUDITOR_HOST=<new auditor IP> bash e2e/replay-check.sh
```

A script that needs a host that is not set stops with a message telling you to create the file. `ARM64_HOST` may stay empty: the architecture scripts then only
print guidance.

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

The addresses, for `AUDITOR_ADDR` and `VENDOR_ADDR` in `e2e/env.local.sh`, come from Foundry's `cast`. `foundryup`
installs it together with `anvil`. On Windows without WSL, take `cast.exe` from `foundry_v1.8.0_win32_amd64.zip`, the
zip `docs/build/tdx.md` step 1 takes `anvil.exe` from (it holds both, with `forge.exe`, `chisel.exe` and `solar.exe`;
checked 2026-11-18):

```
cast wallet address --private-key 0x$(cat auditor.key)
cast wallet address --private-key 0x$(cat deployer.key)
```

Then send test ETH to both addresses on Sepolia. These files hold private keys: keep them out of every commit, message
and screenshot, and use them for nothing but these tests.

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

   The release archive without an installer script, and Windows without WSL: `docs/build/tdx.md`, step 1. On Windows,
   `anvil` is then in `~/.foundry/bin`, which Git Bash does not search: `export PATH="$PATH:$HOME/.foundry/bin"`
   first, or the next step says `anvil: command not found`.
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
- **Records.** The hand-over note (`measurement-record.json`) the Auditor gives the Vendor in flow 1 names `http://127.0.0.1:8545` as its RPC. That
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

## Flow 1 on TDX and SEV-SNP

The same flow on two confidential VMs on GCP, `<PREFIX>-vendor` and `<PREFIX>-auditor` (`PREFIX` default
`attestamp-tdx` or `attestamp-snp`, `ZONE` default `us-central1-a`), made beforehand with
`bash e2e/cloud/gcp-cvm.sh <tdx|snp> create --yes` from one image and machine type. Every script takes the format, and
for TDX the option: `intel-tdx [1|2]`, `amd-sev-snp-initrd`, `amd-sev-snp-verity`. The steps are those of
`docs/build/tdx.md` and `sev-snp.md` ("Getting started", "The Auditor's half"). The chain is `NET` as above, Sepolia by
default, so that a User on another computer reaches it and the transactions show on Etherscan; `NET=local` for
development.

The prerequisites above are Nitro's: this flow needs no EC2 instance, no `pem2.pem` and no `env.local.sh`. It needs
`gcloud` signed in to the project the guests are in (`docs/build/tdx.md`, step 1), the pinned Go and `git` on this
machine (`0-Setup-local-install.sh`), and, for `NET=local`, anvil (below); for Sepolia, the key files and addresses
of "Key files".

| script | who | what it does |
|---|---|---|
| `cvm/0-Setup-guests.sh` | both | checks both guests have the same image and machine type, builds the binaries, copies them, and prepares each guest (an initrd boot, the base initrd, the Option 2 packages; for Option 2 the Auditor gets the Vendor's `base-initrd-v.img`). Leaves what is done as it is, so it can be run before every take |
| `cvm/flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh` | Auditor | builds the application on its guest, installs it, reboots, checks the boot, deploys a registry and runs `audit`. Option `-c 0x...` |
| `cvm/flow1-auditor-registers/2-Vendor-build-launch-check-record.sh` | Vendor | the same build on its guest, then `compare` and `lookup` of its own value against the Auditor's and the registration. Prints the `att-user verify` line for the other computer: trusted |
| `cvm/flow1-auditor-registers/2b-Auditor-audit-vendor.sh` | Auditor | `audit --vendor-tee` with both guests up: the Vendor's guest runs what the Auditor built, exit 0 |
| `cvm/flow1-auditor-registers/4-Vendor-tamper-rebuild-launch.sh` | Vendor | changes one byte of the application, builds, reboots. Prints the `verify` line again: not trusted |
| `cvm/flow1-auditor-registers/4b-Auditor-audit-vendor-tampered.sh` | Auditor | `audit --vendor-tee` again: the Auditor's value is registered and the Vendor's guest runs something else, exit 12 (the pass) |

There is no User script: steps 3 and 5 are the printed `verify` line, typed by hand. Steps 2 and 4 print it ready to
paste: with this machine's `att-user` (`e2e-work/bin/att-user`, `att-user.exe` on Windows, which `0-Setup-guests.sh`
builds), run from the repository root. With `NET=local` that is the only place it works: it names
`--rpc http://127.0.0.1:8545`, and both rehearsals typed it there. With Sepolia the steps also print the line for
another computer, as `./att-user` in the folder that holds it (build it as in the main README, "The binaries"). That
computer's address must be allowed on port 5000 by the firewall rule `attestamp-ra` (`e2e/cloud/README.md`, "From
another computer, or after your address changed").

- **Commands on a guest** are a script file (`cvm/guest/guest-step.sh`) copied with `gcloud compute scp` and run with
  `gcloud compute ssh --command`; nothing is piped into ssh, whose handling of piped input through PuTTY on Windows was
  never checked. SSH goes through IAP (`IAP=0` for the public address).
- **The reboot** is followed by comparing `/proc/sys/kernel/random/boot_id` before and after, polled every
  `REBOOT_POLL` seconds (15) for up to `REBOOT_TIMEOUT` (600); past that the step fails and says what it last saw. The
  serial console is not used (`docs/results/trial-2026-11-16-clean.md`, F6).
- **Registries.** `audit` registers as part of its run, and a value already on a registry stops it at exit 5. So 2b
  and 4b deploy a registry of their own each time (`-c 0x...` for a given one); step 1's registry is the one the
  `verify` line names. Files go to `e2e-work/cvm/<format>/` (`intel-tdx-opt1` or `-opt2` for TDX), results to
  `e2e-work/results/<format>.jsonl`.
- **Checked without a guest** by `cvm/check-flow.sh` (also run by `go test ./cmd`, `TestCVMFlowScripts`): stand-in
  gcloud, go, curl and att; the arguments, what goes to which guest, the boot_id wait and its timeout, the audit and
  lookup calls, the exit codes 2b and 4b accept, the printed `verify` line, and the guest-side script's launch and build
  flags per format; the same for flow 2 below. **Not checked without a guest:** gcloud ssh and scp through IAP from Git Bash, the build, the GRUB
  entry and the reboot, `launch --check`, the responder, and the chain. On guests the flow ran for `intel-tdx`,
  Option 1 and Option 2 (`docs/results/cvm-flow-rehearsal-2026-11-18.md`), and for `amd-sev-snp-verity` and
  `amd-sev-snp-initrd` (`docs/results/cvm-flow-rehearsal-2026-11-19.md`), with `NET=local`, every step passing, with
  the time of each step; Sepolia has not run yet.

### Before and during a CVM run

What those runs needed that the table above does not say (R1 to R9 are the findings of those two records):

- **anvil, with `NET=local`.** Start it before step 1 and leave it running until 4b has finished, through the pauses at
  steps 3 and 5: the `verify` line names step 1's registry, and a restarted anvil starts empty. The scripts do not
  start it; steps 1, 2b and 4b stop at once if nothing answers at `LOCAL_RPC`, and step 2 fails at its `lookup`.
  Typing `anvil` needs it on `PATH`. On Windows,
  `anvil.exe` usually sits in `~/.foundry/bin`, where `go test` finds it without `PATH`, but Git Bash does not look
  there and says `anvil: command not found`: run `export PATH="$PATH:$HOME/.foundry/bin"` first (add it to
  `~/.bashrc` to keep it), then `anvil --version`.
- **The firewall rule `attestamp-ra`.** Steps 1, 2, 2b and 4b reach port 5000 of the guests from the machine running
  the flow, and the `verify` line from the machine typing it. The rule allows only the addresses in it and is never
  updated by the scripts, so after your address changes those calls time out with no other message. How to compare
  your address with the rule and add it: `e2e/cloud/README.md`, "From another computer, or after your address
  changed".
- **The guests stop themselves after four hours.** `prepare` and `check-serve` set `sudo shutdown -h +240` on the guest
  (a reboot cancels it, so it is set again after each one). A flow left waiting at step 3 or 5 for longer finds both
  guests `TERMINATED` (2026-11-19, R6). To carry on: start them (`bash e2e/cloud/gcp-cvm.sh <tdx|snp> start`). They
  boot the installed entry again, and only the responder has to be restarted. Run `check-serve` alone on each, with no
  build or install: `gcloud compute ssh <PREFIX>-vendor --zone us-central1-a --tunnel-through-iap --command 'bash
  attestamp-guest-step.sh check-serve vendor <format> <option>'`, and the same with `auditor`. The option is 1 or 2,
  2 for `amd-sev-snp-verity`. It prints the values; they must be the ones from before the stop (they were, on
  2026-11-19).
- **A started guest can have a new public address.** The scripts read the addresses anew each time (`guest_ip`), but a
  `verify` line printed before the stop still names the old one, and so does `e2e-work/cvm/<format>/verify-command.txt`.
  Take the Vendor's new `EXTERNAL_IP` from `bash e2e/cloud/gcp-cvm.sh <tdx|snp> status`, put it in the line, and keep
  the registry. Check the firewall rule again from the machine that types it.
- **How long it takes.** On 2026-11-18 and 11-19 setup took 184 to 1207 s, and steps 1, 2 and 4 took 266 to 377 s
  each. Most of that time went to the reboot (111 to 161 s until the new boot answered) and to `gcloud` through IAP
  (15 to 20 s per call). 2b and 4b took 16 to 20 s. Of setup's 1207 s, the Vendor's Option 2 `prepare` took 920 s,
  for reasons not found (R7).
- **gcloud on Windows can crash after a command has finished on the guest** (`Segmentation fault`, exit 139; R3 in
  `docs/results/cvm-flow-rehearsal-2026-11-18.md`). The scripts go by the exit code the guest recorded and are not
  affected. For a command typed by hand, read its output rather than the exit code.
- **Running setup again** is safe also while a responder runs: files are copied under a temporary name and renamed
  into place (2026-11-19, R8, fixed in `df0c426`). The running responder keeps its old file until `check-serve`
  restarts it. An Option 1 run after an Option 2 run on the same guests works too: the Option 2 `build` runs as root,
  and the guest-side script gives `out/` back to the user afterwards (R9, fixed in `1e4cfc2`).

## Flow 2 on TDX and SEV-SNP

The Vendor registers its own guest, as in [Flow 2](#flow-2-the-vendor-registers-its-own-enclave), on the same two
guests as flow 1, with the same arguments, setup (`cvm/0-Setup-guests.sh`) and chain. The Vendor's registry has the
Vendor as its approver (`VENDOR_ADDR`, `deployer.key`; anvil account 0 with `NET=local`), the Auditor's has the
Auditor. A registry's approver is fixed when it is deployed (`contracts/README.md`), so these are two contracts.

| script | who | what it does |
|---|---|---|
| `cvm/flow2-vendor-self-registers/1-Vendor-build-launch-self-register.sh` | Vendor | builds the application on its guest, installs it, reboots, checks the boot, deploys a registry of its own and runs `self-register`. Prints the `verify` line for step 2: trusted, on the Vendor's word alone. Option `-c 0x...` |
| `cvm/flow2-vendor-self-registers/3-Vendor-tamper-rebuild-self-register.sh` | Vendor | changes one byte of the application, builds, reboots, and self-registers that value on a new registry of its own. Option `-c 0x...` |
| `cvm/flow2-vendor-self-registers/4-Auditor-audit-vendor.sh` | Auditor | `audit --vendor-tee` with the Auditor's own value, on a registry of its own: registered, and the Vendor's guest runs something else, exit 12 (the pass). Prints the `verify` line for step 5 with both registries: not trusted. Option `-c 0x...` |

Steps 2 and 5 are the printed `verify` lines, typed by hand as in flow 1. Step 4 needs the Auditor's own value, which
`cvm/flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh` writes after building and booting on the Auditor's
guest (as Nitro's flow 2 step 4 needs flow 1 step 1); run it with the same arguments any time before step 4. Without
it, step 4 stops and names it.

- **Why step 3 deploys a new registry.** Step 1's registry would accept the changed value (another value has another
  record key), so the contract does not require it. But a new one leaves step 1's registry vouching only for the
  original build, and a second run of step 3 does not stop at exit 5 on a value already registered there, since the
  guest build gives the same value every time. The hand runs of 2026-11-16 used a registry of the Vendor's for the
  changed value too. Step 4 deploys the Auditor's registry for the exit-5 reason of 2b and 4b.
- **Files** go to flow 1's `e2e-work/cvm/<format>/`, under names of their own: `flow2-vendor-registry.txt` (step 1),
  `flow2-vendor-registry-tampered.txt` (step 3) and `flow2-auditor-registry.txt` (step 4), and the Vendor's values in
  `flow2-vendor-measurements.json` and `flow2-vendor-tampered-measurements.json`. The result lines are T6.
- **"Before and during a CVM run" applies as it stands**: with `NET=local`, anvil keeps running from step 1 until step
  5, whose line names the registries of steps 3 and 4 (flow 1's step 1 needs a chain too, when it runs); the firewall
  rule; the four-hour stop; a new address after a start.
- **Checked without a guest** by `cvm/check-flow.sh`, as flow 1. **Not yet run on a guest** (written 2026-11-20). What
  the steps do ran by hand on 2026-11-16, the changed half only (steps 3 to 5), on `intel-tdx` and
  `amd-sev-snp-initrd`: `docs/results/sessions-2026-11-16/intel-tdx.md` and `amd-sev-snp.md`, "T6".

## Checks outside the two flows

| script | what it checks |
|---|---|
| `toolchain-pin-check.sh` | an att binary built with another Go patch version refuses to `build` |
| `responder-check.sh` | `build --responder` refuses a responder built without `-trimpath`, with version control stamping, with a Go build ID (no `-ldflags=-buildid=`), or for the wrong architecture |
| `build-env-check.sh` | `receive` rejects a `build-config.json` whose `build_env` does not match the instance, and `--ignore-env` only warns |
| `nitro-tools-check.sh` | gap 27 (`docs/results/build-info-gaps-2026-11.md`): `send` on the Vendor records the Amazon Linux release and the docker and nitro-cli versions; on the Auditor, `receive` refuses another release (`--ignore-env` warns) and `build --config` passes the Vendor's versions and refuses another docker before building anything. Needs both instances, after `0-Setup-local-upload.sh` |
| `arch-record-check.sh` | the architecture in records and registrations; sends no transaction |
| `replay-check.sh` | T9: two fresh documents from the running enclave differ, and document 1 against request 2's nonce is refused with exit 6. Needs a running enclave, no chain. Against a TDX or SEV-SNP guest with its responder running (`docs/build/tdx.md` step 9): `TEE_ADDR=<guest-ip>:5000 RESULTS_PLATFORM=intel-tdx NET=local bash e2e/replay-check.sh` (or `amd-sev-snp-initrd`, `amd-sev-snp-verity`); `NET=local` only labels the result lines. It uses `e2e-work/bin/att-user` (`att-user.exe` on Windows), which `0-Setup-local-upload.sh` builds; without the Nitro setup, build it with `CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o e2e-work/bin/att-user ./cmd/user` (`-o e2e-work/bin/att-user.exe` on Windows). A SEV-SNP guest on the stock boot fails `own-nonce-a/b` with exit 7 (`docs/build/sev-snp.md` step 9) |
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
| `SSH_TRIES` | attempts per ssh/scp when the connection itself cannot be set up (default 4). Some networks abort or reset a burst of new connections ("kex_exchange_identification … Software caused connection abort" or "Connection reset by peer"); such an attempt is retried after 10 s, 20 s, then 40 s (3 s and 6 s were not enough on one network; 10 s was, measured once on 2026-11-17). A remote command that fails is never retried, and after the last attempt the failure stands |
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
| `from-auditor/measurement-record.json` | flow 1 step 1 | flow 1 step 2 (registry address, record key). A hand-over note the script writes in its own shape, not the `attestamp.measurement-record/1` file `att-auditor send` writes; no `att` command reads it |
| `vendor-check/doc.cbor`, `parsed.json` | flow 1 step 2 | `compare` in the same script |
| `bin/att-*` | `0-Setup-local-upload.sh` | every script that runs a local `att` command |
| `arch/`, `responder/`, `toolchain/` | the matching check script | the same script |
| `build-env-check/` | `arch-record-check.sh` here, and `build-env-check.sh` on the instance | the same script |
