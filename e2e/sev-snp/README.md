# SEV-SNP closed-VM test: Ollama answers that prove which code made them

Attestamp does not support AMD SEV-SNP yet. This folder is the test we ran on GCP and AWS SEV-SNP
instances to find out whether it can work: a confidential VM runs Ollama, every answer comes with
evidence of the code that produced it, and the User can check that this code cannot have been
changed since boot. Nothing here is used by `att-vendor`, `att-auditor` or `att-user`.

## How it works

- **Closed VM.** The application (responder, Ollama, the model) is packed into the initrd, and the
  kernel starts the responder directly (`rdinit=/attest-init`). There is no disk mount, no shell,
  no SSH server, no login and no cloud agent. Changing the code needs a reboot.
- **Measurement.** On these clouds the SEV-SNP measurement covers only the firmware. grub records
  the kernel and initrd hashes in vTPM PCR 9 and the kernel command line in PCR 8 (GCP Shielded
  vTPM, AWS NitroTPM). A reboot into anything else gives different hashes.
- **One answer, one proof.** For each question the User sends a fresh nonce. The responder computes
  `d = SHA-256("attestamp-ollama-v1" || nonce || SHA-256(prompt) || SHA-256(answer))`, puts `d` into
  the vTPM quote (GCP) or the NitroTPM document (AWS), and puts SHA-512 of that evidence into the
  SEV-SNP report data. The User checks the Google or AWS signature, replays the boot event log, compares
  kernel, initrd and command line with the registered values, and checks the AMD signature.
- **Signed status report.** `/status` returns the running processes, listening ports, mounts and
  kernel settings, bound to a nonce in the same way. So "only this is running" is also evidence.
- **Kernel locks.** As PID 1 the responder disables module loading, kexec and ptrace, turns off
  SysRq and sets kernel lockdown to `integrity`. None of these can be turned back on without a reboot.

## Files

| path | runs on | what it does |
|---|---|---|
| `gcp/responder`, `aws/responder` | Vendor VM (inside the closed image) | Ollama, `/ask`, `/status`, evidence, kernel locks. On AWS it also mounts the system folders, loads `ena` and `sev-guest` and does DHCP, because dracut's init is skipped |
| `gcp/ask`, `aws/ask` | User | asks, verifies, prints the verdict. `-status`, `-tamper`, `-replay`, `-save` |
| `gcp/attest-init` | Vendor VM (inside the image) | GCP first process: mounts, network, then `exec` the responder |
| `gcp/gcp_prepare.sh`, `aws/aws_prepare.sh` | Vendor VM, once | Ollama (CPU files), model, libraries, base initrd, grub entry |
| `gcp/gcp_closed_build.sh`, `aws/aws_closed_build.sh` | Vendor VM | build the closed initrd, compute the expected values, boot it once |
| `gcp/gcp_suite.sh`, `gcp/probe_closed.sh` | a second VM in the same VPC | the GCP checks below |
| `aws/aws_suite.sh`, `aws/monitor.sh` | User machine | the AWS checks below, and a signed status report every 20 s |
| `aws/aws_admin_probe.sh` | AWS CloudShell | EC2 Instance Connect, SSM, serial console, then reboot |
| `common/mkcpio.py` | Vendor VM | the same cpio bytes every time (sorted, time 0, owner 0) |
| `common/make_vendor_modified.sh` | build machine | a responder that appends " [vendor-modified]" to every answer |

The prepare scripts collect the commands we ran by hand. They were not rerun as a whole.

## Running it

Build on your machine (Go 1.26.1, from this folder's `gcp` or `aws`):

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o snp-aproxy ./responder
go build -o snp-ask ./ask
```

GCP (Ubuntu 24.04 `n2d` with SEV-SNP):

1. Upload `common/mkcpio.py` to the Vendor's home, `attest-init` and the responder to `~/closed`, and run `gcp_prepare.sh`.
2. `gcp_closed_build.sh boot` builds the image, writes `~/closed/expected-closed.json` and reboots into it once.
   If the VM's default boot entry is not grub, set `NEXT_BOOT` to the grub (shim) EFI entry.
3. Copy `expected-closed.json` and a Linux `snp-ask` to a second VM in the same VPC, then
   `TARGET=<internal IP> bash gcp_suite.sh before`.
4. `gcloud compute instances reset` brings the VM back to its default entry.

AWS (Amazon Linux 2023 `m6a` with SEV-SNP and NitroTPM):

1. Upload `common/mkcpio.py` to the Vendor's home, the responder as `~/closed-aproxy`, and run `aws_prepare.sh`.
2. `aws_closed_build.sh debug` boots once with `attestamp_debuglog=1`: the responder answers one
   question itself, writes the log and the answer to `/var/tmp`, and reboots. Use it to check the
   image without a console. `aws_closed_build.sh closed` boots the real image once.
3. Open TCP 8443 to your address, then `TARGET=<public IP> bash aws_suite.sh before`.
4. In CloudShell: `INSTANCE=... REGION=... TARGET=... bash aws_admin_probe.sh`. It ends with a reboot.

## What we saw (2026-11-12)

| check | GCP | AWS |
|---|---|---|
| answer with a fresh nonce | approved, all checks pass | approved, all checks pass |
| answer changed in transit, saved answer replayed | rejected | rejected |
| signed status report | responder and Ollama only, no disk mounted, locks on | same |
| all TCP ports from outside | only 8443 | only 8443 |
| SSH, console SSH button / EC2 Instance Connect | connection refused | connection refused |
| startup script in metadata / SSM Run Command | not run, no agent | instance not registered, command refused |
| serial console | no login, typed commands not run | blocked: account setting off |
| status report before and after these attempts | no new process | same processes and PIDs |
| Ollama management API through 8443 | 404 | 404 |
| reboot into a vendor-modified responder | signatures valid, initrd hash differs, rejected | same (checked with the debug boot) |
| reboot back into the registered image | same hash, approved | same hash, approved |
| rebuild from the same inputs | same initrd hash | same initrd hash |

Not tested: the host writing guest memory (SEV-SNP is meant to block this), and bugs in the
responder or Ollama exploited through the question.

## Things that went wrong, so you don't repeat them

- **cpio order.** A cpio appended after a compressed initrd is not unpacked. Put ours first.
- **TPM lockout.** A closed VM ends only by hard reset, and the TPM counts each one as a failure.
  Both vTPMs lock after 3 (about 17 minutes each to recover). GCP's attestation key is affected:
  `gcp_closed_build.sh boot` clears the count from normal mode first. On AWS the responder's NV
  index is created with `NoDA`.
- **AWS reboot takes about 4 minutes.** The closed VM ignores the ACPI request, so EC2 hard-reboots
  after its timeout.
- **Ollama calls out.** At start it asks ollama.com for model recommendations. In the closed VM this
  fails (no DNS) and the answers are not affected.
- **Line endings.** Scripts edited on Windows got CRLF and stopped with `set: -: invalid option`.
