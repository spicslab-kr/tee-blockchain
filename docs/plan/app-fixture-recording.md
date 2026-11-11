# Recording fixtures that carry an application

**Status: see "Results" at the end.** This procedure covers the four cells of the 2026-10-23 slides 7 and 16:

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
`/run/attestamp-app-marker`. This is reliable because of two things in initramfs-tools' `/init`:

- it sources every file in `conf/conf.d` by glob, with no ORDER file to edit;
- it moves its `/run` tmpfs onto the real root.

So the marker file exists after boot **if and only if** the kernel unpacked our archive. The procedure checks both
properties in `/init` on the guest first. (The 2026-11 SEV-SNP scripts already read `/run/attestamp-status`, written
in the initramfs, after boot.)

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

# Check the two properties the marker relies on, in initramfs-tools' /init:
grep -n -E 'conf/conf.d|mount -t tmpfs.*/run|move /run|run-init' /usr/share/initramfs-tools/init

# GRUB_FORCE_PARTUUID off (docs/build/sev-snp.md, "The boot setting changed"); skip if already commented out
grep -n GRUB_FORCE_PARTUUID /etc/default/grub.d/40-force-partuuid.cfg
sudo cp -p /etc/default/grub.d/40-force-partuuid.cfg /root/40-force-partuuid.cfg.orig
sudo sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' /etc/default/grub.d/40-force-partuuid.cfg
sudo update-grub

# The stock entry, as the template for ours
sudo awk "/^menuentry 'Ubuntu'/{f=1} f{print} f&&/^}/{exit}" /boot/grub/grub.cfg > stock-entry.txt
cat stock-entry.txt                        # one menuentry with a linux and an initrd line
```

An entry made from the template, changing only the title, the initrd file and, for SEV-SNP Option 2, the command line:

```
# make-entry <id> <initrd file name> [extra kernel arguments]  -> /etc/grub.d/41_<id>
make_entry() {
  printf '#!/bin/sh\nexec tail -n +3 $0\n' > "41_$1"
  sed -e "s/^menuentry 'Ubuntu'.*{\$/menuentry '$1' --id $1 {/" \
      -e "s#initrd\.img-[^ ]*#$2#" \
      -e "/^[[:space:]]*linux[[:space:]]/ s#\$#${3:+ $3}#" stock-entry.txt >> "41_$1"
  sudo install -m 755 "41_$1" "/etc/grub.d/41_$1"
  echo "GRUB_DEFAULT=$1" | sudo tee /etc/default/grub.d/99-attestamp.cfg
  sudo update-grub
  sudo grep -n -A12 "menuentry '$1'" /boot/grub/grub.cfg
}
```

The entry is made the default rather than booted once with `grub-reboot`. A one-shot entry is lost at the next
maintenance restart, and a stale `initrdfail` can redirect it (`docs/build/tdx.md`, "`GRUB_FORCE_PARTUUID`").

After every reboot, before recording:

```
sudo shutdown -h +240
cat /proc/sys/kernel/random/boot_id        # must differ from the previous boot
cat /proc/cmdline
cat /run/attestamp-app-marker              # Option 1 and TDX Option 2: present and naming the expected text
sudo dmesg | grep -i -E 'initramfs|unpack' # a hint only
```

## AMD SEV-SNP (`attestamp-snp-vendor`): first

Files to copy: `mkappinitrd`, `record-snp`, `make-verity-image.sh`, `initramfs-hook.sh`, `initramfs-local-bottom.sh`.
`GRUB_FORCE_PARTUUID` is already off on this instance (since 2026-11-11).

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
image before the switch to the real root is Phase 3.4 and is not part of this recording.

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

## Results

Not recorded yet.
