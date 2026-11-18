# CVM flow 1 on two TDX guests, first run (rehearsal), 2026-11-18

The first run of the CVM flow scripts (`e2e/cvm/`, "Flow 1 on TDX and SEV-SNP" in `e2e/README.md`) on real guests,
to find what breaks before a recording, and how long each step takes. Times are UTC: 2026-11-18, the last step ending
at 16:08Z (2026-11-19 in Korea). Results: [`cvm-flow-rehearsal-2026-11-18.jsonl`](cvm-flow-rehearsal-2026-11-18.jsonl), 7 lines,
all as expected.

| | |
|---|---|
| guests | `attestamp-tdx-vendor`, `attestamp-tdx-auditor`, made in one call (`gcp-cvm.sh tdx create --yes`), `c3-standard-4`, `us-central1-a`, image `ubuntu-2404-noble-amd64-v20261105`, tag `attestamp-ra` |
| format | `intel-tdx`, Option 1 |
| chain | `NET=local`: an anvil on the machine running the flow (Sepolia was not used for the rehearsal) |
| scripts | at `5109a2b` plus the fix of R3 below, committed as `a5973db` after step 2; the first try of step 1 ran before both |
| the machine running the flow | Windows, Git Bash, gcloud through IAP (PuTTY plink and pscp) |

## Result

Every step passed, and the two scenes typed by hand gave what the flow printed to expect:

| step | verdict | time |
|---|---|---|
| `0-Setup-guests.sh intel-tdx` | both guests prepared; base initrd `4703ec07…` on both | 458 s |
| 1 Auditor | build, boot, `launch --check` (RTMR0 `8847ec5d…`, RTMR2 `0f36ec64…`), `audit` registered | 377 s |
| 2 Vendor | the same build and values; `compare`, `lookup` and the chain comparison passed | 281 s |
| 3, typed | the printed `verify` line: exit 0, `trusted` | |
| 2b Auditor | `audit --vendor-tee`: `same`, exit 0 | 20 s |
| 4 Vendor | one byte changed: RTMR2 `dd366486…` | 282 s |
| 5, typed | the same `verify` line: exit 1, `nobody … registered` | |
| 4b Auditor | `audit --vendor-tee`: `runs something else`, exit 12 | 19 s |

Inside the steps, in seconds:

| step | application | build | install and reboot (waiting for the new boot) | check and responder | the rest |
|---|---|---|---|---|---|
| 1 Auditor | 42 | 41 | 202 (144) | 45 | audit 2, fetch, deploy, lookup |
| 2 Vendor | 28 | 26 | 167 (114) | 33 | request, parse, compare, lookup: about 1 |
| 4 Vendor | 28 | 29 | 162 (111) | 40 | |

Writing one small file (the application step) takes 28 to 42 s: each `gcloud compute scp` or `ssh` through IAP costs
about 15 to 20 s from this machine. The reboots took 111 to 144 s to answer with a new boot ID.

## What ran for the first time, and how it went

- **gcloud ssh and scp through IAP from Git Bash:** every copy and command ran. `--command` lines with `;`, `$?`, `>`
  and `&` reached the guest intact through plink, and exit codes came back (a remote `exit 3` returned 3).
- **prepare:** `update-grub` after turning `GRUB_FORCE_PARTUUID` off, the base initrd copied from `/boot`. Option 1
  only, so `apt` and `mkinitramfs` (Option 2) did not run.
- **build on the guest, the GRUB entry, the reboot by `systemd-run`, the boot_id wait:** as designed.
- **`launch --check` and the responder:** passed each time; port 5000 answered the machine running the flow.

## Findings

R1. **pscp's host-key prompt.** The second copy to each new guest printed "The host key is not cached … Store key in
cache?". Standard input is `/dev/null`, so it did not wait, and the copy went on. Noise on screen only. Not changed.

R2. **pscp's progress meter**, one per copied file, buries the step banners. Noise on screen only. Not changed.

R3. **gcloud crashed as the check-serve session closed.** The local gcloud process ended with "Segmentation fault"
(exit 139) after `check-serve` had finished on the guest, 3 times out of 4, and after no other action. The first time
it failed step 1, although the guest had checked the boot and started the responder and port 5000 answered. Starting a
background process over ssh by hand did not reproduce it; the cause on the gcloud side is not known. **Fixed**
(`a5973db`): each guest action writes its exit code to a file on the guest, and when gcloud fails the script goes by
that file; a missing file still fails the step. With it, steps 1, 2 and 4 passed although the crash recurred.

Not exercised by this run: Option 2 (`apt`, `mkinitramfs`, the Auditor taking the Vendor's `base-initrd-v.img`),
SEV-SNP, Sepolia, and a User on another computer (the two `verify` scenes were typed on the machine that ran the flow).

## Afterwards

The firewall rule `attestamp-ra` was changed to the address of the machine running the flow before the run (its
earlier definition kept outside the repository). Both guests were stopped, not deleted.
