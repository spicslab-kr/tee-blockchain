# Is everything needed to build in the repository?

The rule (project lead, 2026-11-11): every piece of information needed to build and run the project, however small,
must be in the repository. This list is the audit of 2026-11-11: what someone with only this repository would be
missing, for each platform. It is kept up to date as gaps are filled.

"Needs" says who can fill a gap: **repo** (derivable from the repository or public sources), **person** (only
someone with the accounts or the original machines knows), **instance** (needs a running instance).

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
| 1 | **Nitro build environment never recorded**: the AMI ID actually used, `docker` and `aws-nitro-enclaves-cli` versions, kernel. `build` and `launch` install them with `dnf` without a version; PCR1 depends on `nitro-cli` | instance (at the Nitro baseline) | `docs/build/aws-nitro.md`: output of `rpm -q docker aws-nitro-enclaves-cli`, `uname -r`, `/etc/os-release`, the AMI ID |
| 2 | **Reference results**: expected PCR0/1/2 of the test application, and the registry addresses and transactions of a run. No run has been recorded (`docs/plan/test-matrix.md`) | instance (Nitro baseline, after `a379a2c` and `e6ef9b4`) | a results file under `docs/` |
| 3 | **forge version.** Done 2026-11-12 as far as the repository allows: forge 1.8.0 with OpenZeppelin `cab19933` regenerates both committed `registry.go` files byte for byte, metadata hash included. Which forge was used originally is still unknown (person) | — | `docs/build/toolchain.md`; both `generate.go` |
| 4 | **Python.** Done 2026-11-12: Python 3, standard library only; 3.14.6 checked in the same regeneration | — | `docs/build/toolchain.md` |
| 5 | **The SEV-SNP test scripts** (`vendor/`, `option1-initrd/`, `option2-dm-verity/`, `relay-test/`, `mkcpio.py`, `verity_boot.sh`) are outside the repository | decided 2026-11-11: they stay outside; the final implementation is Go in `internal/setup/{build,launch}/snp`, and any reported result writes its procedure into `docs/build/sev-snp.md` | — |
| 6 | **SEV-SNP guest tools**: the versions on our guest are now recorded (tpm2-tools 5.6-1build4, squashfs-tools 1:4.6.1-1build1, cryptsetup-bin 2:2.7.0-1ubuntu4.2; `docs/build/toolchain.md`). Still unknown: the `snpguest` and `gotpm` versions the 2026-11 scripts used | person | `docs/build/toolchain.md` |
| 7 | **The Nitro test application.** Done 2026-11-12: the pinned commit `ef5a25a6` of the personal fork is an unmodified upstream `nginx/docker-nginx` commit (an ancestor of upstream `master`; the fork's only own commit after it adds a built EIF and a record, which no script uses). The four scripts now clone upstream at the same commit; nothing to vendor | — | `e2e/README.md`, "Setup, once" |
| 8 | **Fixture READMEs for Nitro**: `internal/tee/parse/aws/testdata/` and `internal/tee/request/testdata/` have none (date, AMI, `nitro-cli` version, image, nonce source) | person | a `README.md` in each, like the TDX and SEV-SNP ones |
| 9 | **Packing SEV-SNP evidence for `ra-parse`.** Done 2026-11-12: `internal/tee/parse/snp/testdata/pack` joins a recording's three files; checked by packing `gcp-n2d-opt1p-v1` and verifying it with `ra-parse`. Live evidence from `ra-request` needs no packing | — | `internal/tee/parse/snp/testdata/README.md` |
| 10 | **GCP prerequisites.** Done 2026-11-12: Compute Engine API enabled, `default-allow-ssh` present, `xxd` in the stock Ubuntu image, `tpm2-tools` not (installed by hand) | — | `e2e/cloud/README.md`, "GCP prerequisites" |
| 11 | **git on the Nitro instances.** Done 2026-11-12 in the script, **not run**: `0-Setup-local-upload.sh` installs git with `dnf` on both instances when it is missing (`bash -n` only; the first Nitro run tests it) | — | `e2e/0-Setup-local-upload.sh` |
| 12 | **arm64 Nitro host.** Done 2026-11-12: `aws-nitro.sh create-arm64` launches one c7g.xlarge from the latest Amazon Linux 2023 arm64 AMI with Nitro Enclaves on (both types checked with `describe-instance-types` in eu-north-1); `status`, `stop`, `start` and `delete` include it | — | `e2e/cloud/aws-nitro.sh`, `e2e/cloud/README.md` |
| 13 | **vsock port, CID, enclave memory and CPUs.** Done 2026-11-12: defaults, flags and where each goes, including that `launch` opens the port from `0.0.0.0/0` when the role allows it | — | `docs/build/aws-nitro.md`, "Ports, vsock address and enclave resources" |
| 14 | **Intel SGX Root CA.** Done 2026-11-12: identical to Intel's published `Intel_SGX_Provisioning_Certification_RootCA.cer` (serial and full sha256 fingerprint) | — | `docs/build/tdx.md` |
| 15 | **Identifiers in committed files.** Done 2026-11-12: the project ID and every local path were removed from `docs/plan/night-log.md`. The project ID stays inside the eight SEV-SNP `*-att.pb` fixtures, where Google signed it; `internal/tee/parse/snp/testdata/README.md` now says so at the top of its identifiers section. Still for a person to decide: publishing those fixtures; the EC2 instance ID inside the Nitro fixture (also signed, and asserted by `internal/tee/parse/aws/measure_test.go`), and a second, different EC2 instance ID in the signed `module_id` of `internal/tee/request/testdata/nitro-attestation.cbor` (found 2026-11-12, `docs/plan/audit-2026-11-12.md`); and the original author's two Sepolia addresses, which `e2e/env.sh` no longer uses (removed 2026-11-12; they remain in git history) | person (publication decision) | — |
