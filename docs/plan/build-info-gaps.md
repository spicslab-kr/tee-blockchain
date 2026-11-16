# Is everything needed to build in the repository?

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
| stale and non-English lines in `docs/plan/cvm-port.md` | fixed |

## Filled on 2026-11-16, from the trial

The trial of 2026-11-15 (`docs/plan/trial-2026-11-16.md`) followed the TDX and SEV-SNP getting-started procedures in a
clean WSL and on real guests, with the documents frozen. What it found, and where each is now:

| trial step | gap | where it is now |
|---|---|---|
| 2.3 | `att-ra-responder` differed with cgo (`a71bedfc…` native Linux with gcc, `5573b538…` cross-built); nothing set `CGO_ENABLED=0` | `CGO_ENABLED=0` in every build command, `build` refuses a cgo responder, test for it (`ade66e6`); hashes in `docs/build/aws-nitro.md` (`d8475f3`) |
| 2.3 (found running the suite there) | `TestGit_RepoAccessError` failed with git 2.43 | classifier fixed (`d6b97e9`) |
| two-guest comparison | `gcp-cvm.sh` made guests from the image family, which moves | `IMAGE=<exact name>`, the resolved name printed, each disk's image in `status` (`3520a56`); "Use the same image as the other side" in both procedures |
| 1.1 | step 1 ran a repository script before step 2 cloned it | git, clone, install script in one step, in that order (`efef3e7`, `025813c`) |
| 1.3 | the install script ended "Next: …upload.sh", the Nitro script | names the next step of each platform (`0b5f822`) |
| 1.4 | Go on `PATH` only in a new interactive shell | said in step 1, with the fix for scripts |
| 1.5, 3.2 | no install command for `gcloud`; `gcloud` and the clone in different environments | per-system commands (the apt ones run in the clean WSL on 2026-11-16, core 2026.11.06); one environment for all; WSL's appended Windows `PATH` |
| 1.6, 11 | no install command for anvil 1.8.0 | `foundryup --install v1.8.0`, or the release zip on Windows |
| 3.1 | the procedure assumes one guest name | "Names" paragraph: replace it, `status` lists them |
| 3.3 | the first ssh to a new guest failed; PuTTY's host key prompt | step 3 |
| 8a | "wait a minute" after the reboot was too short | `uptime`, `/proc/cmdline` or the serial console in step 7; what `--check` says on the old boot in step 8 |
| Opt. 2 | Option 2's base initrd overwrote Option 1's | `base-initrd-v.img` in steps 4 and 6 |
| 13a | `pkill -f` in one remote command line killed its own shell | `[a]tt-ra-responder` pattern on its own line, with the reason (checked: the plain pattern's `bash -c` exited 143, the new one's 0) |
| 13b | TDX: `rmdir /opt/app-verity` before the reboot, while mounted, error hidden | after the reboot, without `2>/dev/null`, as on SEV-SNP |
| stop | `gcp-cvm.sh stop` with two guests failed on Windows (CR in the names) | each cloud script strips the CR its CLI prints; `aws` showed the same, so `aws-nitro.sh` and `aws-snp.sh` too (`1af3f87`) |

**Not filled, and why.**

| trial step | gap | why |
|---|---|---|
| 2.1 | the documented clone URL holds an older `main` | the commits are not pushed; only a push fills it (a person's decision) |
| environment | the trial ran as root (`wsl --install --no-launch` makes no user) | the trial's own setup; a normal WSL install has a user with `sudo`, and the procedures use `sudo` either way |
| Option 2 in WSL | a stand-in base initrd, since WSL has no `/boot` | not a procedure step: the procedures build on the guest |
| two-guest comparison | the recordings' application was used to compare with 2026-11-11 | the trial's own check, not a procedure step |
| 6 | whether `veritysetup format` works as an ordinary user | only tried as root (the procedure says `sudo`); unknown, not a gap in the procedure |

## Filled on 2026-11-16, from the clean trial

The clean trial of 2026-11-16 (`docs/plan/trial-2026-11-16-clean.md`) followed Nitro, TDX and SEV-SNP from the
documents alone, at `e29d028`, and all three reached the same verdicts. Its findings, and the commit for each, are in
that file's "Afterwards" section. The ones about building and following the procedure:

| finding | gap | where it is now |
|---|---|---|
| F3 | `0-Setup-local-upload.sh` exited 1 with no output unless all five address variables were set | `env.sh` (`593a38f`) |
| F1, F2, F11, F12 | the cloud scripts: an empty `.pem` after a failed create, exit 126 after every create, default names already taken in a shared project, `stop` acting on other people's VMs | `ee00116`, `b26465d`; names in `e2e/cloud/README.md` |
| F4 | flow 1 step 4 needs docker, which nothing had started on the Vendor | `948e73c` |
| F6 (row 8a above) | the serial console cannot tell the new boot from the old | the boot ID before and after, steps 7 and 13 of both procedures (`9c0073f`, `1bd1ede`) |
| F7, row 1.6 above | the anvil install command only in `tdx.md`; the release archive's checksum file name | `e2e/README.md` and both procedures (`85ea837`, `9c0073f`) |
| F14 | the procedures' files in the repository root were not ignored | `.gitignore` (`70b271f`) |
| — | `1af3f87` (row "stop" above) had dropped `--region` from the AWS scripts | `ac9635f` |

## Still missing

Most important first.

| # | gap | needs | where it should go |
|---|---|---|---|
| 1 | **Nitro build environment.** Done 2026-11-12 at the baseline: `ami-048b77d5ca32f6d32` (Amazon Linux 2023.12.20261109), kernel 6.12.114-139.210, aws-nitro-enclaves-cli 1.5.0, docker 25.0.18 (rpm) / 25.0.14 (binary), git 2.50.1 installed by the setup script | — | `docs/build/toolchain.md`, "Nitro parent instance" |
| 2 | **Reference results.** The part without a chain done 2026-11-12: the test application's PCR0/1/2 and every verdict of the chain-free checks. **PCR0 and PCR2 there are values from before `fc105b0`** (2026-11-15), which empties the build day in `/etc/shadow` and so changes them; they were not re-measured, and the current reference PCR0/PCR2 are unknown until the baseline is run again, at `ade66e6` (`CGO_ENABLED=0`, 2026-11-16) or later. Chain run done on a local chain 2026-11-15 (`docs/results/aws-nitro-2026-11-15-local-chain.md`), also before `fc105b0`. Still missing: the registry addresses and transactions of a chain run (needs Sepolia ETH), and the arm64 values | ETH; an arm64 instance | `docs/results/aws-nitro-2026-11-12.md` and `.jsonl` |
| 3 | **forge version.** Done 2026-11-12 as far as the repository allows: forge 1.8.0 with OpenZeppelin `cab19933` regenerates both committed `registry.go` files byte for byte, metadata hash included. Which forge was used originally is still unknown (person) | — | `docs/build/toolchain.md`; both `generate.go` |
| 4 | **Python.** Done 2026-11-12: Python 3, standard library only; 3.14.6 checked in the same regeneration | — | `docs/build/toolchain.md` |
| 5 | **The SEV-SNP test scripts of 2026-10/11** (`vendor/`, `option1-initrd/`, `option2-dm-verity/`, `relay-test/`, `mkcpio.py`, `verity_boot.sh`). **Closed 2026-11-15 as unfillable** (see below) | cannot be filled | — |
| 6 | **The tool versions used with those scripts** (`snpguest`, `gotpm`). **Closed 2026-11-15 as unfillable** (see below). The versions on our own guest are recorded (tpm2-tools 5.6-1build4, squashfs-tools 1:4.6.1-1build1, cryptsetup-bin 2:2.7.0-1ubuntu4.2) | cannot be filled | `docs/build/toolchain.md` |

**Why #5 and #6 are closed.** The 2026-10/11 SEV-SNP scripts and the tool versions used with them cannot be
recovered: the person who wrote them has left the lab and the scripts are not available anywhere. These two gaps are
closed as unfillable rather than pending.

What replaces them: the SEV-SNP work was redone in this repository in 2026-11. The recorded evidence, the measured
values and the procedures in `docs/plan/app-fixture-recording.md` are ours and are reproducible from this repository
alone. The one result that still rests only on the old scripts is T10 (evidence from two machines combined);
`docs/plan/app-fixture-recording.md`, "Reproducing T10", is the procedure to measure it ourselves.
| 7 | **The Nitro test application.** Done 2026-11-12: the pinned commit `ef5a25a6` of the personal fork is an unmodified upstream `nginx/docker-nginx` commit (an ancestor of upstream `master`; the fork's only own commit after it adds a built EIF and a record, which no script uses). The four scripts now clone upstream at the same commit; nothing to vendor | — | `e2e/README.md`, "Setup, once" |
| 8 | **Fixture READMEs for Nitro.** Written 2026-11-12 with what the documents themselves say: signed timestamps (2026-09-21 and 2026-09-14), PCRs, sha256, nonce source, and that `fixture-my-web`'s PCR1 equals nitro-cli 1.5.0's on x86_64 while the other's does not. Still unknown: the AMI, nitro-cli version and application of each recording | person (who recorded them) | a `README.md` in each testdata folder |
| 9 | **Packing SEV-SNP evidence for `ra-parse`.** Done 2026-11-12: `internal/tee/parse/snp/testdata/pack` joins a recording's three files; checked by packing `gcp-n2d-opt1p-v1` and verifying it with `ra-parse`. Live evidence from `ra-request` needs no packing | — | `internal/tee/parse/snp/testdata/README.md` |
| 10 | **GCP prerequisites.** Done 2026-11-12: Compute Engine API enabled, `default-allow-ssh` present, `xxd` in the stock Ubuntu image, `tpm2-tools` not (installed by hand) | — | `e2e/cloud/README.md`, "GCP prerequisites" |
| 11 | **git on the Nitro instances.** Done 2026-11-12 and run at the baseline: the AMI has no git, and `0-Setup-local-upload.sh` installed git 2.50.1 on both instances | — | `e2e/0-Setup-local-upload.sh` |
| 12 | **arm64 Nitro host.** Done 2026-11-12: `aws-nitro.sh create-arm64` launches one c7g.xlarge from the latest Amazon Linux 2023 arm64 AMI with Nitro Enclaves on (both types checked with `describe-instance-types` in eu-north-1); `status`, `stop`, `start` and `delete` include it | — | `e2e/cloud/aws-nitro.sh`, `e2e/cloud/README.md` |
| 13 | **vsock port, CID, enclave memory and CPUs.** Done 2026-11-12: defaults, flags and where each goes, and how the RA port is opened (since 2026-11-12 only with `launch --open-port-to <CIDR>`; before, `launch` opened it to `0.0.0.0/0` whenever the role allowed) | — | `docs/build/aws-nitro.md`, "Ports, vsock address and enclave resources" and "Opening the RA port" |
| 14 | **Intel SGX Root CA.** Done 2026-11-12: identical to Intel's published `Intel_SGX_Provisioning_Certification_RootCA.cer` (serial and full sha256 fingerprint) | — | `docs/build/tdx.md` |
| 15 | **Identifiers in committed files.** **Checked again 2026-11-16, and the short summary "the tracked files are clean apart from one EC2 instance ID; the project ID is only in history before `3dbb04f`" is wrong.** `3dbb04f` removed the project ID from the night log's prose only. In the **current tree**: (a) the eight SEV-SNP `*-att.pb` fixtures (added in `4bfdb53`, `3c0ef5e`, `eb9a7a2`) carry the GCP **project ID**, the project number, the zone, the instance name `attestamp-snp-vendor` and its numeric **instance ID**, inside the Google-signed AK certificate; (b) **two** different EC2 instance IDs, one in the signed `module_id` of `internal/tee/parse/aws/testdata/fixture-my-web.cbor` (also written out as text in `internal/tee/parse/aws/measure_test.go`) and one in `internal/tee/request/testdata/nitro-attestation.cbor`. Found with `git grep -a`; the other matches of the instance-ID pattern are public AMI IDs. In **history** only: the project ID in `docs/plan/night-log.md` before `3dbb04f`, and the original author's two Sepolia addresses. **Before publishing, each needs a decision:** re-record from a project and instances whose identifiers may be public, drop the fixtures and their tests, keep the repository private, or accept them; and for history, rewrite it or accept it. Not added since: the T10 recordings of 2026-11-16 carry two more instances and were deliberately not committed. Earlier text: done 2026-11-12: the project ID and every local path were removed from `docs/plan/night-log.md`. The project ID stays inside the eight SEV-SNP `*-att.pb` fixtures, where Google signed it; `internal/tee/parse/snp/testdata/README.md` now says so at the top of its identifiers section. Still for a person to decide: publishing those fixtures; the EC2 instance ID inside the Nitro fixture (also signed, and asserted by `internal/tee/parse/aws/measure_test.go`), and a second, different EC2 instance ID in the signed `module_id` of `internal/tee/request/testdata/nitro-attestation.cbor` (found 2026-11-12, `docs/plan/audit-2026-11-12.md`); and the original author's two Sepolia addresses, which `e2e/env.sh` no longer uses (removed 2026-11-12; they remain in git history) | person (publication decision) | — |

## Following the documents alone (audit of 2026-11-15)

Before a person follows the documents alone on another computer, from a fresh clone, the documents were read as that
person would read them (an independent read, then spot-checked). **Recorded, not fixed, on purpose:** the trial is
meant to find where a person really gets stuck, and filling the gaps first would make it meaningless. [blocks] means
the person cannot go on; [slows] means they can, after guessing. All rows need **repo** unless marked.

### The path from README.md to a build and a launch

| # | gap |
|---|---|
| 16 | [blocks] **Getting the code and the binaries.** `README.md` (Quick start) points at `../../releases/latest`, but there are no tags and no releases. No clone command is given, and the `--recursive` the contract submodule needs appears only in `docs/plan/cvm-port.md` (Phase 0.1). `CONTRIBUTING.md` calls the repository private, so access needs a **person** **Filled 2026-11-15**, apart from access: the README starts from the source (clone, `git submodule update --init contracts/evm/lib/openzeppelin-contracts`, Go version, binaries for your machine and for a Linux instance) and no longer links to releases (`7007fb2`). Doing so found that the tree had no gitlink for the submodule at all, so a clone could not fetch OpenZeppelin; the gitlink is now recorded at v5.7.0 (`50fa441`) and a fresh clone fetches it. `--recursive` also descends into OpenZeppelin's test submodules and failed on a long Windows path, so the README uses the one-submodule command. Access to a private repository still needs a **person** |
| 17 | [blocks] **TDX and SEV-SNP build and launch have no procedure.** `README.md` (the platform list) says their documents cover verification only. The only build and launch commands are the "what waits for a VM" lists in `docs/plan/cvm-port.md`, a planning document reached as a plain path, not a link. They do not say how to cross-build `att-vendor` for Linux and copy it to the guest, how to make the `app` folder, or how to make the Option 2 base initrd with the hook. For TDX that procedure exists only in the night log (`docs/build/tdx.md` points there) **Filled 2026-11-15**: "Getting started" in `docs/build/tdx.md` (`3a0be09`) and `docs/build/sev-snp.md` (`a52e842`), linked from the README: install, clone and build, the guest, its preparation including the Option 2 base initrd with the hooks, the application, build, launch, check, responder, evidence, audit, verify. Not yet run on a guest |
| 18 | [blocks] **No first-time order on TDX or SEV-SNP** (create, install, turn off `GRUB_FORCE_PARTUUID`, build, launch and reboot, `--check`, register, verify from another machine, undo, stop). `docs/build/tdx.md` and `docs/build/sev-snp.md` "From scratch" cover recording one piece of evidence. Undoing what `launch` installs (`/boot/initrd-attestamp.img`, `/etc/grub.d/41_attestamp`, `99-attestamp.cfg`, and for SEV-SNP Option 2 `/var/lib/attestamp`) is not described; only the fixture entries' undo is (`docs/plan/app-fixture-recording.md`). Nitro has a complete order, split across `e2e/cloud/README.md` and `e2e/README.md` **Filled 2026-11-15** in the same sections, both options, with step 13 putting back everything `launch` installed and `GRUB_FORCE_PARTUUID` (`3a0be09`, `a52e842`) |
| 19 | [slows] **Two parallel paths on SEV-SNP and TDX.** `docs/build/sev-snp.md` and `app-fixture-recording.md` send the reader to the fixture recorders (`mkappinitrd`, `make-grub-entry.sh`), not to `build`/`launch --platform …`, and nothing says which one to use |
| 20 | [slows] **`e2e/cloud/README.md` creates four SEV-SNP VMs by mistake**: `gcp-cvm.sh snp create vendor auditor user y` takes every argument after `create` as a role (`ROLES=("$@")` in `gcp-cvm.sh`), so it makes `attestamp-snp-user` and `attestamp-snp-y` too, each billed **Filled 2026-11-15** (`19ad87c`): the example creates the default two VMs, and the script refuses any role but `vendor` and `auditor` before calling gcloud (checked with a dry-run copy). Every other example command in the documents was checked; none was misread |
| 21 | [slows] **README Quick start.** The Auditor's `audit --registry …` has no `registry-deploy` before it; nothing says how to write a key file (one private key in hex) or where to get Sepolia ETH (`NET=local` avoids both, but Quick start does not mention it); `build-config.json` has to be written by hand, which only `cmd/vendor/README.md` implies |
| 22 | [slows] **Verifying a CVM.** `att-user verify` defaults to `--format aws-nitro`; no document tells a TDX or SEV-SNP verifier to pass `--format intel-tdx` or `amd-sev-snp-*` *Partly, 2026-11-15:* the getting-started procedures pass `--format`; the README Quick start and the User guide still do not say it |
| 23 | [slows] **The CVM `build-config.json`.** Its `tee_info` keys for GCP (`instance_name`, `project_id`, `zone`, `image`; `internal/core/transfer/*/build_config.go`) are not documented; `cmd/vendor/README.md` shows the `aws-nitro` keys only |
| 24 | [slows] **The Go pin outside Claude.** `README.md` (Install) does not say that only go1.26.1 builds binaries `build` accepts; `GOTOOLCHAIN` is set only in `.claude/settings.json`, so it applies in Claude sessions, not in a person's shell. `docs/build/toolchain.md` explains it *Partly, 2026-11-15:* the README says `go version` must print go1.26.1; `GOTOOLCHAIN` is still not mentioned there |
| 25 | [slows] **`e2e/cloud/README.md` says "follow `docs/plan/cvm-port.md`"** for the confidential VMs: a plan, not a procedure |

### What to install (`docs/build/toolchain.md`)

| # | gap |
|---|---|
| 26 | [slows] **Not listed at all:** Git Bash / Git for Windows (the scripts need bash; only "need bash" is said), WSL and Docker Desktop (used on 2026-11-15 for checks that need no instance), winget and Homebrew (used by `0-Setup-local-install.sh`), `sha256sum` (used in `app-fixture-recording.md`) |
| 27 | [slows] **Listed without a way to install:** anvil (the pinned 1.8.0 has no install command; `getfoundry.sh` from the README installs the latest); aws CLI and gcloud ("by hand"; not on Git Bash's PATH on Windows by default; the Compute Engine API must be enabled, which only a check command in `e2e/cloud/README.md` hints at); forge, Python, openssl, cast (needed only to regenerate the contract binding, not stated). Docker and nitro-cli versions are recorded, not pinned *Partly, 2026-11-16:* anvil 1.8.0 and gcloud now have install commands per system in the TDX and SEV-SNP procedures (step 1); the aws CLI and the rest are as listed |
| 28 | [slows] **`0-Setup-local-install.sh`:** winget may need administrator rights; on Linux it always fetches the `linux-amd64` Go tarball, also on arm64 |

### Things outside the repository

| # | gap |
|---|---|
| 29 | [blocks] **Files a fresh clone does not have:** `pem2.pem` (made by `aws-nitro.sh create`, fine), `auditor.key` and `deployer.key` (no instructions; not needed with `NET=local`), `e2e/env.local.sh` (from the example, fine) **Filled 2026-11-15** for the key files (`82e9076`, `e2e/README.md`, "Key files"): not needed with `NET=local`, needed funded only on a public network, and how to make them; the `openssl` step was checked, the `cast wallet address` step was **not run** (cast is not installed on our machine) |
| 30 | [slows] **The security group follows the first computer.** `aws-nitro.sh create` opens port 22 only to the address of the machine that created the group, and reuses the group later. From another computer SSH times out, and no document says why or what to change (**person**: the account owner changes the rule) |
| 31 | [slows] **Instructions tied to one VM:** `app-fixture-recording.md` ("`GRUB_FORCE_PARTUUID` is already off on this instance") and `docs/build/sev-snp.md` ("The boot setting changed on `attestamp-snp-vendor`"). On a fresh VM the setting is on |
| 32 | [slows] **Procedures that live only in the night log:** the TDX Option 2 base initrd with the hook (`docs/build/tdx.md` and `app-fixture-recording.md` cite "Overnight 2026-11-12") |
| 33 | [slows] **Text addressed to people or to Claude rather than to the reader:** "settled with the user" (`tdx.md`), "project lead" (`sev-snp.md`), "a decision for the user" (`app-fixture-recording.md`), "the operator's machine" (`cvm-port.md`); `e2e/cloud/README.md` and `app-fixture-recording.md` address Claude and `CLAUDE.md` |

### Statements that contradict each other or the code

| # | gap |
|---|---|
| 34 | [slows] **Stale "not implemented" lines after today's build and launch:** the `docs/build/sev-snp.md` status line ("not implemented") against its own "Not done yet" and `cmd/vendor/snp.go`; `sev-snp.md` "Checking" ("no `--verify` for SEV-SNP yet"); the `README.md` platform list ("verification only") against its status section; `cmd/README.md` (build and launch only on Nitro, the responder only inside the enclave) and the three role READMEs, which never mention TDX or SEV-SNP; `STRUCTURE.md` ("tdx and snp verify recorded evidence") against its own table; `docs/build/tdx.md` "Not done yet" naming `setup/build/tdx` as the image maker, now `setup/build/verity`; the `docs/plan/cvm-port.md` header "Status (2026-11-11, evening)" *Partly, 2026-11-15:* the README platform list now links the two getting-started procedures. *Partly, 2026-11-16:* the `sev-snp.md` status line and both procedures' "Not done yet" rows now say what ran on 2026-11-15; the other stale lines are as listed |
| 35 | [slows] **Other contradictions:** `e2e/cloud/README.md` says the `attestamp-ra` firewall rule was "added by hand", but `gcp-cvm.sh` creates it; `toolchain.md` says docker is not used here, but it was on 2026-11-15; `README.md` says the AMI, docker and nitro-cli versions are "not recorded anywhere", but `toolchain.md` records them; `docs/build/tdx.md` "From scratch" creates one VM (`tdx create vendor`) where `e2e/cloud/README.md` creates two; `cmd/vendor/README.md` uses `out/app.eif`, but the EIF is named after `--app-dir` |

No current procedure depends on the scripts that no longer exist (the T10 relay uses `record-snp -relay-att`).
