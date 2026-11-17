# Build-info gaps filled and closed, 2026-11

*(Moved here on 2026-11-17 from `docs/status/build-info-gaps.md`, unchanged.)* The record of the build-info gaps
that were filled or closed in 2026-11: when, by what, and why. What is still missing, and the design questions, are
in [`docs/status/build-info-gaps.md`](../status/build-info-gaps.md). The row numbers are the same in both files,
so a reference such as "#20" or "gap 27" finds its row here, and #2, #15, #36 and #37 are there.

The rule (project lead, 2026-11-11): every piece of information needed to build and run the project, however small,
must be in the repository. This list is the audit of 2026-11-11: what someone with only this repository would be
missing, for each platform. It is kept up to date as gaps are filled.

"Needs" says who can fill a gap: **repo** (derivable from the repository or public sources), **person** (only
someone with the accounts or the original machines knows), **instance** (needs a running instance), **cannot be
filled** (closed: the information no longer exists anywhere, and the row says what replaces it).

## Filled on 2026-11-11

| gap | where it is now |
|---|---|
| OpenZeppelin version: `.gitmodules` without a gitlink in this copy | pinned upstream at `cab19933…` (v5.7.0); `contracts/README.md`, `STRUCTURE.md`. The working copy was a zip extract; a recursive clone has the gitlink |
| `att-ra-responder` bytes changed with every commit (Go's VCS stamp) | `-buildvcs=false` everywhere, refused by `build` otherwise; `docs/build/aws-nitro.md` |
| exact GCP image of the recorded TDX and SEV-SNP guests | `ubuntu-2404-noble-amd64-v20261105`, with disk, Shielded VM and maintenance settings; `docs/build/tdx.md`, `docs/build/sev-snp.md`, "The instance" |
| the SEV-SNP boot change needed for an initrd | step by step, with undo; `docs/build/sev-snp.md` |
| e2e approver addresses hard-coded to the original author's keys | `AUDITOR_ADDR` and `VENDOR_ADDR` are set in `e2e/env.local.sh`; since 2026-11-12 there is no default and the chain scripts stop without them (`need_addr`); `e2e/README.md`, `e2e/env.local.example.sh` |
| `e2e/cloud/` not reachable from the e2e guide; "stop the instances from the console" | linked from `e2e/README.md`, which now names `aws-nitro.sh stop` |
| stale and non-English lines in `docs/status/cvm-port.md` | fixed |

## Filled on 2026-11-16, from the trial

The trial of 2026-11-15 (`docs/results/trial-2026-11-16.md`) followed the TDX and SEV-SNP getting-started procedures in a
clean WSL and on real guests, with the documents frozen. What it found, and where each is now:

| trial step | gap | where it is now |
|---|---|---|
| 2.3 | `att-ra-responder` differed with cgo (`a71bedfc…` native Linux with gcc, `5573b538…` cross-built); nothing set `CGO_ENABLED=0` | `CGO_ENABLED=0` in every build command, `build` refuses a cgo responder, test for it (`e57f02d`); hashes in `docs/build/aws-nitro.md` (`c451611`) |
| 2.3 (found running the suite there) | `TestGit_RepoAccessError` failed with git 2.43 | classifier fixed (`4a1a2bd`) |
| two-guest comparison | `gcp-cvm.sh` made guests from the image family, which moves | `IMAGE=<exact name>`, the resolved name printed, each disk's image in `status` (`32531e8`); "Use the same image as the other side" in both procedures |
| 1.1 | step 1 ran a repository script before step 2 cloned it | git, clone, install script in one step, in that order (`ec09861`, `2ccc681`) |
| 1.3 | the install script ended "Next: …upload.sh", the Nitro script | names the next step of each platform (`443863508d`) |
| 1.4 | Go on `PATH` only in a new interactive shell | said in step 1, with the fix for scripts |
| 1.5, 3.2 | no install command for `gcloud`; `gcloud` and the clone in different environments | per-system commands (the apt ones run in the clean WSL on 2026-11-16, core 2026.11.06); one environment for all; WSL's appended Windows `PATH` |
| 1.6, 11 | no install command for anvil 1.8.0 | `foundryup --install v1.8.0`, or the release zip on Windows |
| 3.1 | the procedure assumes one guest name | "Names" paragraph: replace it, `status` lists them |
| 3.3 | the first ssh to a new guest failed; PuTTY's host key prompt | step 3 |
| 8a | "wait a minute" after the reboot was too short | `uptime`, `/proc/cmdline` or the serial console in step 7; what `--check` says on the old boot in step 8 |
| Opt. 2 | Option 2's base initrd overwrote Option 1's | `base-initrd-v.img` in steps 4 and 6 |
| 13a | `pkill -f` in one remote command line killed its own shell | `[a]tt-ra-responder` pattern on its own line, with the reason (checked: the plain pattern's `bash -c` exited 143, the new one's 0) |
| 13b | TDX: `rmdir /opt/app-verity` before the reboot, while mounted, error hidden | after the reboot, without `2>/dev/null`, as on SEV-SNP |
| stop | `gcp-cvm.sh stop` with two guests failed on Windows (CR in the names) | each cloud script strips the CR its CLI prints; `aws` showed the same, so `aws-nitro.sh` and `aws-snp.sh` too (`fd3353f`) |

**Not filled, and why.**

| trial step | gap | why |
|---|---|---|
| 2.1 | the documented clone URL holds an older `main` | the commits are not pushed; only a push fills it (a person's decision) |
| environment | the trial ran as root (`wsl --install --no-launch` makes no user) | the trial's own setup; a normal WSL install has a user with `sudo`, and the procedures use `sudo` either way |
| Option 2 in WSL | a stand-in base initrd, since WSL has no `/boot` | not a procedure step: the procedures build on the guest |
| two-guest comparison | the recordings' application was used to compare with 2026-11-11 | the trial's own check, not a procedure step |
| 6 | whether `veritysetup format` works as an ordinary user | only tried as root (the procedure says `sudo`); unknown, not a gap in the procedure |

## Filled on 2026-11-16, from the clean trial

The clean trial of 2026-11-16 (`docs/results/trial-2026-11-16-clean.md`) followed Nitro, TDX and SEV-SNP from the
documents alone, at `6a92a1f`, and all three reached the same verdicts. Its findings, and the commit for each, are in
that file's "Afterwards" section. The ones about building and following the procedure:

| finding | gap | where it is now |
|---|---|---|
| F3 | `0-Setup-local-upload.sh` exited 1 with no output unless all five address variables were set | `env.sh` (`36d1775`) |
| F1, F2, F11, F12 | the cloud scripts: an empty `.pem` after a failed create, exit 126 after every create, default names already taken in a shared project, `stop` acting on other people's VMs | `535ff57`, `062b905`; names in `e2e/cloud/README.md` |
| F4 | flow 1 step 4 needs docker, which nothing had started on the Vendor | `7e919cd` |
| F6 (row 8a above) | the serial console cannot tell the new boot from the old | the boot ID before and after, steps 7 and 13 of both procedures (`eae8a25`, `d275e95`) |
| F7, row 1.6 above | the anvil install command only in `tdx.md`; the release archive's checksum file name | `e2e/README.md` and both procedures (`8d7471a`, `eae8a25`) |
| F14 | the procedures' files in the repository root were not ignored | `.gitignore` (`79394467763393f`) |
| — | `fd3353f` (row "stop" above) had dropped `--region` from the AWS scripts | `94ac753` |

## Closed rows of the list of 2026-11-11

The rows of the "Still missing" list that were done or closed, in their original order; the open ones (#2, #15) are in
`docs/status/build-info-gaps.md`.

| # | gap | needs | where it should go |
|---|---|---|---|
| 1 | **Nitro build environment.** Done 2026-11-12 at the baseline: `ami-048b77d5ca32f6d32` (Amazon Linux 2023.12.20261109), kernel 6.12.114-139.210, aws-nitro-enclaves-cli 1.5.0, docker 25.0.18 (rpm) / 25.0.14 (binary), git 2.50.1 installed by the setup script | — | `docs/build/toolchain.md`, "Nitro parent instance" |
| 3 | **forge version.** Done 2026-11-12 as far as the repository allows: forge 1.8.0 with OpenZeppelin `cab19933` regenerates both committed `registry.go` files byte for byte, metadata hash included. Which forge was used originally is still unknown (person) | — | `docs/build/toolchain.md`; both `generate.go` |
| 4 | **Python.** Done 2026-11-12: Python 3, standard library only; 3.14.6 checked in the same regeneration | — | `docs/build/toolchain.md` |
| 5 | **The SEV-SNP test scripts of 2026-10/11** (`vendor/`, `option1-initrd/`, `option2-dm-verity/`, `relay-test/`, `mkcpio.py`, `verity_boot.sh`). **Closed 2026-11-15 as unfillable** (see below) | cannot be filled | — |
| 6 | **The tool versions used with those scripts** (`snpguest`, `gotpm`). **Closed 2026-11-15 as unfillable** (see below). The versions on our own guest are recorded (tpm2-tools 5.6-1build4, squashfs-tools 1:4.6.1-1build1, cryptsetup-bin 2:2.7.0-1ubuntu4.2) | cannot be filled | `docs/build/toolchain.md` |

**Why #5 and #6 are closed.** The 2026-10/11 SEV-SNP scripts and the tool versions used with them cannot be
recovered: the person who wrote them has left the lab and the scripts are not available anywhere. These two gaps are
closed as unfillable rather than pending.

What replaces them: the SEV-SNP work was redone in this repository in 2026-11. The recorded evidence, the measured
values and the procedures in `docs/build/app-fixture-recording.md` are ours and are reproducible from this repository
alone. The one result that still rests only on the old scripts is T10 (evidence from two machines combined);
`docs/build/app-fixture-recording.md`, "Reproducing T10", is the procedure to measure it ourselves.
| 7 | **The Nitro test application.** Done 2026-11-12: the pinned commit `ef5a25a6` of the personal fork is an unmodified upstream `nginx/docker-nginx` commit (an ancestor of upstream `master`; the fork's only own commit after it adds a built EIF and a record, which no script uses). The four scripts now clone upstream at the same commit; nothing to vendor | — | `e2e/README.md`, "Setup, once" |
| 8 | **Fixture READMEs for Nitro.** Written 2026-11-12 with what the documents themselves say: signed timestamps (2026-09-21 and 2026-09-14), PCRs, sha256, nonce source, and that `fixture-my-web`'s PCR1 equals nitro-cli 1.5.0's on x86_64 while the other's does not. Still unknown: the AMI, nitro-cli version and application of each recording | person (who recorded them) | a `README.md` in each testdata folder |
| 9 | **Packing SEV-SNP evidence for `ra-parse`.** Done 2026-11-12: `internal/tee/parse/snp/testdata/pack` joins a recording's three files; checked by packing `gcp-n2d-opt1p-v1` and verifying it with `ra-parse`. Live evidence from `ra-request` needs no packing | — | `internal/tee/parse/snp/testdata/README.md` |
| 10 | **GCP prerequisites.** Done 2026-11-12: Compute Engine API enabled, `default-allow-ssh` present, `xxd` in the stock Ubuntu image, `tpm2-tools` not (installed by hand) | — | `e2e/cloud/README.md`, "GCP prerequisites" |
| 11 | **git on the Nitro instances.** Done 2026-11-12 and run at the baseline: the AMI has no git, and `0-Setup-local-upload.sh` installed git 2.50.1 on both instances | — | `e2e/0-Setup-local-upload.sh` |
| 12 | **arm64 Nitro host.** Done 2026-11-12: `aws-nitro.sh create-arm64` launches one c7g.xlarge from the latest Amazon Linux 2023 arm64 AMI with Nitro Enclaves on (both types checked with `describe-instance-types` in eu-north-1); `status`, `stop`, `start` and `delete` include it | — | `e2e/cloud/aws-nitro.sh`, `e2e/cloud/README.md` |
| 13 | **vsock port, CID, enclave memory and CPUs.** Done 2026-11-12: defaults, flags and where each goes, and how the RA port is opened (since 2026-11-12 only with `launch --open-port-to <CIDR>`; before, `launch` opened it to `0.0.0.0/0` whenever the role allowed) | — | `docs/build/aws-nitro.md`, "Ports, vsock address and enclave resources" and "Opening the RA port" |
| 14 | **Intel SGX Root CA.** Done 2026-11-12: identical to Intel's published `Intel_SGX_Provisioning_Certification_RootCA.cer` (serial and full sha256 fingerprint) | — | `docs/build/tdx.md` |

## Following the documents alone (audit of 2026-11-15)

Before a person follows the documents alone on another computer, from a fresh clone, the documents were read as that
person would read them (an independent read, then spot-checked). **Recorded, not fixed, on purpose:** the trial is
meant to find where a person really gets stuck, and filling the gaps first would make it meaningless. [blocks] means
the person cannot go on; [slows] means they can, after guessing. All rows need **repo** unless marked.

### The path from README.md to a build and a launch

| # | gap |
|---|---|
| 16 | [blocks] **Getting the code and the binaries.** `README.md` (Quick start) points at `../../releases/latest`, but there are no tags and no releases. No clone command is given, and the `--recursive` the contract submodule needs appears only in `docs/status/cvm-port.md` (Phase 0.1). `CONTRIBUTING.md` calls the repository private, so access needs a **person** **Filled 2026-11-15**, apart from access: the README starts from the source (clone, `git submodule update --init contracts/evm/lib/openzeppelin-contracts`, Go version, binaries for your machine and for a Linux instance) and no longer links to releases (`b23fdce`). Doing so found that the tree had no gitlink for the submodule at all, so a clone could not fetch OpenZeppelin; the gitlink is now recorded at v5.7.0 (`0a0ebc0`) and a fresh clone fetches it. `--recursive` also descends into OpenZeppelin's test submodules and failed on a long Windows path, so the README uses the one-submodule command. Access to a private repository still needs a **person** |
| 17 | [blocks] **TDX and SEV-SNP build and launch have no procedure.** `README.md` (the platform list) says their documents cover verification only. The only build and launch commands are the "what waits for a VM" lists in `docs/status/cvm-port.md`, a planning document reached as a plain path, not a link. They do not say how to cross-build `att-vendor` for Linux and copy it to the guest, how to make the `app` folder, or how to make the Option 2 base initrd with the hook. For TDX that procedure exists only in that night's work log (`docs/build/tdx.md` points there) **Filled 2026-11-15**: "Getting started" in `docs/build/tdx.md` (`604d256`) and `docs/build/sev-snp.md` (`701e1dc`), linked from the README: install, clone and build, the guest, its preparation including the Option 2 base initrd with the hooks, the application, build, launch, check, responder, evidence, audit, verify. **Closed 2026-11-16:** run on guests on 2026-11-15 (`docs/results/trial-2026-11-16.md`), from the documents alone in the clean trial, and the Auditor's half on two guests in sessions 2 and 3 (`docs/results/sessions-2026-11-16/`) |
| 18 | [blocks] **No first-time order on TDX or SEV-SNP** (create, install, turn off `GRUB_FORCE_PARTUUID`, build, launch and reboot, `--check`, register, verify from another machine, undo, stop). `docs/build/tdx.md` and `docs/build/sev-snp.md` "From scratch" cover recording one piece of evidence. Undoing what `launch` installs (`/boot/initrd-attestamp.img`, `/etc/grub.d/41_attestamp`, `99-attestamp.cfg`, and for SEV-SNP Option 2 `/var/lib/attestamp`) is not described; only the fixture entries' undo is (`docs/build/app-fixture-recording.md`). Nitro has a complete order, split across `e2e/cloud/README.md` and `e2e/README.md` **Filled 2026-11-15** in the same sections, both options, with step 13 putting back everything `launch` installed and `GRUB_FORCE_PARTUUID` (`604d256`, `701e1dc`) |
| 19 | [slows] **Two parallel paths on SEV-SNP and TDX.** `docs/build/sev-snp.md` and `app-fixture-recording.md` send the reader to the fixture recorders (`mkappinitrd`, `make-grub-entry.sh`), not to `build`/`launch --platform …`, and nothing says which one to use. **Closed 2026-11-16 (status audit):** both now say that `build`/`launch` run an application and the recorders make test fixtures |
| 20 | [blocks] **`e2e/cloud/README.md` creates four SEV-SNP VMs by mistake** *(marked [slows] until 2026-11-17: it costs money, not time)*: `gcp-cvm.sh snp create vendor auditor user y` takes every argument after `create` as a role (`ROLES=("$@")` in `gcp-cvm.sh`), so it makes `attestamp-snp-user` and `attestamp-snp-y` too, each billed **Filled 2026-11-15** (`83bc5a2`): the example creates the default two VMs, and the script refuses any role but `vendor` and `auditor` before calling gcloud (checked with a dry-run copy). Every other example command in the documents was checked; none was misread. *(Not enough, as 2026-11-16 showed: the
role check worked, but the only other guard was a 4-second pause before creating.)* **It happened, 2026-11-16, 14:40
UTC:** checking this gap, the command `PATH="C:/…/fakebin:$PATH" bash e2e/cloud/gcp-cvm.sh snp create`, meant to run
the script against a stand-in `gcloud`, ran the real one (Bash splits `PATH` at the colon of `C:`, so the stand-in
folder was never on it). Run with no terminal to press Ctrl-C in, the pause passed unseen and the script created **two
`n2d-standard-2` SEV-SNP VMs**, `attestamp-snp-vendor` (insert 14:40:12–14:40:22) and `attestamp-snp-auditor`
(14:40:38–14:40:45), each with a 64 GB boot disk. Deleted on the operator's decision with their disks (auto-delete) at
14:45:53–14:46:49 UTC, about six minutes after they were made; the project then listed no instances and no disks.
**Closed 2026-11-17:** `create` now makes VMs only with `--yes`. Without it, it prints how many VMs it would make and
their names and exits 3; `--dry-run` prints the same and calls no `gcloud` at all; the roles are `vendor` and `auditor`,
each at most once; an unknown option stops it. Every check runs before the first `gcloud` call. Checked against a
stand-in `gcloud`, with `PATH` holding only the stand-in's folder and `/usr/bin` so the real one could not be reached
(and the project listed no instances or disks afterwards): `snp create vendor auditor user y` exit 2, 0 `gcloud`
calls; the same with `--dry-run`, exit 2, 0 calls; with `--yes`, exit 2, 0 calls; `create vendor vendor` exit 2, 0
calls; `create --verbose` exit 2, 0 calls; `create --dry-run` the plan (2 VMs and their names), exit 0, 0 calls;
`create` the plan, exit 3, 0 calls; `create --yes` the two `instances create` calls, to the stand-in only. Every
`create` example in the guides, `e2e/cloud/README.md` and `cvm-port.md` now carries `--yes`; the dated records keep the
commands as they ran. *Added 2026-11-17:* the same rules now hold in `aws-nitro.sh` (`create`, `create-arm64`; their
4-second pauses removed) and `aws-snp.sh` (`base`, `mkami`, `vm`, which had no warning at all), and in all three scripts
`--yes`/`--dry-run` given to `status`, `stop`, `start` or `delete` is refused, as are extra words (`gcp-cvm.sh delete
vendor` used to delete every VM of the prefix). Checked in 28 cases against stand-in `aws` and `gcloud` with the real
CLIs off `PATH` (confirmed with `which` before the run): every refusal, every run without `--yes` and every dry run made
0 CLI calls; only the `--yes` runs reached a creating call, on the stand-ins |
| 21 | [slows] **README Quick start.** The Auditor's `audit --registry …` has no `registry-deploy` before it; nothing says how to write a key file (one private key in hex) or where to get Sepolia ETH (`NET=local` avoids both, but Quick start does not mention it); `build-config.json` has to be written by hand, which only `cmd/vendor/README.md` implies **Closed 2026-11-16:** Quick start now has `registry-deploy` before `audit` (it prints the address `--registry` takes), says a key file is one 32-byte hex private key and links "Key files" for making one, names a Sepolia faucet for test ETH and links "Running without a public testnet" for `NET=local`, and says `build-config.json` is written by hand, linking the Vendor guide's example |
| 22 | [slows] **Verifying a CVM.** `att-user verify` defaults to `--format aws-nitro`; no document tells a TDX or SEV-SNP verifier to pass `--format intel-tdx` or `amd-sev-snp-*` *Partly, 2026-11-15:* the getting-started procedures pass `--format`. **Closed 2026-11-16 (status audit):** the User guide (`cmd/user/README.md`), which said "Only `aws-nitro` can be verified", now lists the four formats and says to give `--format` for a guest |
| 23 | [slows] **The CVM `build-config.json`.** Its `tee_info` keys for GCP (`instance_name`, `project_id`, `zone`, `image`; `internal/core/transfer/*/build_config.go`) are not documented; `cmd/vendor/README.md` shows the `aws-nitro` keys only. **Closed 2026-11-16 (status audit):** `cmd/vendor/README.md` lists the CVM keys from `TEEInfoKeys` |
| 24 | [slows] **The Go pin outside a configured session.** `README.md` (Install) does not say that only go1.26.1 builds binaries `build` accepts; `GOTOOLCHAIN` is set only in a local tooling configuration, so it applies in sessions run with that configuration, not in a person's shell. `docs/build/toolchain.md` explains it *Partly, 2026-11-15:* the README says `go version` must print go1.26.1. **Closed 2026-11-16:** the README and `docs/build/toolchain.md` say how to set `GOTOOLCHAIN=go1.26.1` by hand (bash, Windows, per command), no longer pointing to that local settings file |
| 25 | [slows] **`e2e/cloud/README.md` says "follow `docs/status/cvm-port.md`"** for the confidential VMs: a plan, not a procedure. **Closed 2026-11-16 (status audit):** it now points to the getting-started procedures and the Auditor's half |

### What to install (`docs/build/toolchain.md`)

| # | gap |
|---|---|
| 26 | [slows] **Not listed at all:** Git Bash / Git for Windows (the scripts need bash; only "need bash" is said), WSL and Docker Desktop (used on 2026-11-15 for checks that need no instance), winget and Homebrew (used by `0-Setup-local-install.sh`), `sha256sum` (used in `app-fixture-recording.md`) **Closed 2026-11-16:** `docs/build/toolchain.md`, "What to install", names Git Bash from Git for Windows as the bash on Windows (with git, ssh, scp and `sha256sum`), the installers `0-Setup-local-install.sh` uses (apt, dnf, Homebrew, winget), and an "optional" row for WSL 2 and Docker Desktop with what each was used for |
| 27 | [slows] **Listed without a way to install:** anvil (the pinned 1.8.0 has no install command; `getfoundry.sh` from the README installs the latest); aws CLI and gcloud ("by hand"; not on Git Bash's PATH on Windows by default; the Compute Engine API must be enabled, which only a check command in `e2e/cloud/README.md` hints at); forge, Python, openssl, cast (needed only to regenerate the contract binding, not stated). Docker and nitro-cli versions are recorded, not pinned *Partly, 2026-11-16:* anvil 1.8.0 and gcloud now have install commands per system in the TDX and SEV-SNP procedures (step 1); the aws CLI and the rest are as listed. Docker and nitro-cli versions are now recorded for 2026-11-16 too, x86_64 and arm64 (`docs/build/toolchain.md`) **Closed 2026-11-16, except one point:** `toolchain.md` gives a way to install each: anvil 1.8.0 (`foundryup --install v1.8.0`, also in README "Testing"), gcloud and the aws CLI per system (winget on Windows, then a new Git Bash window to get them on `PATH`), enabling the Compute Engine API (`gcloud services enable compute.googleapis.com`, also in `e2e/cloud/README.md`), and says forge and Python are only for regenerating the committed binding and openssl and `cast` only for optional cross-checks. **Still open: docker and nitro-cli are recorded, not pinned.** Pinning them is a change to `build` itself (it installs both with `dnf` and no version), not a document fix, and whether a versioned install stays possible as Amazon updates its repository was not checked. *Investigated 2026-11-17, not implemented; a decision is needed.* The approach tried first, carrying the docker and nitro-cli versions in `build_env` and comparing them in `receive` as OS, architecture and Go are compared, **does not work in the documented order**: the Vendor's `send` runs after its `build`, so the versions could be recorded, but the Auditor's `receive` runs **before** its `build`, and `build` is what installs both (`dnf install -y -q aws-nitro-enclaves-cli … docker`, no version, `internal/setup/build/*/aws/build.go`). On a fresh Auditor instance neither is installed at `receive`, so there is nothing to compare, and `build` then installs whatever the repository gives. What decides those versions instead: Amazon Linux 2023 locks each AMI to one versioned package repository by default ("deterministic upgrades", AWS documentation), so `dnf install` gives the same versions on every instance of the same AMI release until someone changes `releasever`. Our records agree (two instances of one AMI, identical docker and nitro-cli on 2026-11-12 and 2026-11-16); that `dnf install` follows the lock was not tested by changing the release. **Options:** (a) compare the AMI's **release** (`2023.12.20261109`, from the AMI name or `/etc/os-release`; not the AMI ID, which differs per region) in `receive`: `build-config.json` already carries `tee_info.ami_id`, and the Auditor's instance can read its own before anything is installed; (b) record docker and nitro-cli in `build_env` at `send` and compare them in `build`, after its `dnf install` (a new refusal in `build`, which was ruled out); (c) record them only, as a warning at `receive` when the Auditor has them installed already. (a) checks the thing that decides the versions, at a point where both sides know it; none was built. Not checkable here: no Nitro instance was used. *Decided 2026-11-17 (operator): check at two points, (a) and (b) together; implemented the same day, **not closed**.* The Vendor builds before the Auditor, so the Vendor's `send` (after its `build`, on its instance) records three optional `build_env` fields for `aws-nitro`: `ami_release` (from `PRETTY_NAME` in `/etc/os-release`, e.g. `2023.12.20261109`; not the AMI ID, which differs per region), `docker` and `nitro_cli` (`rpm -q` version-release, e.g. `25.0.18-1.amzn2023.0.3`, `1.5.0-0.amzn2023`). The Auditor's `receive` compares the release before anything is installed; that is **a proxy, not a guarantee** (a `dnf update` with another release, `releasever=latest`, or a package installed by hand moves the versions without moving the release; said in the code comments and in `docs/build/aws-nitro.md`). The Auditor's `build --config <the Vendor's build-config.json>` compares the versions themselves right after its `dnf install` and stops before building anything; **that is the real check**. `--ignore-env` turns either into a difference that only warns. *Decided 2026-11-17 (operator):* the three fields are **required** on `aws-nitro` (a config without them is invalid), because a warning would leave a path that skips the check for good, and nobody outside uses the format yet; the CVM formats are not affected. Unit tests: `TestCheckBuildEnvNitro_AMIRelease`, `TestCheckNitroTools`, `TestBuildEnv_NitroFields` (both roles), `TestParseAmazonLinuxRelease`, `TestParseRPMVersions`; on the command line `TestBuildEnv_SendOffAmazonLinuxLeavesNitroFieldsEmpty`, `TestBuildEnv_ReceiveComparesAMIRelease`, `TestBuild_ConfigFlag`. **Not checked on an instance:** whether `rpm`, `/etc/os-release` and the hook after `dnf install` behave on Amazon Linux as written; `e2e/nitro-tools-check.sh` does that on the two instances. Closed only when that script passes; the release comparison alone does not close it. **Closed 2026-11-17:** `e2e/nitro-tools-check.sh` on two instances at `ab714cd` (`docs/results/aws-nitro-2026-11-17.md`): `send` recorded `ami_release` `2023.12.20261112`, `docker` `25.0.18-1.amzn2023.0.3`, `nitro_cli` `1.5.0-0.amzn2023`, what the instance's `/etc/os-release` and `rpm -q` say; `receive` passed the same release, refused another and only warned with `--ignore-env`; `build --config` passed the same versions and refused another docker right after `dnf install`, before building anything. Both checks ran; the proxy is still a proxy, and the version check is what holds |
| 28 | [slows] **`0-Setup-local-install.sh`:** winget may need administrator rights; on Linux it always fetches the `linux-amd64` Go tarball, also on arm64 **Closed 2026-11-16:** administrator rights were already handled (the script checks with `net session` and says to open an administrator window); the Go tarball now follows `uname -m` (`amd64` or `arm64`, anything else stops with a message). Checked: `bash -n`, and the mapping function alone with `uname -m` stubbed to `x86_64`, `aarch64` and `riscv64` (`amd64`, `arm64`, refused). The whole script was not run on an arm64 Linux machine |

### Things outside the repository

| # | gap |
|---|---|
| 29 | [blocks] **Files a fresh clone does not have:** `pem2.pem` (made by `aws-nitro.sh create`, fine), `auditor.key` and `deployer.key` (no instructions; not needed with `NET=local`), `e2e/env.local.sh` (from the example, fine) **Filled 2026-11-15** for the key files (`afe2e42`, `e2e/README.md`, "Key files"): not needed with `NET=local`, needed funded only on a public network, and how to make them; the `openssl` step was checked, the `cast wallet address` step was **not run** (cast is not installed on our machine) |
| 30 | [slows] **The security group follows the first computer.** `aws-nitro.sh create` opens port 22 only to the address of the machine that created the group, and reuses the group later. From another computer SSH times out, and no document says why or what to change (**person**: the account owner changes the rule). **Partly 2026-11-16:** `aws-nitro.sh status` and `start` now say whether this machine's current address is allowed and print the `/32` commands if not (`6f62448`); the change itself is still the account owner's. **Closed 2026-11-17:** what
was missing was a document that says why SSH times out and what to run. `e2e/README.md` (Prerequisites) and
`e2e/cloud/README.md`, "From another computer, or after your address changed", now do, for AWS and for GCP's RA port
(GCP SSH goes through IAP and does not depend on the address). `status` also lists the addresses the group allows now,
prints the `revoke` command for one no longer used, and warns, with the command that removes it, if a rule is open to
`0.0.0.0/0`; it still changes nothing itself and never suggests a wider range than `/32`. Checked against stand-in
`aws` and `curl` (a group with an old `/32` on port 22 and `0.0.0.0/0` on port 5000): both missing ports named with
their `/32` commands, both rules listed, the `0.0.0.0/0` warning printed, 9 read-only calls, nothing changed. Not
checked against a real security group. Changing the group stays with whoever holds the account's rights (**person**) |
| 31 | [slows] **Instructions tied to one VM:** `app-fixture-recording.md` ("`GRUB_FORCE_PARTUUID` is already off on this instance") and `docs/build/sev-snp.md` ("The boot setting changed on `attestamp-snp-vendor`"). On a fresh VM the setting is on. *Partly, 2026-11-15:* both getting-started procedures turn it off on a fresh VM (step 4); the two passages named are as listed (they describe the recording VM) **Closed 2026-11-16:** the `sev-snp.md` section is now "The boot setting: turning `GRUB_FORCE_PARTUUID` off" and starts by saying every new guest has the setting on; `app-fixture-recording.md` says to turn it off and that it was already off only on the recording instance. The four references to the old heading follow it |
| 32 | [slows] **Procedures that live only in the work log of the overnight sessions:** the TDX Option 2 base initrd with the hook (`docs/build/tdx.md` and `app-fixture-recording.md` cite "Overnight 2026-11-12"). **Closed 2026-11-16:** "Getting started" step 4 in `docs/build/tdx.md` has it (since 2026-11-15, run on guests), and the Option 2 section now points there. How the `gcp-c3-opt2m-*` fixtures were made, with the four boots' results, is in `docs/build/app-fixture-recording.md`, "TDX Option 2 mounted at boot" (moved from the work log when it was untracked) |
| 33 | [slows] **Text addressed to particular people or to a local tool rather than to the reader:** "settled with the user" (`tdx.md`), "project lead" (`sev-snp.md`), "a decision for the user" (`app-fixture-recording.md`), "the operator's machine" (`cvm-port.md`); `e2e/cloud/README.md` and `app-fixture-recording.md` address that tool and its rules file **Closed 2026-11-16:** reworded to the reader in `tdx.md` ("decided in the project"), `sev-snp.md` and `runtime-immutability.md` (no "project lead" or "supervising professor"), `app-fixture-recording.md` (two places), `cvm-port.md` (two places, one of them the Phase 0 row that listed the local tooling configuration's files, which are not in the repository) and `test-matrix.md`; `e2e/cloud/README.md` and `app-fixture-recording.md` no longer address that tool or point at its rules file (not in the repository; the rules a contributor needs are in `CONTRIBUTING.md` and `STRUCTURE.md`). The dated result files state when and at which commit each run was made, and name no runner |

### Statements that contradict each other or the code

| # | gap |
|---|---|
| 34 | [slows] **Stale "not implemented" lines after today's build and launch:** the `docs/build/sev-snp.md` status line ("not implemented") against its own "Not done yet" and `cmd/vendor/snp.go`; `sev-snp.md` "Checking" ("no `--verify` for SEV-SNP yet"); the `README.md` platform list ("verification only") against its status section; `cmd/README.md` (build and launch only on Nitro, the responder only inside the enclave) and the three role READMEs, which never mention TDX or SEV-SNP; `STRUCTURE.md` ("tdx and snp verify recorded evidence") against its own table; `docs/build/tdx.md` "Not done yet" naming `setup/build/tdx` as the image maker, now `setup/build/verity`; the `docs/status/cvm-port.md` header "Status (2026-11-11, evening)" *Partly, 2026-11-15:* the README platform list now links the two getting-started procedures. *Partly, 2026-11-16:* the `sev-snp.md` status line and both procedures' "Not done yet" rows now say what ran on 2026-11-15. **Closed 2026-11-16 (status audit, against `docs/status/test-matrix.md`):** `sev-snp.md` "Checking" (it has `--verify` and the Auditor's comparison); the README platform list and "AWS Nitro Enclaves today"; `cmd/README.md` (build, launch and the responder per platform); the User and Vendor guides (formats, CVM `tee_info`); `STRUCTURE.md` (tdx and snp run end to end, with what was run where); `cvm-port.md` (status of 2026-11-16, the launch rows, Phase 3 item 6 and 4b item 5 done) |
| 35 | [slows] **Other contradictions:** `e2e/cloud/README.md` says the `attestamp-ra` firewall rule was "added by hand", but `gcp-cvm.sh` creates it; `toolchain.md` says docker is not used here, but it was on 2026-11-15; `README.md` says the AMI, docker and nitro-cli versions are "not recorded anywhere", but `toolchain.md` records them; `docs/build/tdx.md` "From scratch" creates one VM (`tdx create vendor`) where `e2e/cloud/README.md` creates two; `cmd/vendor/README.md` uses `out/app.eif`, but the EIF is named after `--app-dir`. *Partly, 2026-11-16 (status audit):* the firewall rule's origin is corrected in `e2e/cloud/README.md`; the `toolchain.md` docker line and the README "not recorded anywhere" are no longer there; one VM in "From scratch" against two in the cloud README is no longer a contradiction (the roles are explained); `out/app.eif` is as listed **Closed 2026-11-16:** two of the "no longer there" were wrong and are fixed now: README item 5 still said the versions are "not recorded anywhere" (it now says `build` does not record them and links `toolchain.md`), and `toolchain.md` still said docker is "not used by the project here" (now: optional, and what it was used for on 2026-11-15). In `cmd/vendor/README.md` the `pack` example's `app.eif` was right (named after the docker image), and `build`'s `launch --eif` now names the `--app-dir` directory |

No current procedure depends on the scripts that no longer exist (the T10 relay uses `record-snp -relay-att`).
