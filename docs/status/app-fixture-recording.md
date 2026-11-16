# Recording fixtures that carry an application

**Status: run on 2026-11-11; see "Results" at the end.** This is how the **test fixtures** were recorded, with its own
tools (`mkappinitrd`, `make-grub-entry.sh`, `make-verity-image.sh`). To run an application on a guest, use `build` and
`launch --platform …` instead: "Getting started" in `docs/build/tdx.md` and `docs/build/sev-snp.md`. This procedure
covers the four cells of the 2026-10-23 slides 7 and 16:

| | Option 1: application cpio in the initrd | Option 2: squashfs + dm-verity image |
|---|---|---|
| Intel TDX (`intel-tdx`) | archive placed before the base initrd (`cpio.AppFirst`) | the image and its hash tree inside the initrd, also `AppFirst`; RTMR2 covers it, nothing is extended at run time |
| AMD SEV-SNP (`amd-sev-snp-initrd` / `amd-sev-snp-verity`) | archive appended after the base initrd (`cpio.AppLast`) | the image on disk; a verifying initrd checks it against `attestamp.roothash=` on the kernel command line and extends vTPM PCR12 |

## Why: two recordings per cell, one byte apart

Test-matrix row T5 ("application swapped after registration → not trusted") is the project's central claim. On TDX
and SEV-SNP it was empty, because no recorded evidence carried an application.

So every cell is recorded **twice on the same VM, with only the application changed by one byte** (`attestamp app v1`
→ `attestamp app v2`). Expected:

| | Option 1 | Option 2 |
|---|---|---|
| TDX | RTMR2 differs; MRTD, RTMR0, RTMR1 equal | RTMR2 differs; MRTD, RTMR0, RTMR1 equal |
| SEV-SNP | INITRD differs; MEASUREMENT, PCR4, KERNEL, CMDLINE equal | CMDLINE (new root hash) and PCR12 differ; MEASUREMENT, PCR4, KERNEL, INITRD equal |

On TDX one value carries the application (RTMR2) in both options, under the same format and the same four names. On
SEV-SNP Option 2 two values carry it (CMDLINE and PCR12), and `amd-sev-snp-verity` is the first time PCR12 is non-zero
in real evidence.

With the pairs committed, T5 becomes a permanent code test: register v1 on a local chain, present v2, and `verify`
must refuse. From then on it is proved by `go test`, with no VM.

If a value does not change as expected, it is written down as it came out. In particular, if RTMR2 does not differ on
TDX, the assumption in `docs/build/tdx.md`, "The values" (GRUB's measurements land in RTMR2) is wrong.

## Did the application really get unpacked?

Option 1 and TDX Option 2 put the application into the initrd. GRUB measures the whole file whether or not the kernel
manages to unpack the appended archive (`docs/build/tdx.md`, "Before or after the base initrd"). So a changed
measurement alone does not prove the application was there.

**Why `ls /opt/app` after boot proves nothing.** initramfs-tools unpacks the initrd into the initial rootfs, runs
`/init`, mounts the real root on `${rootmnt}` and switches to it (`run-init`, which also deletes the initramfs
contents). After boot `/opt/app` is absent whether or not the archive was unpacked. The procedure checks this on the
guest by reading `/usr/share/initramfs-tools/init` before relying on it.

**What is used instead: a marker carried in the archive.** `mkappinitrd` adds `conf/conf.d/zz-attestamp-app-marker`.
The marker writes the names and sizes of the files in `/opt/app`, and the application's text, to
`/dev/attestamp-app-marker`. This is reliable because of three things, checked on the SEV-SNP guest (Ubuntu 24.04,
2026-11-11):

- initramfs-tools' `/init` sources every file in `conf/conf.d` by glob (line 94), with no ORDER file to edit;
- `/dev` (devtmpfs) is mounted before that (line 40), and udev's init-bottom script moves it onto the real root
  (`mount -n -o move /dev "${rootmnt}/dev"`);
- a regular file can be created in devtmpfs.

So the marker file exists after boot **if and only if** the kernel unpacked our archive.

`/run` would have been the obvious place, and it does not work: `/init` mounts its tmpfs on `/run` only at line 253,
after sourcing `conf/conf.d`, so a file written there is hidden. (The SEV-SNP Option 2 check runs later, at
local-bottom, so its `/run/attestamp-status` does survive.)

`dmesg | grep -i initramfs` is recorded too, but only as a hint: the message text differs between kernel versions, and
the ring buffer can wrap.

**SEV-SNP Option 2 does not have this problem.** The image is on disk, not in the initrd. If verification fails,
`/run/attestamp-status` says `fail` and PCR12 records `fail`. If the verifying script never runs, PCR12 stays zero,
and `ra-parse` refuses that.

## Reproducibility of the Option 2 image

From `docs/build/sev-snp.md`, "Variables", built into `internal/setup/build/cpio/testdata/make-verity-image.sh`:

- squashfs, not ext4: ext4 gave different hashes for two builds even with a fixed UUID and `hash_seed`;
- `mksquashfs -mkfs-time 0 -all-time 0`, every file and directory time set to 0, owners root;
- `veritysetup format --salt=0011…eeff --uuid=11111111-2222-3333-4444-555555555555`: fixed, not random;
- **`umask 022`**, set by the script, with modes set explicitly (755). Folder modes 755 vs 775 gave different root
  hashes in 2026-11, and that remains an open issue for builds that do not set them.

## Tools, built on your machine

From the repository root, one command per line:

```
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o mkappinitrd ./internal/setup/build/cpio/testdata/mkappinitrd
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o record-tdx ./internal/tee/parse/tdx/testdata/record
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o record-snp ./internal/tee/parse/snp/testdata/record
sha256sum mkappinitrd record-tdx record-snp
```

| file | what it does |
|---|---|
| `mkappinitrd` | the application archive (`internal/setup/build/cpio`) with the marker, joined to the base initrd by `cpio.Join`: `-app-text` (Option 1) or `-app-dir` (TDX Option 2); `-placement first` (`AppFirst`) or `last` (`AppLast`). Prints sizes, sha256 and the base size mod 4 |
| `internal/setup/build/cpio/testdata/make-verity-image.sh` | the reproducible squashfs image, hash tree and root hash |
| `internal/tee/parse/snp/testdata/record/initramfs-hook.sh`, `initramfs-local-bottom.sh` | SEV-SNP Option 2: the verifying initrd's hook and its check-mount-extend-PCR12 script |
| `record-tdx` | ConfigFS TSM quote with REPORTDATA = SHA-512 of `-nonce-text` |
| `record-snp` | vTPM attestation (GCE AK) over SHA-256 of `-nonce-text` and an SNP report bound to it by SHA3-512 |

Copy each file to the instance with `gcloud compute scp <file> <instance>:<file> --zone us-central1-a`, one file per
call.

## Common steps on the instance

Run once at the start, as the login user:

```
sudo shutdown -h +240                      # safety stop; every reboot cancels it, so repeat after each one
chmod +x mkappinitrd record-*
sha256sum mkappinitrd record-*             # must equal the local values
uname -r
cat /proc/sys/kernel/random/boot_id

# Check what the marker relies on (conf.d sourced after /dev is mounted; udev moves /dev to the real root):
grep -n -E 'devtmpfs|conf/conf.d|tmpfs.*/run' /usr/share/initramfs-tools/init
grep -n move /usr/share/initramfs-tools/scripts/init-bottom/udev

# GRUB_FORCE_PARTUUID off (docs/build/sev-snp.md, "The boot setting: turning GRUB_FORCE_PARTUUID off"); skip if already commented out
grep -n GRUB_FORCE_PARTUUID /etc/default/grub.d/40-force-partuuid.cfg
sudo cp -p /etc/default/grub.d/40-force-partuuid.cfg /root/40-force-partuuid.cfg.orig
sudo sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' /etc/default/grub.d/40-force-partuuid.cfg
sudo update-grub

# The stock entry, as the template for ours
sudo awk "/^menuentry 'Ubuntu'/{f=1} f{print} f&&/^}/{exit}" /boot/grub/grub.cfg > stock-entry.txt
cat stock-entry.txt                        # one menuentry with a linux and an initrd line
```

An entry made from the template, changing only the title, the initrd file and, for SEV-SNP Option 2, the command line.
`internal/setup/build/cpio/testdata/make-grub-entry.sh` does it (copy it to the instance with the other files); below,
`make_entry <id> <initrd file> [extra arguments]` stands for `sudo sh make-grub-entry.sh <id> <initrd file> [extra
arguments]`. It writes `/etc/grub.d/41_<id>` and `/etc/default/grub.d/99-attestamp.cfg`, runs `update-grub` and prints
the new entry.

The entry is made the default rather than booted once with `grub-reboot`. A one-shot entry is lost at the next
maintenance restart, and a stale `initrdfail` can redirect it (`docs/build/tdx.md`, "`GRUB_FORCE_PARTUUID`").

After every reboot, before recording:

```
sudo shutdown -h +240
cat /proc/sys/kernel/random/boot_id        # must differ from the previous boot
cat /proc/cmdline
cat /dev/attestamp-app-marker              # Option 1 and TDX Option 2: present and naming the expected text
sudo dmesg | grep -i -E 'initramfs|unpack' # a hint only
```

## AMD SEV-SNP (`attestamp-snp-vendor`): first

Files to copy: `mkappinitrd`, `record-snp`, `make-verity-image.sh`, `initramfs-hook.sh`, `initramfs-local-bottom.sh`.
Turn `GRUB_FORCE_PARTUUID` off (common steps): it is on in every new guest of the image. On `attestamp-snp-vendor`, the
instance the 2026-11-11 recordings were made on, it had already been turned off that day.

### Option 1 (`amd-sev-snp-initrd`, `AppLast`)

```
sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v1" -placement last -out /boot/initrd-attestamp-app.img
make_entry attestamp-app initrd-attestamp-app.img
sudo systemctl reboot
# after the reboot: the checks above, then
sudo ./record-snp -out opt1-v1 -nonce-text attestamp-snp-opt1-v1

sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v2" -placement last -out /boot/initrd-attestamp-app.img
sudo systemctl reboot
# after the reboot: the checks, then
sudo ./record-snp -out opt1-v2 -nonce-text attestamp-snp-opt1-v2
```

### Option 2 (`amd-sev-snp-verity`), in the same session

```
sudo apt-get update                                                  # tpm2-tools is not found without it
sudo apt-get install -y squashfs-tools cryptsetup-bin tpm2-tools     # before installing the hook below
dpkg-query -W squashfs-tools cryptsetup-bin tpm2-tools
sudo sh make-verity-image.sh "attestamp app v1" verity-v1
sudo mkdir -p /var/lib/attestamp /opt/app-verity                    # the root is read-only when the check runs
sudo cp verity-v1/app.img verity-v1/app.hash /var/lib/attestamp/
sudo install -m 755 initramfs-hook.sh /etc/initramfs-tools/hooks/attestamp-verity
sudo install -m 755 initramfs-local-bottom.sh /etc/initramfs-tools/scripts/local-bottom/attestamp-verity
sudo mkinitramfs -o /boot/initrd-attestamp-verity.img $(uname -r)    # the verifying initrd, used for v1 and v2
make_entry attestamp-verity initrd-attestamp-verity.img "attestamp.roothash=$(cat verity-v1/roothash)"
sudo systemctl reboot
# after the reboot
cat /run/attestamp-status                  # must be ok
cat /run/attestamp-log
cat /opt/app-verity/opt/app/app            # attestamp app v1
sudo ./record-snp -out opt2-v1 -nonce-text attestamp-snp-opt2-v1

sudo sh make-verity-image.sh "attestamp app v2" verity-v2
sudo cp verity-v2/app.img verity-v2/app.hash /var/lib/attestamp/
make_entry attestamp-verity initrd-attestamp-verity.img "attestamp.roothash=$(cat verity-v2/roothash)"
sudo systemctl reboot
# after the reboot: the same three checks, then
sudo ./record-snp -out opt2-v2 -nonce-text attestamp-snp-opt2-v2
```

## Intel TDX (`attestamp-tdx-vendor`): second

Files to copy: `mkappinitrd`, `record-tdx`, `make-verity-image.sh`. Turn `GRUB_FORCE_PARTUUID` off (common steps).

### Option 1 (`AppFirst`)

```
sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v1" -placement first -out /boot/initrd-attestamp-app.img
make_entry attestamp-app initrd-attestamp-app.img
sudo systemctl reboot
sudo ./record-tdx -out opt1-v1 -nonce-text attestamp-tdx-opt1-v1     # after the checks
sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v2" -placement first -out /boot/initrd-attestamp-app.img
sudo systemctl reboot
sudo ./record-tdx -out opt1-v2 -nonce-text attestamp-tdx-opt1-v2
```

### Option 2 (image inside the initrd, `AppFirst`), in the same session

```
sudo apt-get install -y squashfs-tools cryptsetup-bin
dpkg-query -W squashfs-tools cryptsetup-bin
sudo sh make-verity-image.sh "attestamp app v1" verity-v1
sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-dir verity-v1 -placement first -out /boot/initrd-attestamp-app.img
sudo systemctl reboot                      # same entry as Option 1
sudo ./record-tdx -out opt2-v1 -nonce-text attestamp-tdx-opt2-v1     # after the checks; the marker names app.img, app.hash, roothash
sudo sh make-verity-image.sh "attestamp app v2" verity-v2
sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-dir verity-v2 -placement first -out /boot/initrd-attestamp-app.img
sudo systemctl reboot
sudo ./record-tdx -out opt2-v2 -nonce-text attestamp-tdx-opt2-v2
```

This records what TDX Option 2 measures: the image and hash tree carried inside the initrd. Opening and mounting the
image before the switch to the real root is Phase 3.4 and is not part of this recording; it was recorded separately on
2026-11-12, next section.

### TDX Option 2 mounted at boot (`gcp-c3-opt2m-*`, 2026-11-12)

On `attestamp-tdx-vendor`, stock image `ubuntu-2404-noble-amd64-v20261105`, kernel `7.0.0-1017-gcp`. The scripts are
`internal/tee/parse/tdx/testdata/record/initramfs-{hook,local-bottom}.sh` (SEV-SNP's, without PCR12).

1. The hook and the local-bottom script installed into `/etc/initramfs-tools`, `mkinitramfs -o
   initrd-verity-base.img`, and both removed again at once. The base carries `veritysetup`, `dm-verity.ko` and the
   script; squashfs, loop and dm-mod are built into this kernel. (For a real build, "Getting started" step 4 in
   `docs/build/tdx.md` makes this base.)
2. The images `verity-v1` and `verity-v2` of the Option 2 recordings above (sha256 checked) put before the base with
   `mkappinitrd -placement first`, and the boot entry made with `make-grub-entry.sh attestamp-verity
   initrd-attestamp-verity.img`.
3. One boot each:

| boot | `/run/attestamp-status` | mounted | content | RTMR2 |
|---|---|---|---|---|
| stock | — | — | — | `9960b923…` |
| v1 | `ok e29ec085…` | `/opt/app-verity`, squashfs ro on `/dev/mapper/attestamp-app`, `dmsetup status` `V` | `attestamp app v1` | `fc0d75eb…` |
| v2 | `ok a2f4df92…` | same | `attestamp app v2` | `71cd0493…` |
| tamper: v2's image with v1's root hash | `fail e29ec085…`, log "Verification of root hash failed." | nothing; no dm device | — | `01964c57…` |

MRTD, RTMR0 and RTMR1 were the same on every boot. On the v2 boot `e2e/cvm/in-guest-verify.sh tdx intel-tdx` passed
(11 of 11), and the live quote's RTMR2 equalled the recording's. **The tampered boot carried on to a normal login**: the
script records `fail` and exits 0. That is enough for a verifier (RTMR2 differs from every registered value), but a
locked guest (Phase 5 in `docs/status/cvm-port.md`) must stop the boot instead.

Committed as `gcp-c3-opt2m-v1-*` and `gcp-c3-opt2m-v2-*` with their boot logs; `TestMountedFixtures_OnlyRTMR2Changes`
checks them. The tamper quote was not committed; its RTMR2 is above. Undone afterwards as in the next section, with
`41_attestamp-verity` and `initrd-attestamp-verity.img` in place of the `-app` files.

## Undo, fetch, stop (each instance)

```
sudo rm -f /etc/grub.d/41_attestamp-app /etc/grub.d/41_attestamp-verity /etc/default/grub.d/99-attestamp.cfg
sudo rm -f /etc/initramfs-tools/hooks/attestamp-verity /etc/initramfs-tools/scripts/local-bottom/attestamp-verity
sudo rm -f /boot/initrd-attestamp-app.img /boot/initrd-attestamp-verity.img
sudo update-grub
sha256sum opt*/*
```

Fetch each recorded file with `gcloud compute scp`, one file per call, into `gcp-n2d-<opt1|opt2>-<v1|v2>-<file>`
(SEV-SNP) or `gcp-c3-<opt1|opt2>-<v1|v2>-<file>` (TDX), compare sha256 with the instance, then
`gcloud compute instances stop <instance> --zone us-central1-a`. `GRUB_FORCE_PARTUUID` stays off.

## Afterwards, with no VM

1. Commit the eight recordings under `internal/tee/parse/{tdx,snp}/testdata/`, with their identifying information
   listed in each `README.md`.
2. Parser tests for each pair: the expected values differ and the rest are equal (table at the top). On SEV-SNP,
   INITRD of Option 1 must equal the "combined" sha256 `mkappinitrd` printed, and PCR12 of Option 2 must equal the
   formula in `initramfs-local-bottom.sh`, each computed outside the parser.
3. A `cmd` chain test per cell: register v1 on anvil, serve v2 with `ra-serve --extractor fixture=...`, and `verify`
   must exit 1. That is T5 as code.
4. Write the base size mod 4 and the marker and `dmesg` results into `docs/build/tdx.md`, "Before or after the base
   initrd", whatever they show.

## Results (2026-11-11)

All recorded in one session per platform. The instances ran from start to stop (GCP's `lastStartTimestamp` and
`lastStopTimestamp`): `attestamp-snp-vendor` 07:00:39–08:18:20 UTC, `attestamp-tdx-vendor` 08:31:47–08:52:50 UTC. Both guests booted
the same base initrd (50,100,283 bytes, size mod 4 = 3, sha256 `4703ec07…b0a2`). Every recording verifies with its
nonce, and its boot log is committed next to it (`*-boot.txt`).

### The four cells of the 2026-10-23 slides (7 and 16), against what was measured

| cell | slides claimed | measured | right or wrong |
|---|---|---|---|
| **TDX, Option 1** (cpio, `AppFirst`) | works | the archive was unpacked on both boots (marker present); only RTMR2 changed between v1 and v2 (MRTD, RTMR0, RTMR1 equal); T5 refused v2 | **right** |
| **TDX, Option 2** (image inside the initrd, `AppFirst`) | works; RTMR2 covers the image, nothing extended at run time | image, hash tree and root hash unpacked on both boots (marker); only RTMR2 changed; T5 refused v2. Mounting the image inside the TDX boot was not built or tested | **right for what is measured**; the run-time use of the image is not built |
| **SEV-SNP, Option 1** (cpio, `AppLast`) | works on AWS and GCP | **as appended in 2026-11 (no padding), the kernel discarded the archive**: `Initramfs unpacking failed: invalid magic`, no marker, yet INITRD changed with the application. With the base zero-padded to 4 bytes: unpacked (marker present), only INITRD changed, T5 refused v2. `cpio.Join(AppLast)` now pads | **wrong as presented** (the application never ran); right once padded |
| **SEV-SNP, Option 2** (image on disk, PCR12) | works on AWS and GCP | `status ok` and the application mounted on both boots; PCR12 equal to the formula; CMDLINE and PCR12 changed, MEASUREMENT, PCR4, KERNEL, INITRD did not; first real `amd-sev-snp-verity` evidence; T5 refused v2 | **right** (on GCP; AWS is no longer used for SEV-SNP) |

### Expected against measured

| | expected | measured |
|---|---|---|
| TDX Option 1 | RTMR2 differs; MRTD, RTMR0, RTMR1 equal | as expected |
| TDX Option 2 | RTMR2 differs; MRTD, RTMR0, RTMR1 equal | as expected |
| SEV-SNP Option 1 | INITRD differs; MEASUREMENT, PCR4, KERNEL, CMDLINE equal | as expected, in both the unpadded and the padded pair. The unpadded pair's application never ran, which the evidence cannot show |
| SEV-SNP Option 2 | CMDLINE and PCR12 differ; MEASUREMENT, PCR4, KERNEL, INITRD equal | as expected; PCR12 equal to SHA-256(0³² ‖ SHA-256("attestamp-verity-ok:" ‖ root hash)) for both root hashes |

### Also measured in the session

- **Placement, not platform, decides.** The same base initrd failed unpadded with `AppLast`, worked padded, and worked
  with `AppFirst` on four TDX boots (`docs/build/tdx.md`, "Before or after the base initrd").
- **The marker works, and `/run` would not have.** initramfs-tools' `/init` sources `conf/conf.d` before it mounts
  `/run`, so the marker goes to `/dev` (checked on both guests).
- **The Option 2 image is reproducible across machines.** `make-verity-image.sh`, with `umask 022`, gave the same
  `app.img`, `app.hash` and root hash on both guests.
- **TDX with `AppLast` was not tested.** The 2026-10 failure on TDX remains the Phase 3.2 record.

### Not done

- the image of TDX Option 2 is carried and measured, not opened or mounted at boot (done since, on 2026-11-12:
  `gcp-c3-opt2m-*`, with a tamper boot);
- the SEV-SNP Option 2 tamper case (a changed image under an unchanged root hash) was not recorded;
- one boot ID (SEV-SNP Option 2 v2) was not captured; that boot's command line, carrying the v2 root hash, shows it
  happened.

## Reproducing T10: evidence from two VMs combined

**Status: run on 2026-11-16; the relay is accepted (T10 reproduced).** The result and its reading:
`docs/build/ra-comparison.md`, "The SEV-SNP relay, measured"; the run: `docs/results/sessions-2026-11-16/amd-sev-snp.md`;
the recordings: "The 2026-11-16 recordings" at the end of this section. Written 2026-11-15. Test-matrix row T10 asks whether a vTPM attestation from one VM, combined with
an SNP report from another, is refused. The only answer so far is the 2026-11 record ("it verifies"), made with a
relay script that no longer exists. It is the project's main claim about SEV-SNP's weakness, so it has to be measured
here.

**What T10 is, and what `TestVerifySig_MixedRecordingsFailTheBinding` is not.** That test puts the report of one
recording next to the vTPM attestation of another recording **of the same VM**. Each report was requested over its
own attestation, so the SHA3-512 binding fails, as it must. The relay is different: VM B is asked for a report whose
REPORT_DATA is the SHA3-512 of **VM A's** attestation. The binding then holds by construction, and the question is
whether anything else in the evidence ties the two halves to the same VM. Two VMs are needed for that, and the
recorder has a mode for it (`-relay-att`, `internal/tee/parse/snp/testdata/record/main.go`).

### Where each compared value comes from in a relay (written before the run, 2026-11-16)

T10 is not "does it work" but "is it refused, as the row expects". Either outcome is possible, and which one says
something different, so the sources of the values are written down first. In the relay document, **B's** SNP report is
bound to **A's** vTPM attestation (REPORT_DATA = SHA3-512 of A's `att.pb`):

| value | taken from | signed by | so in the relay it is |
|---|---|---|---|
| MEASUREMENT | the SNP report | AMD (B's chip's VCEK) | **B's** firmware digest. Equal to A's when both run the same firmware on the same machine type (every recording so far: `f6790085…`), so a registration cannot tell them apart |
| PCR4, KERNEL, CMDLINE, INITRD (and PCR12 for verity) | the vTPM event log, replayed against the vTPM quotes | Google's AK (A's vTPM) | **A's** boot |
| the nonce | the vTPM quotes' `extraData` (`snp/nonce.go`); the report has none and covers it only through the binding | Google's AK | **A's** quote over the verifier's nonce |
| debug | the report's POLICY | AMD | **B's** policy |

What the verifier checks that touches both halves (`snp/verify.go`): the AMD signature on the report, the AK chain to
Google's root, the quotes' signatures by that AK, and REPORT_DATA = SHA3-512(vTPM attestation). The last is the only
link between the two halves, and the attacker satisfies it by construction.

**The nonce decides what a live relay needs.** Since the nonce is inside A's vTPM quotes, a relay that answers a
verifier's **fresh** nonce needs A alive at that moment: the attacker forwards the nonce to A, takes A's attestation,
and asks B for a report over it (a real-time man in the middle). Replaying an old attestation of A fails the nonce
check (exit 6). The recorder below uses one fixed nonce text for A and the relay, which stands for that forwarding.

So the two possible results mean:

- **refused**: there is a check tying the halves to one VM that this table does not list; find which step refused
  (the exit code and message) and write it down;
- **accepted**: nothing in the evidence ties B's report to A's vTPM. That is a property of the platform as GCP offers
  it, not a defect to fix here; record it as it is (`docs/build/sev-snp.md`, "Same VM").

### Before you start

- **A second SEV-SNP VM is a billable cloud resource: ask before creating it** (`e2e/cloud/README.md`). The first VM is
  `attestamp-snp-vendor`. The second one can be made with `bash e2e/cloud/gcp-cvm.sh snp create auditor` (same
  machine type, image and zone; it becomes `attestamp-snp-auditor`).
- On VM A, `GRUB_FORCE_PARTUUID` must be off (`docs/build/sev-snp.md`, "The boot setting: turning `GRUB_FORCE_PARTUUID` off"), so that its vTPM
  event log measures an initrd and `ra-parse` gets as far as the binding. `attestamp-snp-vendor` already has it off.
  VM B's boot setting does not matter: only its SNP report is used.
- On both: `sudo shutdown -h +240` as a safety stop.
- Build the recorder and the packer on your machine:

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o record-snp ./internal/tee/parse/snp/testdata/record
```

Copy `record-snp` to both VMs (`gcloud compute scp`, one file per call).

### Record (one nonce text for everything)

```
# VM A: its own, honest pair (the control that must verify)
sudo ./record-snp -out t10-a -nonce-text attestamp-snp-t10

# VM B: its own, honest pair (a second control; and the source of the "naive mix" below)
sudo ./record-snp -out t10-b -nonce-text attestamp-snp-t10

# copy t10-a/att.pb from A to B as t10-a-att.pb (through your machine), then on VM B: the relay
sudo ./record-snp -out t10-relay -nonce-text attestamp-snp-t10 -relay-att t10-a-att.pb
```

`t10-relay` then holds A's `att.pb` unchanged and B's `report.bin` and `auxblob.bin`, with B's REPORT_DATA equal to
SHA3-512 of A's attestation; `record.txt` says `relay_att`. Also save on each VM, for the record: `uname -r`,
`cat /proc/cmdline`, `/proc/sys/kernel/random/boot_id`, and `sudo dmesg | grep -i sev`.

Copy the three folders back, then **stop both VMs**. Deleting the VMs is a separate decision, not part of this procedure.

### Check, with no VM

```
go run ./internal/tee/parse/snp/testdata/pack -report t10-a/report.bin -certs t10-a/auxblob.bin -vtpm t10-a/att.pb -out a.cbor
go run ./internal/tee/parse/snp/testdata/pack -report t10-relay/report.bin -certs t10-relay/auxblob.bin -vtpm t10-relay/att.pb -out relay.cbor
go run ./internal/tee/parse/snp/testdata/pack -report t10-b/report.bin -certs t10-b/auxblob.bin -vtpm t10-a/att.pb -out naive.cbor
att-user ra-parse --doc <each> --format amd-sev-snp-initrd --nonce <the nonce from record.txt>
```

| document | what it is | expected |
|---|---|---|
| `a.cbor` | VM A, honest | accepted (exit 0) |
| `naive.cbor` | B's own report with A's attestation | refused at the binding (exit 5, "REPORT_DATA is not SHA3-512"), as in the same-VM test |
| `relay.cbor` | **T10**: B's report over A's attestation | the row expects **refused**; the 2026-11 record says it was **accepted** |

Record for `relay.cbor`, whatever happens:

- the exit code and the full message, and **which step refused it, if any**: the AMD chain or report signature (5),
  the AK chain or quotes (5), the binding (5, "REPORT_DATA"), the nonce (6), the measurements (7);
- if accepted: the values it yields, next to `a.cbor`'s. MEASUREMENT is B's firmware digest and the vTPM values are
  A's; whether the two VMs' MEASUREMENT is equal tells whether a verifier comparing against a registration could even
  notice;
- `CHIP_ID` of A's and B's report (64 bytes at offset 0x1A0): whether the two VMs ran on the same physical machine;
- what in A's vTPM evidence names A (the AK certificate carries the instance's identity from Google), and whether
  anything in B's report could be compared with it. If nothing can, that is the structural reason T10 fails, and it
  goes into `docs/build/sev-snp.md`, "Same VM".

### Commit, so T10 runs with no VM (not done: decided against on 2026-11-16)

The operator decided not to commit the recordings: each `att.pb` carries the project ID and an instance's name and
numeric ID in the Google-signed AK certificate (checked: subject and the GCE instance-information extension), and
adding identifiers to the history before the history question is settled is not to be done. T10 stays a live result,
like Nitro's T8. What committing would have meant, kept for when that decision changes:

- the three folders as fixtures `internal/tee/parse/snp/testdata/gcp-n2d-t10-{a,b,relay}-*`, with a `-boot.txt` each,
  in the same layout as the other recordings;
- a test in `internal/tee/parse/snp` that pins what was observed: if the relay verifies, a test named for the known
  weakness that asserts it is accepted (so a future fix shows up as a deliberate test change); if it is refused, a test
  that asserts the refusal and the step;
- the T10 cell in `docs/status/test-matrix.md` becomes **code**, with the date of the recording.

**Identifiers.** Every `att.pb` carries the project ID inside the Google-signed AK certificate, and may carry the
instance's identity too; check with `grep -a` before committing, as for the other SEV-SNP fixtures
(`internal/tee/parse/snp/testdata/README.md`). An instance ID must not be committed (`CONTRIBUTING.md`, "Secrets and certificates"); if the
certificate holds one, committing these fixtures waits for the same publication decision as the others.

### The 2026-11-16 recordings (kept locally, not committed)

Made with the procedure above on `d3-snp-auditor` (VM A, running the registered application) and `d3-snp-vendor` (VM
B, running a one-byte-changed one), two `n2d-standard-2` guests of `ubuntu-2404-noble-amd64-v20261105`, `record-snp`
`5b0e8f5d…` built at `57b4350`, nonce text `attestamp-snp-t10` (nonce `0f8964068dccac4d42f09277c59a1e7d73e8ca5a163056d418ae173239df8766`).
They are on the development machine that made them, under `e2e-work/t10/` (gitignored), the only copy: the two VMs, which held the same
folders, were deleted on 2026-11-16 after these hashes were checked against the local files.

| file | sha256 |
|---|---|
| `a/att.pb` (A's vTPM attestation; also `relay/att.pb`, unchanged) | `a1ff314e2d45a9eca6f2b3b4e85e672a314f18277422f5a4a563b56625350983` |
| `a/report.bin` | `59295a51b0854438b8ccc0c710787b923c937ea1951348b3b67704f7746f38bb` |
| `a/auxblob.bin` | `ea15e9de6481d6625d0df7a863797760f9fa863f7b2c4d52ab5b0619ae2adf03` |
| `b/att.pb` | `f13734bcd1f373c72a56ea906fe9aa9fc0bad792bd70118b6540d11333042320` |
| `b/report.bin` | `89c29b3dda784e388058cceceb114ed3e9bf2f8ce4b9aaf24e5f732dc9f0f110` |
| `b/auxblob.bin` (also `relay/auxblob.bin`) | `980d9dff4f000ddfe69dcbd9b90cdb0525a3e0f7d7c622ddbd9b2bd61dcc3eac` |
| `relay/report.bin` (B's report over A's attestation) | `e487d6c8e4bec527579246abad9049e57a30d27c1c21dc3713df5a77fdb200d7` |
| `a.cbor` (packed) | `ab06f1af0599c455cfb30549ae8ffc7defeb44726ba57944421226de36dda32d` |
| `b.cbor` | `8354dbf79497072952d0f66298b491fd9517f4c1179ccad2e5232919801d43af` |
| `naive.cbor` | `8d4e51c28625a1a4eb858d7d35325848392b4a2db4a67a939a9540cce733d9ad` |
| `relay.cbor` | `1dcaecdc570563ba1115272b47819992a43d7d621b3079b2c0f40685d963fda6` |

REPORT_DATA of both `a/report.bin` and `relay/report.bin` is `62b9a1f3…2c14`, SHA3-512 of `a/att.pb`. What each
document gave under `att-user ra-parse --format amd-sev-snp-initrd --nonce <the nonce above>`: `a.cbor` exit 0,
`b.cbor` exit 0, `naive.cbor` exit 5 (the binding), `relay.cbor` **exit 0 with A's values**.

**To run it again** (two SEV-SNP guests of one image, with approval): follow "Before you start" and "Record" above,
then "Check, with no VM". Two things that differ on Windows: `gcloud compute scp` through PuTTY takes **one remote
source per call**, so fetch each folder with `gcloud compute scp --recurse <vm>:t10-a <local folder>`; and VM A must be
booted into an initrd (our entry), or every format refuses its evidence (exit 7) before the relay question is reached.
To check that a relay's values would be trusted, `att-user lookup --measurements relay.json --platform
amd-sev-snp-initrd` against the registry that holds A's registration.
