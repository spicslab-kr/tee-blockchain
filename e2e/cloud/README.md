# Creating the instances

Scripts that create, stop and delete the instances each platform needs. Every command they run is
printed in the script's header comment, and every flag was taken from the vendor's own
documentation (see the comments for which page).

**These scripts create billable resources.** `.claude/settings.json` marks every `create`,
`run-instances`, `delete` and `terminate-instances` as a command Claude must ask about before
running. Do not loosen that.

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
| RA port for a verifier outside the guest | `attestamp-ra`, tcp:5000 from one `/32`, added by hand; not needed when the whole chain runs on the guest (`e2e/cvm/in-guest-verify.sh`) | same |
| `xxd` on the guest (the manual quote recording in `internal/tee/parse/tdx/testdata/README.md`) | present in the stock image `ubuntu-2404-noble-amd64-v20261105` (package `xxd` 2:9.1.0016-1ubuntu7.20); not installed by hand (`/var/log/apt/history.log` shows only `tpm2-tools`) | `command -v xxd` on the guest |
| `tpm2-tools` on the SEV-SNP guest | **not** in the stock image; installed by hand (`apt-get install -y tpm2-tools`, 5.6-1build4) for the Option 2 hook | `dpkg-query -W tpm2-tools` |

## Order

Follow `docs/plan/cvm-port.md`. Nitro comes first because it is the baseline the other two are
compared against.

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
