# Prerequisites — instance info

## Local machine

Go version: **exactly 1.26.1** (not "or newer" — reproducible-build comparisons need the same compiler)
Installed by: `e2e/0-Setup-local-install.sh`
Tested on: Windows (Git Bash) — real install verified, admin rights required on Windows
Not yet tested on: Ubuntu, mac (script has apt/dnf/yum/brew branches, unverified on real hardware)

## Vendor / Auditor Nitro instance (AWS EC2)

Instance spec: Nitro Enclaves enabled, 4+ vCPU (m5, c5, m6i, ...)
Region: eu-north-1
OS info: Amazon Linux 2023.12.20260918, kernel 6.18.48-109.150.amzn2023.x86_64
Preinstalled on the AMI (not installed by us): git 2.50.1, nitro-cli 1.5.0, docker 25.0.14

docker/nitro-cli don't need a separate install step — `tb-vendor build`/`tb-auditor build` installs
them itself on first run (`dnf install -y aws-nitro-enclaves-cli aws-nitro-enclaves-cli-devel docker`).
