# Toolchain: everything a build or a run calls

What someone with an empty machine and only this repository has to install, which step calls which tool, and the
versions that were actually used. Every version below was printed by the tool itself (`--version`, `dpkg-query -W`) on
the date given; a version nobody printed is marked **not recorded**, never guessed.

The platform guides refer here: [AWS Nitro](aws-nitro.md), [Intel TDX](tdx.md), [AMD SEV-SNP](sev-snp.md).

## What to install, by machine

| machine | needs | how it gets there |
|---|---|---|
| **your machine** (any OS; the e2e scripts need bash) | a bash: on Windows **Git Bash**, which comes with [Git for Windows](https://git-scm.com/download/win) together with git, ssh, scp and `sha256sum`; on Linux and macOS the system's. Go at exactly `internal/setup/build/GO_TOOLCHAIN`; git; ssh/scp; for the chain tests anvil | Go and git by hand or `bash e2e/0-Setup-local-install.sh` (apt, dnf, Homebrew on macOS, winget on Windows; on Windows, installing Go needs an administrator window, which the script checks for and says); anvil 1.8.0 from the Foundry release, commands in `docs/build/tdx.md` step 1 (Linux, macOS, WSL, Windows). `go.mod` says only `go 1.26.1` (no `toolchain` line), so a newer installed Go is used as is and `build` then refuses. Set `GOTOOLCHAIN=go1.26.1` in your shell and Go downloads and uses exactly the pinned one: `export GOTOOLCHAIN=go1.26.1` (bash; add it to `~/.bashrc` to keep it), `setx GOTOOLCHAIN go1.26.1` (Windows, for new windows), or per command, `GOTOOLCHAIN=go1.26.1 go build …`. `go version` then prints `go1.26.1` |
| your machine, to create instances | `aws` CLI (Nitro) or `gcloud` (TDX, SEV-SNP), signed in | `gcloud`: one command per system in `docs/build/tdx.md` step 1 (Windows: `winget install --id Google.CloudSDK`, then a **new** Git Bash window, which is how it gets on Git Bash's `PATH`). `aws`: AWS's installer for your system, from <https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html> (Windows: `winget install --id Amazon.AWSCLI`, then a new Git Bash window). Then sign in and, on GCP, enable the Compute Engine API once: `gcloud services enable compute.googleapis.com` (`e2e/cloud/README.md`, "GCP prerequisites") |
| your machine, only to regenerate the contract binding | forge (solc 0.8.28 is fetched by forge), Python 3, OpenZeppelin as the submodule | `go generate` in `internal/chain/register/*/ether`; see "The Registry binding". Not needed to build, test or run anything: the binding is committed |
| your machine, optional | openssl and `cast` (Foundry's, installed with anvil) for the cross-checks some documents quote; WSL 2 with Ubuntu 24.04 to build an [Option 2](tdx.md#option-1-and-option-2) image with the real tools without a VM (below); Docker Desktop to compare two `docker build`s of a Nitro application without an instance (done 2026-11-15, `docs/build/aws-nitro.md`, "Two builds compared") | none of these is needed by a procedure; each is named where it is used |
| **Nitro parent instance** (Amazon Linux 2023) | docker, `aws-nitro-enclaves-cli` (nitro-cli and the allocator), git | `att-* build` and `launch` install docker and nitro-cli with `dnf` themselves (no version pinned); `0-Setup-local-upload.sh` installs git, which the AMI does not have. No Go: the binaries are cross-built on your machine and copied |
| **GCP TDX or SEV-SNP guest** (Ubuntu 24.04) | for the app fixtures: squashfs-tools, cryptsetup (veritysetup), initramfs-tools, GRUB; for SEV-SNP Option 2 also tpm2-tools; for the in-guest chain run anvil | the stock image has all but tpm2-tools (`e2e/cloud/README.md`, "GCP prerequisites"); `apt-get install -y tpm2-tools`; anvil from the Foundry release. No Go: binaries are cross-built and copied |

## Which step calls which tool

| step | where | tools it calls |
|---|---|---|
| `go build` of `att-vendor`, `att-auditor`, `att-user`, `att-ra-responder` | your machine | go (`CGO_ENABLED=0` and `-trimpath -buildvcs=false -ldflags=-buildid=`, pinned version; `build` refuses a responder built otherwise) |
| `go test ./...` | your machine | go; anvil for the chain tests (looked up on `PATH`, then `~/.foundry/bin`); git for the `core/transfer` tests |
| `go generate` (the Registry binding) | your machine | forge → solc 0.8.28, called as `$HOME/.foundry/bin/forge` (where foundryup installs it; `PATH` is not searched); `python` under that name (where only `python3` exists, as on stock Ubuntu, add a `python` link or alias); `go run …/abigen` |
| `att-vendor build`, `att-auditor build`, `pack` | Nitro instance | dnf, systemctl, docker (`build`, `create`, `export`, `image inspect`), nitro-cli (`build-enclave`, `describe-eif`); `build` also rpm (`rpm -q docker aws-nitro-enclaves-cli`, the versions it installed) |
| `att-* launch` | Nitro instance | dnf, systemctl (the allocator), nitro-cli (`run-enclave`, `describe-enclaves`, `terminate-enclave`), aws (reading the security group; opening the port only with `--open-port-to`, when the instance role allows) |
| `att-vendor send` | aws-nitro: the Nitro instance that ran `build` (it reads the Amazon Linux release from `/etc/os-release` and the docker and nitro-cli versions with rpm, unless `build_env` already holds them); other formats: anywhere | git; rpm on aws-nitro |
| `att-auditor receive` | where you will build: it compares this machine's OS, architecture and Go with the Vendor's `build_env`, and on aws-nitro the Amazon Linux release, and refuses a mismatch unless `--ignore-env` | git |
| `att-auditor send` | anywhere | git; with `--config --measurements`, nitro-cli (`--version`, for the record's `build_tool`) unless `--build-tool-version` is given |
| `att-vendor receive` | anywhere | git |
| e2e flow and check scripts | your machine, then the instances over ssh | bash, ssh, scp, go (cross-builds), curl (instance metadata), git (cloning the test application on the instance) |
| `e2e/cloud/aws-nitro.sh` | your machine | aws, curl (your public IP) |
| `e2e/cloud/gcp-cvm.sh` | your machine | gcloud |
| app fixtures on the CVM guests | guest | mksquashfs, veritysetup, mkinitramfs, update-grub; `tpm2_pcrextend` in the SEV-SNP Option 2 hook |
| `e2e/cvm/in-guest-verify.sh` | guest | anvil, the three att binaries, sudo |
| `e2e/cvm/build-env-check.sh` | guest | `att-vendor`, `att-auditor`, git (the image has it); no sudo, no network |
| checks quoted in the docs | anywhere | openssl (certificate fingerprints, SHA3-512 cross-check), `cast` (keccak cross-check) |

## Versions actually used

These are records of what each run used, by date. **What to set so that both sides get the same values is in
[reproduce.md](reproduce.md)**; this section is the evidence behind it.

### Your machine (the development machine of 2026-11-12, Windows 11 with Git Bash)

| tool | `--version` output | used for |
|---|---|---|
| go | `go version go1.26.1 windows/amd64` | everything; the pin |
| git | `git version 2.46.0.windows.1` | the repository, `send`/`receive` tests |
| bash | `GNU bash, version 5.2.26(1)-release (x86_64-pc-msys)` | the e2e scripts |
| anvil | `anvil Version: 1.8.0`, commit `61ae26af36320d4fa1020f7db53785885e29eeb5` | the chain tests |
| forge | `forge Version: 1.8.0`, commit `61ae26af36320d4fa1020f7db53785885e29eeb5` (from `foundry_v1.8.0_win32_amd64.zip`, sha256 `e31241db…982e`, equal to the release's `.sha256`) | regenerating the binding |
| solc | 0.8.28, fetched by forge (`--use 0.8.28`); also in the deployed bytecode's metadata | the contract |
| python | `Python 3.14.6` | the binding's second generate step |
| abigen | go-ethereum v1.17.5, the module's (`go run`) | the binding |
| aws | `aws-cli/2.39.7 Python/3.14.8 Windows/11 exe/AMD64` | creating Nitro instances |
| gcloud | `Google Cloud SDK 592.0.0` (core 2026.10.30) | the GCP guests |
| openssl | `OpenSSL 3.2.2 4 Jun 2024` | the documented certificate checks |
| docker | `Docker version 29.8.0, build 88096ef` (Docker Desktop) | optional: no procedure needs it here (`build` runs docker on the Nitro instance); used on 2026-11-15 to compare two builds of the test application without an instance |

### The Registry binding

Which forge produced the committed `registry.go` was never recorded. Checked on 2026-11-12: forge 1.8.0 with
OpenZeppelin at `cab19933c33c2ad1d4c7a84864a3601dddfd16f3` (v5.7.0) and Python 3.14.6 run the three `go:generate`
steps to a `registry.go` byte-identical to the committed one in both `chain/register/auditor/ether` and
`chain/register/Vendor/ether`, metadata hash included. A different forge may pass other default settings to solc and
change the metadata hash at the end of the bytecode, and with it the deployed code.

### GCP guests (`attestamp-tdx-vendor`, `attestamp-snp-vendor`, 2026-11-11 and 2026-11-12)

Image `ubuntu-2404-noble-amd64-v20261105`, Ubuntu 24.04.5 LTS, kernel `7.0.0-1017-gcp` on both.

| package or tool | version (`dpkg-query -W` or `--version`) | on |
|---|---|---|
| squashfs-tools | `1:4.6.1-1build1` (mksquashfs 4.6.1) | both |
| cryptsetup-bin | `2:2.7.0-1ubuntu4.2` (veritysetup 2.7.0) | both |
| tpm2-tools | `5.6-1build4`, installed by hand | SEV-SNP |
| xxd | `2:9.1.0016-1ubuntu7.20`, in the stock image | both |
| git | `git version 2.43.0` (`1:2.43.0-1ubuntu7.4`) | SEV-SNP checked; not used by the recordings |
| curl | `8.5.0-2ubuntu10.16` | SEV-SNP checked |
| anvil | `anvil Version: 1.8.0`, from `foundry_v1.8.0_linux_amd64.tar.gz` (sha256 `8c8560de…ac57`, equal to the release's `.sha256`) | both, for `in-guest-verify.sh` |

The att binaries on the guests were cross-built on the development machine (go1.26.1, `GOOS=linux GOARCH=amd64
-trimpath -buildvcs=false`); no Go was installed on a guest.

### A clean Ubuntu 24.04 under WSL (2026-11-15): Option 2 with the real tools, no VM

A new WSL 2 distribution (`wsl --install Ubuntu-24.04 --name attestamp-trial`, Ubuntu 24.04.5 LTS, WSL 2.7.14.0,
kernel 6.18.33.2-2), with `apt-get install -y squashfs-tools cryptsetup-bin` and nothing else for the image:

| package or tool | version |
|---|---|
| squashfs-tools | `1:4.6.1-1build1` (`mksquashfs version 4.6.1 (2023/03/25)`) |
| cryptsetup-bin | `2:2.7.0-1ubuntu4.2` (`veritysetup 2.7.0`) |
| Go | go1.26.1 linux/amd64, from `bash e2e/0-Setup-local-install.sh` |

The same package versions as on the two guests, and the same bytes: `att-vendor build --platform amd-sev-snp-verity`
(and `--platform intel-tdx --option 2`) on the recordings' one-file application gave `app.img` sha256 `28bdfcff…ec1c`
and root hash `e29ec085…af0c9` for v1, `b8e2656f…fb27` and `a2f4df92…51e1` for v2, which is what
`make-verity-image.sh` gave on both guests on 2026-11-11, and the PCR12 predicted for v1 (`470379c7…`) is the PCR12
in the recorded `gcp-n2d-opt2-v1` evidence. `--verify` passed each time.

### Nitro parent instance

Recorded on both instances of the Nitro baseline, 2026-11-12 (`docs/results/aws-nitro-2026-11-12.md`); the two were
identical in every line:

| what | value, as printed |
|---|---|
| AMI | `ami-048b77d5ca32f6d32` (`al2023-ami-2023.12.20261109.0-kernel-6.12-x86_64`, created 2026-11-04), from instance metadata `ami-id` and `aws ec2 describe-images` |
| OS | `PRETTY_NAME="Amazon Linux 2023.12.20261109"` |
| kernel | `uname -r`: `6.12.114-139.210.amzn2023.x86_64`; `uname -m`: `x86_64` |
| instance type, region | m6i.xlarge, eu-north-1 |
| aws-nitro-enclaves-cli | `rpm -q`: `aws-nitro-enclaves-cli-1.5.0-0.amzn2023.x86_64` (and `-devel` the same); `nitro-cli --version`: `Nitro CLI 1.5.0` |
| docker | `rpm -q`: `docker-25.0.18-1.amzn2023.0.3.x86_64`; `docker --version`: `Docker version 25.0.14, build 0bab007` (the package and the binary report different patch numbers; both are recorded as printed) |
| git | `git version 2.50.1` (`git-2.50.1-1.amzn2023.0.1`); **not in the AMI**: `0-Setup-local-upload.sh` installed it |
| enclave allocator | `memory_mib: 512`, `cpu_count: 2` after the builds (raised by `launch` when needed) |

The same two instances on 2026-11-16 (the baseline taken again, `docs/results/aws-nitro-2026-11-16.md`): every line
the same. The **arm64** instance of that day, a `c7g.xlarge` made with `aws-nitro.sh create-arm64`:

| what | value, as printed |
|---|---|
| AMI | `ami-0ce2f8dd9b0fcfb35`, the newest Amazon Linux 2023 arm64 on 2026-11-16 |
| kernel | `6.1.190-237.442.amzn2023.aarch64`; `uname -m`: `aarch64` (Amazon's arm64 image ships kernel 6.1, the x86_64 one 6.12) |
| aws-nitro-enclaves-cli | `aws-nitro-enclaves-cli-1.5.0-0.amzn2023.aarch64`; `Nitro CLI 1.5.0` |
| docker | `docker-25.0.18-1.amzn2023.0.3.aarch64`; `Docker version 25.0.14, build 0bab007` |
| git | `git-2.50.1-1.amzn2023.0.2.aarch64`, installed by `0-Setup-local-upload.sh` |

On **2026-11-17** (the baseline for the empty build ID, `docs/results/aws-nitro-2026-11-17.md`) three new instances,
made with `aws-nitro.sh create --yes` and `create-arm64 --yes`. The two x86_64 ones were identical in every line, and
on **another image** than the earlier x86_64 pair:

| what | x86_64 (both) | arm64 |
|---|---|---|
| AMI | `ami-0f71b90bee7050a75` (the newest Amazon Linux 2023 that day) | `ami-0ce2f8dd9b0fcfb35` (the same as on 2026-11-16) |
| OS | `PRETTY_NAME="Amazon Linux 2023.12.20261112"` | the same |
| kernel | `6.18.62-127.179.amzn2023.x86_64` | `6.1.190-237.442.amzn2023.aarch64` |
| aws-nitro-enclaves-cli | `aws-nitro-enclaves-cli-1.5.0-0.amzn2023.x86_64` (and `-devel`); `Nitro CLI 1.5.0` | `…1.5.0-0.amzn2023.aarch64`; `Nitro CLI 1.5.0` |
| docker | `docker-25.0.18-1.amzn2023.0.3.x86_64`; `Docker version 25.0.14, build 0bab007` | `docker-25.0.18-1.amzn2023.0.3.aarch64`; the same |
| git | `git-2.50.1-1.amzn2023.0.2.x86_64`, installed by `0-Setup-local-upload.sh` | `git-2.50.1-1.amzn2023.0.2.aarch64` |

Later on **2026-11-17** (the baseline after the shared Nitro request package,
`docs/results/aws-nitro-2026-11-17-shared-aws.md`) three more new instances, made the same way, came from the same two
AMIs and gave **every line of the table above again**, on both x86_64 instances and on arm64.

`build` and `launch` install docker and `aws-nitro-enclaves-cli` with `dnf` without a version, so another day's image
may bring others. Since 2026-11-17 the Vendor's `send` records the Amazon Linux release and both package versions in
`build_env`, the Auditor's `receive` compares the release and its `build --config` the versions after installing them
(`docs/build/aws-nitro.md`, "What `build` cannot change"; run on two instances on 2026-11-17). PCR1 depends on nitro-cli's kernel files: with 1.5.0 the test application's PCR1 was
`4b4d5b36…a493` on x86_64 (2026-11-12 at `ab55208`, 2026-11-16 at `23b83f9`, and 2026-11-17 at `83cb2dc` and at
`6146c10` on the newer image) and `3b4a7e1b…` on arm64 (2026-11-16 at `23b83f9` and 2026-11-17 at `83cb2dc` and at
`6146c10`). To record a later instance the same way:

```
rpm -q docker aws-nitro-enclaves-cli git
nitro-cli --version
docker --version
uname -r
cat /etc/os-release
curl -s -H "X-aws-ec2-metadata-token: $(curl -s -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60')" http://169.254.169.254/latest/meta-data/ami-id
```

(`docs/results/build-info-gaps-2026-11.md`, #1.)
