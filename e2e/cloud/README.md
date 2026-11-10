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
| quota | SEV-SNP shared tenancy exists only in `us-east-2` and `eu-west-1` | C3 (TDX) and N2D (SEV-SNP) quota in the zone you pick |

For the Nitro e2e suite you also need, in the repository root (all gitignored):

- the SSH key, named `pem2.pem` — `aws-nitro.sh create` writes it for you
- `auditor.key` and `deployer.key`, two Ethereum key files with **Sepolia test ETH**. Sepolia is
  the default; `NET=mainnet` spends real ETH and additionally requires `CONFIRM_MAINNET=1`
- `e2e/env.local.sh`, copied from `env.local.example.sh` — `aws-nitro.sh status` prints the lines

## Order

Follow `docs/plan/cvm-port.md`. Nitro comes first because it is the baseline the other two are
compared against.

```
1. Nitro baseline        bash e2e/cloud/aws-nitro.sh create      # 2 instances, eu-north-1
2. TDX fixture           bash e2e/cloud/gcp-cvm.sh tdx create    # 2 instances, us-central1-a
3. SEV-SNP on GCP        bash e2e/cloud/gcp-cvm.sh snp create vendor auditor user y
4. SEV-SNP on AWS        bash e2e/cloud/aws-snp.sh base → mkami → vm
```

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

**`aws-snp.sh delete` deliberately does not remove AMIs and snapshots.** They keep costing storage,
so `status` lists them and you remove them yourself once you are sure you are done with them.

## The awkward one: AWS SEV-SNP

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
| SEV-SNP on? | `aws ec2 describe-instances --query 'Reservations[].Instances[].CpuOptions'` | `gcloud compute instances describe <n> --format='value(confidentialInstanceConfig)'` |
| TDX on? | n/a (AWS has no TDX) | same field, value `TDX` |
| vTPM present? | `ls -l /dev/tpm0` on the instance | same |
| Nitro Enclaves on? | `nitro-cli describe-enclaves` after installing the CLI | n/a |
