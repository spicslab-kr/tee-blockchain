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

## Still missing

Most important first.

| # | gap | needs | where it should go |
|---|---|---|---|
| 1 | **Nitro build environment.** Done 2026-11-12 at the baseline: `ami-048b77d5ca32f6d32` (Amazon Linux 2023.12.20261109), kernel 6.12.114-139.210, aws-nitro-enclaves-cli 1.5.0, docker 25.0.18 (rpm) / 25.0.14 (binary), git 2.50.1 installed by the setup script | — | `docs/build/toolchain.md`, "Nitro parent instance" |
| 2 | **Reference results.** The part without a chain done 2026-11-12: the test application's PCR0/1/2 and every verdict of the chain-free checks. **PCR0 and PCR2 there are values from before `fc105b0`** (2026-11-15), which empties the build day in `/etc/shadow` and so changes them; they were not re-measured, and the current reference PCR0/PCR2 are unknown until the baseline is run again. Chain run done on a local chain 2026-11-15 (`docs/results/aws-nitro-2026-11-15-local-chain.md`), also before `fc105b0`. Still missing: the registry addresses and transactions of a chain run (needs Sepolia ETH), and the arm64 values | ETH; an arm64 instance | `docs/results/aws-nitro-2026-11-12.md` and `.jsonl` |
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
| 15 | **Identifiers in committed files.** Done 2026-11-12: the project ID and every local path were removed from `docs/plan/night-log.md`. The project ID stays inside the eight SEV-SNP `*-att.pb` fixtures, where Google signed it; `internal/tee/parse/snp/testdata/README.md` now says so at the top of its identifiers section. Still for a person to decide: publishing those fixtures; the EC2 instance ID inside the Nitro fixture (also signed, and asserted by `internal/tee/parse/aws/measure_test.go`), and a second, different EC2 instance ID in the signed `module_id` of `internal/tee/request/testdata/nitro-attestation.cbor` (found 2026-11-12, `docs/plan/audit-2026-11-12.md`); and the original author's two Sepolia addresses, which `e2e/env.sh` no longer uses (removed 2026-11-12; they remain in git history) | person (publication decision) | — |
