# Creating the instances

Scripts that create, stop and delete the instances each platform needs. Every command they run is
printed in the script's header comment, and every flag was taken from the vendor's own
documentation (see the comments for which page).

**These scripts create billable resources.** Every `create`, `run-instances`, `delete` and
`terminate-instances` they run costs money or destroys state. Run them only when you mean to, and if an automated
agent runs commands for you, have it ask before each of these.

## What you have to do yourself, once

Nothing below can be automated — it needs your account and your identity.

| | AWS | GCP |
|---|---|---|
| account with billing | yes | yes |
| CLI installed | `aws` | `gcloud` |
| signed in | `aws configure` | `gcloud auth login` and `gcloud config set project <id>` |
| used for | Nitro Enclaves only | Intel TDX and AMD SEV-SNP |
| quota | On-Demand Standard vCPUs in `eu-north-1` (the Nitro scripts' region; c7g counts in the same Standard family) | C3 (TDX) and N2D (SEV-SNP) quota in the zone you pick |

For the Nitro e2e suite you also need, in the repository root (all gitignored):

- the SSH key, named `pem2.pem` — `aws-nitro.sh create` writes it for you
- `auditor.key` and `deployer.key`, two Ethereum key files with **Sepolia test ETH**. Sepolia is
  the default; `NET=mainnet` spends real ETH and additionally requires `CONFIRM_MAINNET=1`
- `e2e/env.local.sh`, copied from `env.local.example.sh` — `aws-nitro.sh status` prints the lines

### GCP prerequisites (checked 2026-11-12 on the project the recordings were made in)

| what | state found | check it with |
|---|---|---|
| Compute Engine API | enabled | `gcloud services list --enabled --filter=config.name=compute.googleapis.com` |
| SSH to the guests | the default network's `default-allow-ssh` rule (tcp:22 from `0.0.0.0/0`), created with the project; `gcloud compute ssh` relies on it | `gcloud compute firewall-rules list` |
| RA port for a verifier outside the guest | `attestamp-ra`, tcp:5000 from one `/32`, created by `gcp-cvm.sh … create` for the address of the machine that runs it (later addresses are added or removed by hand); not needed when the whole chain runs on the guest (`e2e/cvm/in-guest-verify.sh`) | same |
| `xxd` on the guest (the manual quote recording in `internal/tee/parse/tdx/testdata/README.md`) | present in the stock image `ubuntu-2404-noble-amd64-v20261105` (package `xxd` 2:9.1.0016-1ubuntu7.20); not installed by hand (`/var/log/apt/history.log` shows only `tpm2-tools`) | `command -v xxd` on the guest |
| `tpm2-tools` on the SEV-SNP guest | **not** in the stock image; installed by hand (`apt-get install -y tpm2-tools`, 5.6-1build4) for the Option 2 hook | `dpkg-query -W tpm2-tools` |

### SSH to the guests: do not leave port 22 open to everyone

A new GCP project's default network has `default-allow-ssh`, tcp:22 from `0.0.0.0/0`, for **every** VM in it. The
guests here need SSH only from you. Two ways to narrow it:

**IAP (recommended).** Google's Identity-Aware Proxy carries the SSH connection, so port 22 needs to be open only to
Google's IAP range, `35.235.240.0/20`, and your own address does not matter.

What was checked on 2026-11-16 (Windows, Git Bash, gcloud with PuTTY's plink and pscp, an SEV-SNP guest with an
external address, while `default-allow-ssh` was still open):

| what | result |
|---|---|
| `gcloud compute ssh <vm> --zone … --tunnel-through-iap --command '…'` | works |
| `gcloud compute scp <file> <vm>:<file> --tunnel-through-iap`, both directions | works; a 1 MiB file came back with the same SHA-256 |
| `gcloud compute scp --recurse <folder> <vm>: --tunnel-through-iap` | works |
| where the connection comes from, seen on the guest (`$SSH_CONNECTION`) | `35.235.242.218`, inside `35.235.240.0/20` |
| host key | plink asks to store it again, under `compute.<instance id>` instead of the address: answer `y` |
| the IAP API (`iap.googleapis.com`) | was **not** listed as enabled, and the tunnel worked anyway |
| the first attempt right after the VM started | failed (`Remote side unexpectedly closed network connection`); the second worked. Probably the guest still booting; not proven |

**Not checked:** that SSH still works after `0.0.0.0/0` is removed and only `35.235.240.0/20` is allowed (the project's
rule was left as it was); which IAM role is the least that suffices (the account tested holds `roles/owner`; Google
documents `roles/iap.tunnelResourceAccessor` for this, not tried alone); a guest without an external address; macOS
and Linux clients. With IAP every `gcloud compute ssh` and `scp` in the procedures needs `--tunnel-through-iap`.

The rule it needs, limited to this project's guests by their network tag (not yet applied):

```
gcloud compute firewall-rules create attestamp-ssh-iap --allow=tcp:22 \
  --source-ranges=35.235.240.0/20 --target-tags=attestamp-ra
```

and then `default-allow-ssh` removed or narrowed, which affects every VM in the project, not only these.

**Your own address (`/32`).** The same rule with `--source-ranges=<your address>/32`. It works without IAP, but it
breaks as soon as your public address changes, and it changes with the network you are on: twice on 2026-11-16 alone
(`aws-nitro.sh status` now tells you on AWS; on GCP check `attestamp-ra` and the SSH rule by hand).

## Order

The procedures: Nitro, the flows in `e2e/README.md`; TDX and SEV-SNP, "Getting started" in `docs/build/tdx.md` and
`docs/build/sev-snp.md` (one guest, then "The Auditor's half" on a second). `docs/plan/cvm-port.md` is the plan
behind them. Nitro comes first because it is the baseline the other two are compared against.

```
1. Nitro baseline        bash e2e/cloud/aws-nitro.sh create      # 2 instances, eu-north-1
   (architecture rows)   bash e2e/cloud/aws-nitro.sh create-arm64  # 1 more, c7g.xlarge (Graviton), for ARM64_HOST
2. TDX fixture           bash e2e/cloud/gcp-cvm.sh tdx create    # 2 instances (vendor, auditor), us-central1-a
3. SEV-SNP on GCP        bash e2e/cloud/gcp-cvm.sh snp create    # 2 instances (vendor, auditor), us-central1-a
```

Every word after `create` is a role, and every role is **one billable VM**. Without roles you get `vendor` and
`auditor`; `create vendor` makes only the Vendor's. The script accepts no other role names, so a typo stops it
instead of creating a VM.

**One exact image for both guests.** `gcp-cvm.sh` resolves the image family once, prints the exact image it picked
(for example `ubuntu-2404-noble-amd64-v20261105`) and makes every VM of that call from it. When the second guest is
made later, pass that name: `IMAGE=ubuntu-2404-noble-amd64-v20261105 bash e2e/cloud/gcp-cvm.sh tdx create auditor`.
The family moves to a new version every few weeks, and the disk UUIDs and the stock initrd that end up in TDX RTMR2
and the SEV-SNP CMDLINE come with the image (`docs/build/tdx.md`, "Use the same image as the other side").
`gcp-cvm.sh <plat> status` shows each boot disk's image.

**Names already taken.** The default names (`attestamp-e2e` on AWS, `attestamp-<tdx|snp>-<role>` on GCP) are shared by
everyone using the same account or project. `gcp-cvm.sh` reuses a VM of that name without creating one, and
`aws-nitro.sh` cannot use a key pair whose `.pem` was made on another machine. Give your own names:
`PREFIX=<you>-tdx bash e2e/cloud/gcp-cvm.sh tdx create vendor`, or
`KEY_NAME=<you> SG_NAME=<you> TAG=<you> bash e2e/cloud/aws-nitro.sh create`, and pass the same `PREFIX=` or `TAG=` to
`status`, `stop`, `start` and `delete`: each of them acts only on the VMs or instances with that prefix or tag.

Each platform runs on one cloud: Nitro on AWS, TDX and SEV-SNP on GCP. SEV-SNP on AWS was decided against on
2026-11-11; `aws-snp.sh` is kept only as a record of how that path works.

They do not all have to exist at the same time. Each platform's rows in
[`docs/plan/test-matrix.md`](../../docs/plan/test-matrix.md) can be filled in its own session.

## Cost

Every instance is charged by the hour while it is running. `stop` ends the hourly charge; the disk
keeps costing until you `delete`. There is no free tier for any of these machine types.

Check the current price for your region before creating anything:

- AWS: `aws ec2 describe-instance-types --instance-types m6i.xlarge` for the spec, and the EC2
  pricing page for the rate
- GCP: `gcloud compute machine-types describe c3-standard-4 --zone us-central1-a`, and the
  Compute Engine pricing page

## Not used: AWS SEV-SNP

Kept for reference only; SEV-SNP is done on GCP (decided 2026-11-11). The 2026-11-10 AWS SEV-SNP runs are still a
valid record: they showed the same results as GCP, including the same-VM weakness (see `docs/build/sev-snp.md`).

**`aws-snp.sh delete` deliberately does not remove AMIs and snapshots.** They keep costing storage,
so `status` lists them and you remove them yourself once you are sure you are done with them.

SEV-SNP is an **instance** option and the vTPM is an **AMI** attribute, so they are set at
different times and the stock AMI has no TPM. That is why `aws-snp.sh` has three steps:

```
base   launch a plain SEV-SNP instance from the stock Ubuntu 24.04 AMI
mkami  stop it, snapshot the root volume, and register a new AMI from that snapshot
       with --boot-mode uefi --tpm-support v2.0
vm     launch the real instance from that AMI, with --cpu-options AmdSevSnp=enabled
```

Neither can be turned on after launch: AWS states "You can't enable AMD SEV-SNP after launch", and
TPM support is fixed when the image is registered. If you get it wrong, make a new instance.

GCP needs none of this: `--confidential-compute-type=SEV_SNP` and the vTPM is already there.

## Checking it actually worked

| what | AWS | GCP |
|---|---|---|
| SEV-SNP on? | not used (for reference: `aws ec2 describe-instances --query 'Reservations[].Instances[].CpuOptions'`) | `gcloud compute instances describe <n> --format='value(confidentialInstanceConfig)'` |
| TDX on? | n/a (AWS has no TDX) | same field, value `TDX` |
| vTPM present? | `ls -l /dev/tpm0` on the instance | same |
| Nitro Enclaves on? | `nitro-cli describe-enclaves` after installing the CLI | n/a |
