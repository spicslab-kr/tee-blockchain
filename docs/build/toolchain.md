# Toolchain: everything a build or a run calls

What someone with an empty machine and only this repository has to install, which step calls which tool, and the
versions that were actually used. Every version below was printed by the tool itself (`--version`, `dpkg-query -W`) on
the date given; a version nobody printed is marked **not recorded**, never guessed.

The platform guides refer here: [AWS Nitro](aws-nitro.md), [Intel TDX](tdx.md), [AMD SEV-SNP](sev-snp.md).

## What to install, by machine

| machine | needs | how it gets there |
|---|---|---|
| **your machine** (any OS; the e2e scripts need bash) | Go at exactly `internal/setup/build/GO_TOOLCHAIN`; git; ssh/scp; for the chain tests anvil | Go and git by hand or `bash e2e/0-Setup-local-install.sh`; anvil from the Foundry release (below). `go.mod` says only `go 1.26.1` (no `toolchain` line), so a newer installed Go is used as is and `build` then refuses; set `GOTOOLCHAIN=go1.26.1` (as `.claude/settings.json` does) and Go downloads and uses exactly the pinned one |
| your machine, to create instances | `aws` CLI (Nitro) or `gcloud` (TDX, SEV-SNP), signed in | by hand; see `e2e/cloud/README.md` |
| your machine, only to regenerate the contract binding | forge (solc 0.8.28 is fetched by forge), Python 3, OpenZeppelin as the submodule | `go generate` in `internal/chain/register/*/ether`; see "The Registry binding" |
| **Nitro parent instance** (Amazon Linux 2023) | docker, `aws-nitro-enclaves-cli` (nitro-cli and the allocator), git | `att-* build` and `launch` install docker and nitro-cli with `dnf` themselves (no version pinned); `0-Setup-local-upload.sh` installs git, which the AMI does not have. No Go: the binaries are cross-built on your machine and copied |
| **GCP TDX or SEV-SNP guest** (Ubuntu 24.04) | for the app fixtures: squashfs-tools, cryptsetup (veritysetup), initramfs-tools, GRUB; for SEV-SNP Option 2 also tpm2-tools; for the in-guest chain run anvil | the stock image has all but tpm2-tools (`e2e/cloud/README.md`, "GCP prerequisites"); `apt-get install -y tpm2-tools`; anvil from the Foundry release. No Go: binaries are cross-built and copied |

## Which step calls which tool

| step | where | tools it calls |
|---|---|---|
| `go build` of `att-vendor`, `att-auditor`, `att-user`, `att-ra-responder` | your machine | go (`-trimpath -buildvcs=false`, pinned version; `build` refuses a responder built otherwise) |
| `go test ./...` | your machine | go; anvil for the chain tests (looked up on `PATH`, then `~/.foundry/bin`); git for the `core/transfer` tests |
| `go generate` (the Registry binding) | your machine | forge → solc 0.8.28; python; `go run …/abigen` |
| `att-vendor build`, `att-auditor build`, `pack` | Nitro instance | dnf, systemctl, docker (`build`, `create`, `export`, `image inspect`), nitro-cli (`build-enclave`, `describe-eif`) |
| `att-* launch` | Nitro instance | dnf, systemctl (the allocator), nitro-cli (`run-enclave`, `describe-enclaves`, `terminate-enclave`), aws (opening the security-group port, when the instance role allows) |
| `att-auditor send`, `receive`; `att-vendor send`, `receive` | anywhere | git |
| e2e flow and check scripts | your machine, then the instances over ssh | bash, ssh, scp, go (cross-builds), curl (instance metadata), git (cloning the test application on the instance) |
| `e2e/cloud/aws-nitro.sh` | your machine | aws, curl (your public IP) |
| `e2e/cloud/gcp-cvm.sh` | your machine | gcloud |
| app fixtures on the CVM guests | guest | mksquashfs, veritysetup, mkinitramfs, update-grub; `tpm2_pcrextend` in the SEV-SNP Option 2 hook |
| `e2e/cvm/in-guest-verify.sh` | guest | anvil, the three att binaries, sudo |
| checks quoted in the docs | anywhere | openssl (certificate fingerprints, SHA3-512 cross-check), `cast` (keccak cross-check) |

## Versions actually used

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
| docker | `Docker version 29.8.0, build 88096ef` | **not used by the project here**; docker runs only on the Nitro instance |

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

`build` and `launch` install docker and `aws-nitro-enclaves-cli` with `dnf` without a version, so another day's image
may bring others. PCR1 depends on nitro-cli's kernel files: with 1.5.0 the test application's PCR1 was
`4b4d5b36…a493`. To record a later instance the same way:

```
rpm -q docker aws-nitro-enclaves-cli git
nitro-cli --version
docker --version
uname -r
cat /etc/os-release
curl -s -H "X-aws-ec2-metadata-token: $(curl -s -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60')" http://169.254.169.254/latest/meta-data/ami-id
```

(build-info gap #1.)
