# CVM flow 1 on two SEV-SNP guests, first runs (rehearsal), 2026-11-19

The first runs of the CVM flow scripts (`e2e/cvm/`, "Flow 1 on TDX and SEV-SNP" in `e2e/README.md`) on SEV-SNP guests,
after the two TDX runs of the day before (`cvm-flow-rehearsal-2026-11-18.md`): `amd-sev-snp-verity` (Option 2) first,
then `amd-sev-snp-initrd` (Option 1) on the same two guests. Times are UTC, 2026-11-19 (2026-11-19 and 11-20 in Korea).
Results of the verity run, all as expected:
[`cvm-flow-rehearsal-2026-11-19-verity.jsonl`](cvm-flow-rehearsal-2026-11-19-verity.jsonl), 7 lines.

| | |
|---|---|
| guests | `attestamp-snp-vendor`, `attestamp-snp-auditor`, made in one call (`IMAGE=ubuntu-2404-noble-amd64-v20261105 gcp-cvm.sh snp create --yes`), `n2d-standard-2`, `us-central1-a`, tag `attestamp-ra`; the API reports both `SEV_SNP`. The two TDX guests of 2026-11-18 were deleted first (two instances at most) |
| chain | `NET=local`: an anvil 1.8.0 already running on the machine running the flow (Sepolia was not used) |
| scripts | at `38ef80b` |
| the machine running the flow | Windows, Git Bash, gcloud through IAP (PuTTY plink and pscp) |
| the `verify` scenes | typed by the user on the machine running the flow, not by the flow |

## `amd-sev-snp-verity`

| step | verdict | time |
|---|---|---|
| `0-Setup-guests.sh amd-sev-snp-verity` | `apt` installed `squashfs-tools` 1:4.6.1-1build1, `cryptsetup-bin` 2:2.7.0-1ubuntu4.2 and `tpm2-tools` 5.6-1build4 on both guests; the Vendor made `base-initrd-v.img` (`16408aed…`) and the Auditor got that file | 1207 s |
| 1 Auditor | root hash `5dff1f1d…` (the value of TDX Option 2 and of the WSL build of 2026-11-15: the same application), PCR12 `33784785…`; boot, `launch --check`, `audit` registered | 313 s |
| 2 Vendor | the same six values; `compare`, `lookup` and the chain comparison passed | 285 s |
| (restart, R6) | both guests started again; `check-serve` only, no build or install; the six values unchanged on both | 79 s, 26 s |
| 3, typed | exit 0, `trusted`; the six values equal to step 2's; approver anvil account 1 | |
| 2b Auditor | `same`, exit 0 | 18 s |
| 4 Vendor | one byte changed: root hash `aba41856…`, PCR12 `d1c4e101…`, CMDLINE `22dea1d6…` | 281 s |
| 5, typed | exit 1, `nobody` | |
| 4b Auditor | `runs something else`, exit 12 | 17 s |

The six values of steps 1 and 2: CMDLINE `d12176cc…`, INITRD `16408aed…`, KERNEL `3cf8c2c3…`, MEASUREMENT `f6790085…`,
PCR12 `33784785…`, PCR4 `5b65d2d3…`. After the one-byte change only CMDLINE (it carries the root hash) and PCR12 moved.

Inside the steps, in seconds:

| step | prepare (Vendor, Auditor) | application | build | install and reboot (waiting for the new boot) | check and responder |
|---|---|---|---|---|---|
| setup | 920, 43 | | | | |
| 1 Auditor | | 23 | 22 | 208 (161) | 26 |
| 2 Vendor | | 23 | 22 | 194 (150) | 26 |
| 4 Vendor | | 22 | 23 | 192 (148) | 25 |

Against TDX Option 2 (`cvm-flow-rehearsal-2026-11-18.md`): each step about as long (281 to 313 s against 293 to 312 s);
the new boot answered after 148 to 161 s against 111 to 138 s; the time goes to the reboot and to gcloud.

### What the verity hook did, checked by hand on the guests

- **`/run/attestamp-status`** read `ok` on both guests after every boot, without the root hash: the SEV-SNP hook
  (`internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh`) writes only the status, as its two recordings
  show; the TDX hook writes `ok <root hash>`. The root hash the boot used was read from `/proc/cmdline`
  (`attestamp.roothash=…`).
- **PCR12**: `tpm2_pcrread sha256:12` gave `0x33784785…` on both guests after steps 1 and 2, and `0xD1C4E101…` on the
  Vendor's after step 4: each time the `pcr12` of `build-value.json`, which `build` computes with `PCR12(root hash)`
  (`internal/setup/build/snp/build.go`) from the root hash of that build.
- **the image**: mounted read-only on `/opt/app-verity` from `/dev/mapper/attestamp-app` (squashfs); the application
  file is at `/opt/app-verity/opt/app/hello.txt`, and after step 4 it read `Hello from attestamp`.

## Findings

R3 (gcloud crashing as a session closed, `cvm-flow-rehearsal-2026-11-18.md`) did not occur in the verity run. One
connection could not be set up and was tried again once (the Auditor's `check-serve` after the restart, hence its
79 s).

R5. **`launch` on SEV-SNP names TDX's register.** Step 2 of `launch` says GRUB "measures its SHA-384 into RTMR2" also on
SEV-SNP; the text is in the GRUB code both platforms share (`internal/setup/launch/grub/install.go`). Wording only.
Left until both SEV-SNP runs are done, so that they run the same binaries.

R6. **The four-hour safety stop turned both guests off** while the flow waited at step 3 (`guest-step.sh` sets
`shutdown -h +240` in `prepare` and `check-serve`). Started again, each guest got a new public address, and the step-3
`verify` line had to be made again with the Vendor's new one (read with `guest_ip`); the firewall rule's one source
address was still this machine's. `check-serve` alone, with no build or install, gave the same six values on both
guests: a stop and start of the guest moves no measurement.

R7. **The Vendor's `prepare` took 920 s**, the Auditor's 43 s. `apt` itself took 5 s (`/var/log/apt/history.log`), and
neither `unattended-upgrades` nor `apt-daily` ran; where the rest went was not looked into.

R8. **`0-Setup-guests.sh` could not be run again while a responder ran.** Its header says it "can be run again before
every take"; at the start of the initrd run it failed copying `att-ra-responder` to the Vendor's guest (`pscp: unable to
open att-ra-responder`), because the verity run's responder was still running from that file and Linux refuses to
write to a running executable (`Text file busy`). Not met on TDX, where setup only ran on freshly started guests.
