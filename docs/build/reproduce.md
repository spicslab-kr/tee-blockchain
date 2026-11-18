# Reproducing the measurements: what both sides must match

The Vendor and the Auditor build and boot the same thing on two machines, and the registered value is only worth
something if both get the same measurements. This page is the checklist: **set every row below the same on both
sides**, and check it with the command given. Each row says which registered value it moves, and whether that was
**measured** here (by changing only that and comparing) or follows from the **structure** of the evidence and was not
measured.

The values given are the ones our runs used and measured with. A newer one is fine **if both sides use the same**,
except where a row says "exactly". What was used on which day, as a record: [toolchain.md](toolchain.md), "Versions
actually used", and `docs/results/`.

## Every platform

| what | set it to | check with | moves | how we know |
|---|---|---|---|---|
| this repository's commit | one and the same commit on both sides, for the `att` binaries and the responder | `git rev-parse HEAD` | Nitro: PCR0 and PCR2 change with any change to code the responder can run, or to the lines that code sits on; with the empty build ID (since 2026-11-17) code it never runs and comments that move no line no longer move them (`1967749d` did, with Go's build ID). TDX and SEV-SNP: the archive or image `build` writes, so RTMR2 / INITRD (and for SEV-SNP Option 2 CMDLINE and PCR12) | measured on Nitro (`5573b538…` → `c61c988e…` moved PCR0 and PCR2); CVM by structure (the cpio and verity writers are pinned by golden tests) |
| Go for the `att` binaries | **exactly** go1.26.1 (`internal/setup/build/GO_TOOLCHAIN`); set `GOTOOLCHAIN=go1.26.1` if another Go is installed | `go version`; `att-vendor version` | Nitro: PCR0 and PCR2 (the responder is in the image); `build` refuses any other version on every platform | measured: responders `c61c988e…` (go1.26.1) vs `24724239…` (go1.26.2); refusal measured on Nitro, TDX and SEV-SNP |
| build flags | `CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid=` for every binary | `go version -m <binary>` shows `-trimpath=true`, `CGO_ENABLED=0`, no `vcs.*`; `go tool buildid <binary>` prints nothing, since the binary carries no build ID (with one it prints four `/`-separated hashes); `go version -m` cannot show this setting, because Go does not record `-ldflags` in a `-trimpath` binary's build info (checked 2026-11-18) | Nitro: PCR0 and PCR2 | measured: without `-buildvcs=false` two commits of the same code gave two responders; with cgo `a71bedfc…` vs `5573b538…`; with Go's build ID one unused function in a linked package gave `c61c988e…` vs `5573b538…`, and without it both are `215423b8…` (`docs/build/aws-nitro.md`). `build` refuses a responder missing any of the four (the empty build ID since 2026-11-17) |
| OS and CPU architecture of the build | the same: `linux/amd64` (or `linux/arm64` on Nitro). TDX and SEV-SNP guests on GCP are `x86_64` | `uname -m`; `receive` compares the Vendor's `build_env` (OS, architecture, Go) and refuses a mismatch | Nitro: all three PCRs | measured: x86_64 vs arm64 differ in PCR0, PCR1 and PCR2 (`docs/results/aws-nitro-2026-11-16.md`); `receive`'s refusal measured on Nitro and on TDX and SEV-SNP guests |
| your application | the same files byte for byte; for a Go application, the same Go version and the three flags above | `sha256sum` of every file | Nitro: PCR0 and PCR2. TDX: RTMR2. SEV-SNP: INITRD (Option 1), or root hash, CMDLINE and PCR12 (Option 2) | measured: one byte changed moved exactly these on all three (T5); go1.26.1 vs go1.26.2 for a Go application moved RTMR2 and INITRD (T2) |

## AWS Nitro Enclaves

The values are the EIF's: PCR0 (the whole image), PCR1 (the kernel and boot ramdisk Nitro adds), PCR2 (the
application and the responder). Details: [aws-nitro.md](aws-nitro.md).

| what | set it to | check with | moves | how we know |
|---|---|---|---|---|
| AMI | the same AMI on both sides. Ours, in `eu-north-1`: Amazon Linux 2023 `al2023-ami-2023.12.20261109.0-kernel-6.12-x86_64`, `ami-048b77d5ca32f6d32` (x86_64); `ami-0ce2f8dd9b0fcfb35`, the newest AL2023 arm64 AMI on 2026-11-16 (arm64). Since 2026-11-17 the x86_64 values are from `ami-0f71b90bee7050a75` (release 2023.12.20261112, kernel 6.18), with the same nitro-cli and docker; arm64 is the same AMI. An AMI ID is per region | the instance metadata `ami-id`; `cat /etc/os-release`. `receive` compares the release (`build_env.ami_release`, recorded by `send`) since 2026-11-17: a proxy, not a guarantee: each Amazon Linux release has its own package repository, but a `dnf update` with another release, `releasever=latest` in `/etc/dnf/vars`, or a package installed by hand moves the versions without moving the release | through the docker and nitro-cli it brings (next rows) | structure; not measured by changing only the AMI |
| nitro-cli | the same version on both sides. Ours: `aws-nitro-enclaves-cli-1.5.0-0.amzn2023` (`Nitro CLI 1.5.0`). `build` installs it with `dnf` without a version, so the AMI decides | `rpm -q aws-nitro-enclaves-cli`; `nitro-cli --version`. `build --config` compares it with the Vendor's `build_env.nitro_cli` right after installing it (since 2026-11-17) | PCR1 (it supplies the kernel files) | structure; **not measured** with another version. With 1.5.0, PCR1 was `4b4d5b36…` on x86_64 and `3b4a7e1b…` on arm64 |
| docker | the same version on both sides. Ours: package `docker-25.0.18-1.amzn2023.0.3` (`docker --version`: 25.0.14) | `rpm -q docker`; `docker --version`. `build --config` compares the package version with the Vendor's `build_env.docker` right after installing it (since 2026-11-17) | possibly PCR0 and PCR2, through how it exports the image; `build` repacks the files in a fixed order with fixed times | **not measured** with another version |
| the application's Dockerfile | a base image pinned by digest, every package and `git clone` pinned, nothing that writes the time, the host name or a random value | `att-vendor build --verify` (builds twice and compares) | PCR0 and PCR2 | measured: an unpinned build differed through the build day in `/etc/shadow`, fixed by `a402827` (`docs/build/aws-nitro.md`, "Two builds compared") |
| instance type | ours: `m6i.xlarge` (x86_64), `c7g.xlarge` (arm64) | `aws ec2 describe-instances` | not expected to move anything: the PCRs are of the image | two `m6i.xlarge` gave the same PCRs; another type of the same architecture **not measured** |

Not part of the measurement: the region, the instance's own kernel, the enclave's CPU and memory (`launch --cpus`,
`--memory`).

## Intel TDX (GCP)

The values are MRTD (the firmware), RTMR0 (its view of the VM's configuration), RTMR1 (shim, GRUB, kernel) and RTMR2
(GRUB's commands, the kernel command line and the initrd). Details: [tdx.md](tdx.md), "The same inputs".

| what | set it to | check with | moves | how we know |
|---|---|---|---|---|
| guest image | **exactly** the Vendor's image version, never the family. Ours: `ubuntu-2404-noble-amd64-v20261105` in the project `ubuntu-os-cloud` (`IMAGE=ubuntu-2404-noble-amd64-v20261105` for `e2e/cloud/gcp-cvm.sh`; the project is its default `IMAGE_PROJECT`) | `gcloud compute disks describe <vm> --zone <z> --format='value(sourceImage)'` | RTMR1 (kernel), RTMR2 (the root file system UUID on the command line, the stock initrd) | measured: guests of one image made days apart gave the same four values; the UUID comes with the image version |
| kernel | the image's own, not upgraded. Ours: `7.0.0-1017-gcp` | `uname -r` | RTMR1, and the Option 1 base initrd, so RTMR2 | structure: the kernel is in RTMR1; not measured by changing only the kernel |
| machine type | **the Vendor's**. Ours: `c3-standard-4` (`MACHINE=`) | `gcloud compute instances describe <vm> --zone <z> --format='value(machineType.basename())'` | RTMR0 | measured: `c3-standard-8` gave RTMR0 `eba48352…` instead of `8847ec5d…`, nothing else moved. vCPUs and memory could not be told apart: GCP refused `c3-highmem-4` and `c3-highcpu-8` for TDX, and Google's list of TDX machine types (read 2026-11-18) has only `standard` shapes, where both grow together |
| zone | the Vendor's. Ours: `us-central1-a` | the same `describe` | nothing, where measured | measured: `us-central1-b` gave the same four values (one other zone, one day) |
| firmware (TDVF) | cannot be chosen: GCP supplies it | MRTD itself | MRTD and RTMR0 | structure; a GCP update would change every registration |
| `GRUB_FORCE_PARTUUID` | **commented out** in `/etc/default/grub.d/40-force-partuuid.cfg`, then `update-grub` ("Getting started" step 4) | `grep GRUB_FORCE_PARTUUID /etc/default/grub.d/40-force-partuuid.cfg` shows `#`; after the boot `/proc/cmdline` has `root=UUID=` | whether an initrd is loaded at all, so RTMR2 | measured: with it set the stock entry boots without any initrd and the application is never loaded; `launch` refuses such a guest |
| the boot entry | the one `launch` writes (`attestamp`: the stock entry with only its title and initrd changed) | `launch --check` | RTMR2 (GRUB's commands and the command line) | measured: `launch --check` replays the event log and requires the built initrd |
| base initrd | Option 1: the image's own `/boot/initrd.img-7.0.0-1017-gcp` (sha256 `4703ec07…` on every guest of this image). Option 2: **the Vendor's** `base-initrd-v.img`, copied, never remade (`mkinitramfs` writes a random seed) | `sha256sum` on both sides | RTMR2 (the initrd hash) | measured for Option 1: the same file on every guest of the image. Option 2: the record of the old out-of-repository scripts saw `mkinitramfs` give a different file every time; our procedures therefore copy the Vendor's and never compared two remade ones |
| Option 2 image tools | squashfs-tools `1:4.6.1-1build1`, cryptsetup `2:2.7.0-1ubuntu4.2` (the image's own) | `dpkg-query -W squashfs-tools cryptsetup-bin` | the image and its root hash, so RTMR2 | measured: equal bytes on two guests and in a WSL with these versions |
| option | the Vendor's (`--option 1` or `2`) | `build-value.json` | everything after the firmware | by construction |

## AMD SEV-SNP (GCP)

The values are MEASUREMENT (the firmware, from the SNP report), PCR4 (shim and GRUB), KERNEL, CMDLINE and INITRD
(from the vTPM's event log), and for Option 2 PCR12. Details: [sev-snp.md](sev-snp.md), "The same inputs".

| what | set it to | check with | moves | how we know |
|---|---|---|---|---|
| guest image | **exactly** the Vendor's image version. Ours: `ubuntu-2404-noble-amd64-v20261105` (`IMAGE=`, as for TDX) | as for TDX | CMDLINE (root UUID), the stock initrd, often KERNEL and PCR4 | measured: guests of one image gave the same five values |
| kernel | the image's own, not upgraded. Ours: `7.0.0-1017-gcp` | `uname -r` | KERNEL, and the Option 1 base initrd, so INITRD | structure; not measured by changing only the kernel |
| machine type | **the Vendor's**, at least the same vCPU count. Ours: `n2d-standard-2` | as for TDX | MEASUREMENT, by the vCPU count | measured: `n2d-standard-4` gave `2279d213…` instead of `f6790085…`; `n2d-highmem-2` (2 vCPUs, more memory) did not change it |
| CPU platform | `--min-cpu-platform="AMD Milan"` (what `gcp-cvm.sh` sets) | `grep 'model name' /proc/cpuinfo` (ours: `AMD EPYC 7B13`) | MEASUREMENT, by structure | **not measured** on another CPU platform |
| zone | the Vendor's. Ours: `us-central1-a` | as for TDX | nothing, where measured | measured: `us-central1-b` gave the same five values |
| firmware (OVMF) | cannot be chosen: GCP supplies it | MEASUREMENT itself | MEASUREMENT | structure |
| `GRUB_FORCE_PARTUUID` | **commented out**, as for TDX | as for TDX | whether an initrd is loaded; without one every format refuses the evidence | measured: an initrdless boot is refused, exit 7 |
| the boot entry | the one `launch` writes; for Option 2 it adds `attestamp.roothash=<root hash>` | `launch --check` | CMDLINE | measured |
| base initrd | Option 1: the image's own `/boot/initrd.img-7.0.0-1017-gcp` (`4703ec07…`). Option 2: **the Vendor's** `base-initrd-v.img` (it carries the hook that extends PCR12), copied, never remade | `sha256sum` | INITRD | measured, as for TDX |
| Option 2 tools | squashfs-tools and cryptsetup as for TDX; on the Vendor's guest also `tpm2-tools` (`5.6-1build4`, installed by hand) for the hook | `dpkg-query -W squashfs-tools cryptsetup-bin tpm2-tools` | the root hash, so CMDLINE and PCR12 | measured for the image tools, as for TDX |
| format | the Vendor's: `amd-sev-snp-initrd` or `amd-sev-snp-verity` | `build-value.json` | which values are registered | by construction |

## What these rows do not cover

- A value that moves with the provider: GCP's firmware (MRTD, MEASUREMENT) and Nitro's PCR1 through nitro-cli. Both
  sides then move together and register again.
- Settings not listed here were not found to move a value, which is not the same as checked: for example Shielded VM
  Secure Boot (off on our guests) was never changed.
