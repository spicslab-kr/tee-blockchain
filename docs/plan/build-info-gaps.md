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
| e2e approver addresses hard-coded to the original author's keys | `AUDITOR_ADDR` and `VENDOR_ADDR` can be set in `e2e/env.local.sh`; `e2e/README.md`, `e2e/env.local.example.sh` |
| `e2e/cloud/` not reachable from the e2e guide; "stop the instances from the console" | linked from `e2e/README.md`, which now names `aws-nitro.sh stop` |
| stale and non-English lines in `docs/plan/cvm-port.md` | fixed |

## Still missing

Most important first.

| # | gap | needs | where it should go |
|---|---|---|---|
| 1 | **Nitro build environment never recorded**: the AMI ID actually used, `docker` and `aws-nitro-enclaves-cli` versions, kernel. `build` and `launch` install them with `dnf` without a version; PCR1 depends on `nitro-cli` | instance (at the Nitro baseline) | `docs/build/aws-nitro.md`: output of `rpm -q docker aws-nitro-enclaves-cli`, `uname -r`, `/etc/os-release`, the AMI ID |
| 2 | **Reference results**: expected PCR0/1/2 of the test application, and the registry addresses and transactions of a run. No run has been recorded (`docs/plan/test-matrix.md`) | instance (Nitro baseline, after `a379a2c` and `e6ef9b4`) | a results file under `docs/` |
| 3 | **forge version**: `generate.go` gives only a path; the forge that produced `registry.go` is not recorded. (The anvil the tests run with is now recorded in `README.md`, "Testing": 1.8.0.) | person (forge version used) | `CONTRIBUTING.md`, a toolchain table |
| 4 | **Python**: `generate.go` calls `python` (version not given) | repo | same table |
| 5 | **The SEV-SNP test scripts** (`vendor/`, `option1-initrd/`, `option2-dm-verity/`, `relay-test/`, `mkcpio.py`, `verity_boot.sh`) are outside the repository | decided 2026-11-11: they stay outside; the final implementation is Go in `internal/setup/{build,launch}/snp`, and any reported result writes its procedure into `docs/build/sev-snp.md` | — |
| 6 | **SEV-SNP guest tools**: `snpguest` version used in 2026-11 unknown; `gotpm`, tpm2-tools, squashfs-tools, cryptsetup versions not given | person | `docs/build/sev-snp.md` |
| 7 | **The Nitro test application** is a personal fork (`github.com/qixiangme/docker-nginx@ef5a25a6`), referenced from `e2e/flow1-*/1-*.sh` | repo (vendor it, or record upstream plus diff) | `e2e/testapp/` |
| 8 | **Fixture READMEs for Nitro**: `internal/tee/parse/aws/testdata/` and `internal/tee/request/testdata/` have none (date, AMI, `nitro-cli` version, image, nonce source) | person | a `README.md` in each, like the TDX and SEV-SNP ones |
| 9 | **Packing SEV-SNP evidence for `ra-parse`** needs hand-written Go (`snp.Encode`); there is no command | repo | a small command next to `snp/testdata/record` or an `att-user` subcommand |
| 10 | **GCP prerequisites**: the default network's `default-allow-ssh` rule, the Compute Engine API enabled, `xxd` on the guest for the TDX recording | repo | `e2e/cloud/README.md` |
| 11 | **git on the Nitro instances**: no script installs it | repo | the remote setup script |
| 12 | **arm64 Nitro host**: no create script; `aws-nitro.sh` filters x86_64 images | repo | `e2e/cloud/aws-nitro.sh` |
| 13 | **vsock port, CID, enclave memory and CPUs**: in code only (`entrypoint.sh`, `cmd/auditor/main.go`) | repo | `docs/build/aws-nitro.md` |
| 14 | **Intel SGX Root CA** not compared with Intel's published copy | repo (public download) | `docs/build/tdx.md` |
| 15 | **`docs/plan/night-log.md`** carries the GCP project ID and local scratchpad paths. The project ID is also in the committed SEV-SNP fixture by decision | person (decide whether to scrub the log) | — |
