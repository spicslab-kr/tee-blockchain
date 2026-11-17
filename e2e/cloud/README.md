# Creating the instances

Scripts that create, stop and delete the instances each platform needs. Each script's header comment says how to call
it and which of the vendor's documentation its flags were checked against; the `gcloud` and `aws` calls themselves are
in the script, one function or case per action.

**These scripts create billable resources.** Every `create`, `run-instances`, `delete` and
`terminate-instances` they run costs money or destroys state. Run them only when you mean to, and if another program
runs commands for you, have it ask before it runs any of these scripts' creating or deleting actions: a rule
that asks before `gcloud … create` or `aws ec2 run-instances` does not see the calls a script makes inside. Nothing is
created without `--yes` ("Nothing is created without `--yes`", below).

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
| Compute Engine API | enabled. In a new project it is off: enable it once with `gcloud services enable compute.googleapis.com` (the first `gcloud compute` command also offers to) | `gcloud services list --enabled --filter=config.name=compute.googleapis.com` |
| SSH to the guests | a new project's default network has `default-allow-ssh` (tcp:22 from `0.0.0.0/0`), and plain `gcloud compute ssh` relies on it. **Changed on 2026-11-16 in the project the recordings were made in:** that rule is deleted and `attestamp-ssh-iap` (tcp:22 from `35.235.240.0/20`, tag `attestamp-ra`) replaces it, so every `gcloud compute ssh` and `scp` there needs `--tunnel-through-iap` (next section) | `gcloud compute firewall-rules list` |
| RA port for a verifier outside the guest | `attestamp-ra`, tcp:5000 from one `/32`, created by `gcp-cvm.sh … create` for the address of the machine that runs it (later addresses are added or removed by hand); not needed when the whole chain runs on the guest (`e2e/cvm/in-guest-verify.sh`) | same |
| `xxd` on the guest (the manual quote recording in `internal/tee/parse/tdx/testdata/README.md`) | present in the stock image `ubuntu-2404-noble-amd64-v20261105` (package `xxd` 2:9.1.0016-1ubuntu7.20); not installed by hand (`/var/log/apt/history.log` shows only `tpm2-tools`) | `command -v xxd` on the guest |
| `tpm2-tools` on the SEV-SNP guest | **not** in the stock image; installed by hand (`apt-get install -y tpm2-tools`, 5.6-1build4) for the Option 2 hook | `dpkg-query -W tpm2-tools` |

### SSH to the guests: do not leave port 22 open to everyone

(This and the other settings this project changes are collected, with what breaks if each differs, in
`docs/build/settings-and-hashes.md`, "Network access".)

A new GCP project's default network has `default-allow-ssh`, tcp:22 from `0.0.0.0/0`, for **every** VM in it. The
guests here need SSH only from you. Two ways to narrow it:

**IAP (recommended).** Google's Identity-Aware Proxy carries the SSH connection, so port 22 needs to be open only to
Google's IAP range, `35.235.240.0/20`, and your own address does not matter.

The two rules, as applied on 2026-11-16 to the project the recordings were made in:

```
gcloud compute firewall-rules create attestamp-ssh-iap --network=default --direction=INGRESS --allow=tcp:22 \
  --source-ranges=35.235.240.0/20 --target-tags=attestamp-ra
gcloud compute firewall-rules delete default-allow-ssh
```

The first allows SSH only from Google's IAP range and only to VMs tagged `attestamp-ra` (`gcp-cvm.sh` tags every guest
it makes). The second removes SSH from everywhere for **every** VM in the project's default network, not only these:
check that nothing else in the project needs it first.

What was checked on 2026-11-16 (Windows, Git Bash, gcloud with PuTTY's plink and pscp, a SEV-SNP guest with an
external address). First while `default-allow-ssh` was still there, then again **after it was deleted**, with
`attestamp-ssh-iap` the only rule for port 22:

| what | with `0.0.0.0/0` still open | after it was deleted |
|---|---|---|
| `gcloud compute ssh <vm> --zone … --tunnel-through-iap --command '…'` | works | **works** |
| `gcloud compute scp <file> <vm>:<file> --tunnel-through-iap`, both directions | works; a 1 MiB file came back with the same SHA-256 | **works**; the same check |
| `gcloud compute scp --recurse <vm>:<folder> . --tunnel-through-iap` | works (to the guest) | **works** (from the guest; a folder with a subfolder, the same SHA-256) |
| plain `gcloud compute ssh` without `--tunnel-through-iap` | works | **fails**, `Network error: Connection timed out`: port 22 is closed to the internet |
| where the connection comes from, seen on the guest (`$SSH_CONNECTION`) | `35.235.242.218` | `35.235.242.217`; both inside `35.235.240.0/20` |
| host key | plink asks to store it again, under `compute.<instance id>` instead of the address: answer `y` | the same |
| the IAP API (`iap.googleapis.com`) | was **not** listed as enabled, and the tunnel worked anyway | the same |
| right after the VM starts | the first attempt failed (`Remote side unexpectedly closed network connection`), the second worked | attempts failed for a few minutes after the start, then worked; the guest was still booting |

**Not checked:** which IAM role is the least that suffices (the account tested holds `roles/owner`; Google documents
`roles/iap.tunnelResourceAccessor` for this, not tried alone); a guest without an external address; macOS and Linux
clients. With IAP every `gcloud compute ssh` and `scp` in the procedures needs `--tunnel-through-iap`; the RA port
(`attestamp-ra`, tcp:5000) is a separate rule and does not go through IAP.

To undo it: `gcloud compute firewall-rules create default-allow-ssh --network=default --allow=tcp:22
--source-ranges=0.0.0.0/0 --priority=65534`, the rule as GCP made it.

**Your own address (`/32`).** The same rule with `--source-ranges=<your address>/32`. It works without IAP, but it
breaks as soon as your public address changes, and it changes with the network you are on: twice on 2026-11-16 alone
(`aws-nitro.sh status` now tells you on AWS; on GCP check `attestamp-ra` and the SSH rule by hand).

### From another computer, or after your address changed

The firewall rules that matter here allow **single addresses** (`/32`), starting with the address of the machine that
created them; nothing in them follows you to another computer or another network. The symptom is always the same:
`ssh` (or a `verify` from outside) **times out**, with no other message.

- **AWS (Nitro).** `aws-nitro.sh create` opened ports 22 and 5000 to the creating machine's `/32` and reuses the group
  afterwards. From any other computer, or after your address changed, run `bash e2e/cloud/aws-nitro.sh status` (or
  `start`, which runs the same check). It says whether this machine's public address is allowed, prints the exact
  `authorize-security-group-ingress … --cidr <your address>/32` commands for whatever is missing, lists the addresses
  the group allows now, and prints the `revoke-security-group-ingress` command to remove one you no longer use. It
  changes nothing itself: whoever may change the group in that AWS account runs the commands. If it finds a rule open
  to `0.0.0.0/0` it says so and prints the command that removes it; never add one.
- **GCP, SSH.** Through IAP (above) your address does not matter, once the project has the IAP rule.
- **GCP, the RA port.** `attestamp-ra` allows tcp:5000 from the `/32` of the machine that ran the first `gcp-cvm.sh
  create`. Only a verifier outside the guest needs it (the procedures can run the whole chain on the guest). To allow
  another address, list what is there with `gcloud compute firewall-rules describe attestamp-ra
  --format='value(sourceRanges)'` and set the full list again, one `/32` each:
  `gcloud compute firewall-rules update attestamp-ra --source-ranges=<old>/32,<new>/32`. `gcp-cvm.sh` does not check
  this for you.

## Order

The procedures: Nitro, the flows in `e2e/README.md`; TDX and SEV-SNP, "Getting started" in `docs/build/tdx.md` and
`docs/build/sev-snp.md` (one guest, then "The Auditor's half" on a second). Nitro comes first because it is the baseline the other two are compared against.

```
1. Nitro baseline        bash e2e/cloud/aws-nitro.sh create --yes        # 2 instances, eu-north-1
   (architecture rows)   bash e2e/cloud/aws-nitro.sh create-arm64 --yes  # 1 more, c7g.xlarge (Graviton), for ARM64_HOST
2. TDX fixture           bash e2e/cloud/gcp-cvm.sh tdx create --yes  # 2 instances (vendor, auditor), us-central1-a
3. SEV-SNP on GCP        bash e2e/cloud/gcp-cvm.sh snp create --yes  # 2 instances (vendor, auditor), us-central1-a
```

Every word after `create` is a role, and every role is **one billable VM**. Without roles you get `vendor` and
`auditor`; `create vendor` makes only the Vendor's. The script accepts no other role names and no role twice, so a
typo stops it before anything runs.

### Nothing is created without `--yes`: the same rules in all three scripts

`gcp-cvm.sh create`, `aws-nitro.sh create` and `create-arm64`, and `aws-snp.sh base`, `mkami` and `vm` behave alike:

- **Without `--yes` they create nothing.** They print how many billable resources they would make and their names (or
  tags), and stop with exit 3. Run once without `--yes` to read the plan, then again with it.
- **`--dry-run`** prints the same plan and calls neither `gcloud` nor `aws`, not even to sign in.
- **Every check runs before the first CLI call**: an unknown role or option, a role given twice, and `--yes` or
  `--dry-run` given to `status`, `stop`, `start` or `delete` all stop the script with exit 2. The last is refused
  rather than ignored, so `delete --dry-run` cannot really delete; and `gcp-cvm.sh delete vendor` is refused because
  `delete` acts on every VM of the prefix, not on one role.
- **There is no pause to press Ctrl-C in.** The scripts used to warn and wait 4 seconds (`aws-snp.sh` did not even do
  that). On 2026-11-16 a command run without a terminal went through the pause unseen and created two VMs nobody meant
  to make (`docs/results/build-info-gaps-2026-11.md`, #20).

**Why a flag and not a pause, or an approval prompt.** An approval rule in a tool's settings, such as
"ask before `gcloud compute instances create`", sees only the command a person or a program typed, for example
`bash e2e/cloud/gcp-cvm.sh snp create`. It does not see the `gcloud` or `aws` calls the script makes inside, so it
never asks. A pause protects only someone watching a terminal. A flag in the typed command is the one thing both a
person and an approval rule can see.

**Testing these scripts without creating anything.** Run `bash e2e/cloud/check-scripts.sh` (`go test ./cmd` runs it
too, as `TestCloudScripts`). It runs every action of the three scripts against stand-in `aws`, `gcloud` and `curl`
commands that only log their arguments, checks each exit status and the CLI calls each action made, and fails when a
script has an action that no case runs as far as its CLI call. The stand-ins must be what `PATH` finds, or it stops
before running anything. It was written on 2026-11-17: the check by hand the night before ran `create` only, and
`stop` and `start` in both AWS scripts still failed at once on `$1` (fixed in `bc87898`); run on those versions, it
fails on exactly those four. A new action needs a new case there.

For a one-off look at a single command, use `--dry-run` first. To go further by hand, put stand-in `gcloud` and
`aws` scripts (that only print or log their arguments) first on `PATH`, and check with `which gcloud aws` that they are
the ones found **before** running anything. In Git Bash write the folder as `/c/Users/...`, never `C:/Users/...`:
`PATH` is split at every colon, so `C:/Users/.../fakebin:$PATH` becomes `C` and `/Users/.../fakebin`, neither of which
exists, and the real CLI runs. That is what happened on 2026-11-16. Safer still, set `PATH` to the stand-in folder and
`/usr/bin` only, so the real CLIs (under `C:/Program Files`) cannot be found at all, and point `PEM_OUT` outside the
repository so a key file is not written into it.

**One exact image for both guests.** `gcp-cvm.sh` resolves the image family once, prints the exact image it picked
(for example `ubuntu-2404-noble-amd64-v20261105`) and makes every VM of that call from it. When the second guest is
made later, pass that name: `IMAGE=ubuntu-2404-noble-amd64-v20261105 bash e2e/cloud/gcp-cvm.sh tdx create auditor --yes`.
The family moves to a new version every few weeks, and the disk UUIDs and the stock initrd that end up in TDX RTMR2
and the SEV-SNP CMDLINE come with the image (`docs/build/tdx.md`, "Use the same image as the other side").
`gcp-cvm.sh <plat> status` shows each boot disk's image.

**Names already taken.** The default names (`attestamp-e2e` on AWS, `attestamp-<tdx|snp>-<role>` on GCP) are shared by
everyone using the same account or project. `gcp-cvm.sh` reuses a VM of that name without creating one, and
`aws-nitro.sh` cannot use a key pair whose `.pem` was made on another machine. Give your own names:
`PREFIX=<you>-tdx bash e2e/cloud/gcp-cvm.sh tdx create vendor --yes`, or
`KEY_NAME=<you> SG_NAME=<you> TAG=<you> bash e2e/cloud/aws-nitro.sh create --yes`, and pass the same `PREFIX=` or `TAG=` to
`status`, `stop`, `start` and `delete`: each of them acts only on the VMs or instances with that prefix or tag.

Each platform runs on one cloud: Nitro on AWS, TDX and SEV-SNP on GCP. SEV-SNP on AWS was decided against on
2026-11-11; `aws-snp.sh` is kept only as a record of how that path works.

They do not all have to exist at the same time. Each platform's rows in
[`docs/status/test-matrix.md`](../../docs/status/test-matrix.md) can be filled in its own session.

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
