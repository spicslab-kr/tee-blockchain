# Recording fixtures that carry an application

**Status: prepared, not run.** Nothing in this document has been executed on an instance yet (2026-11-11). The tools
it uses compile; the commands have not been typed on a guest.

## Why: two recordings per platform, one byte apart

Test-matrix row T5 ("application swapped after registration → not trusted") is the project's central claim, and on
TDX and SEV-SNP it is empty. None of the recorded evidence carries an application: `gcp-c3-quote.bin` and
`gcp-n2d-*` booted the distribution initrd.

So record **twice per platform, on the same VM, with only the application changed by one byte** (`attestamp app v1`
→ `attestamp app v2`). Then:

- on TDX, RTMR2 must differ between the two quotes while MRTD, RTMR0 and RTMR1 stay equal;
- on SEV-SNP, INITRD must differ while MEASUREMENT, PCR4, KERNEL and CMDLINE stay equal.

With the two recordings committed, T5 becomes a permanent code test: register v1 on a local chain, present v2, and
`verify` must refuse. From then on it is proved by `go test` with no VM. One VM session per platform is all it takes.

What the application is does not matter. What the recordings prove is that a changed application changes the
measured value, so the application is one file, `/opt/app/app`, with one line of text.

If RTMR2 does **not** differ on TDX, write that down: it would contradict the assumed mapping of GRUB's
measurements into RTMR2 (`docs/build/tdx.md`, "The values"), which has not been confirmed on GCP.

The same session also measures what `docs/build/tdx.md`, "Before or after the base initrd", left open: the base
initrd's size modulo 4, and whether the kernel logged `Initramfs unpacking failed` for each placement.

## Tools, built on your machine

From the repository root (Git Bash, macOS or Linux), one command per line:

```
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o mkappinitrd ./internal/setup/build/cpio/testdata/mkappinitrd
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o record-tdx ./internal/tee/parse/tdx/testdata/record
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o record-snp ./internal/tee/parse/snp/testdata/record
sha256sum mkappinitrd record-tdx record-snp
```

| tool | what it does |
|---|---|
| `mkappinitrd` | writes the one-file application archive with `internal/setup/build/cpio` and joins it to the base initrd with `cpio.Join` (`-placement first` = `AppFirst`, `last` = `AppLast`). Prints sizes, sha256 and the base size mod 4 |
| `record-tdx` | asks ConfigFS TSM (`tdx_guest`) for a quote with REPORTDATA = SHA-512 of `-nonce-text`; writes `quote.bin`, `record.txt` |
| `record-snp` | the existing SEV-SNP recorder: vTPM attestation (GCE AK) over SHA-256 of `-nonce-text`, then an SNP report with REPORT_DATA = SHA3-512 of it; writes `att.pb`, `report.bin`, `auxblob.bin`, `record.txt` |

All three are under `testdata`, so `go build ./...` and `go test ./...` skip them.

## Intel TDX (`attestamp-tdx-vendor`, `cpio.AppFirst`)

Ask before starting the instance (it is billed while running). From your machine:

```
gcloud compute instances start attestamp-tdx-vendor --zone us-central1-a
gcloud compute scp mkappinitrd attestamp-tdx-vendor:mkappinitrd --zone us-central1-a
gcloud compute scp record-tdx attestamp-tdx-vendor:record-tdx --zone us-central1-a
gcloud compute ssh attestamp-tdx-vendor --zone us-central1-a
```

On the instance:

```
sudo shutdown -h +240                      # safety stop; a reboot cancels it, so repeat after every reboot
chmod +x mkappinitrd record-tdx
sha256sum mkappinitrd record-tdx           # must equal the local values
uname -r                                   # write down; 7.0.0-1017-gcp when the first fixture was recorded
cat /proc/sys/kernel/random/boot_id        # write down

# 1. Turn GRUB_FORCE_PARTUUID off (same as docs/build/sev-snp.md, "The boot setting changed")
grep -n GRUB_FORCE_PARTUUID /etc/default/grub.d/40-force-partuuid.cfg
sudo cp -p /etc/default/grub.d/40-force-partuuid.cfg /root/40-force-partuuid.cfg.orig
sudo sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' /etc/default/grub.d/40-force-partuuid.cfg
sudo update-grub                           # must print "Found initrd image: /boot/initrd.img-..."

# 2. The application initrd, v1. Writes the combined file next to the stock initrd.
sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v1" -placement first -out /boot/initrd-attestamp-app.img
#    write down all four printed lines, in particular "size mod 4"

# 3. A GRUB entry that boots it, made from the stock entry with only the initrd path changed
sudo awk "/^menuentry 'Ubuntu'/{f=1} f{print} f&&/^}/{exit}" /boot/grub/grub.cfg > stock-entry.txt
cat stock-entry.txt                        # check: one menuentry with a linux and an initrd line
printf '#!/bin/sh\nexec tail -n +3 $0\n' > 41_attestamp_app
sed -e "s/^menuentry 'Ubuntu'.*{\$/menuentry 'attestamp-app' --id attestamp-app {/" -e 's#initrd\.img-[^ ]*#initrd-attestamp-app.img#' stock-entry.txt >> 41_attestamp_app
cat 41_attestamp_app                       # check: the title and the initrd line changed, nothing else
sudo install -m 755 41_attestamp_app /etc/grub.d/41_attestamp_app
echo 'GRUB_DEFAULT=attestamp-app' | sudo tee /etc/default/grub.d/99-attestamp-app.cfg
sudo update-grub
sudo grep -n -A12 "menuentry 'attestamp-app'" /boot/grub/grub.cfg

# 4. Boot it
sudo systemctl reboot
```

The entry is made the default rather than booted once with `grub-reboot`: a one-shot entry is lost at the next
maintenance restart, and a stale `initrdfail` can redirect it (`docs/build/tdx.md`, "`GRUB_FORCE_PARTUUID`").

After the reboot, `gcloud compute ssh attestamp-tdx-vendor --zone us-central1-a` again:

```
sudo shutdown -h +240
cat /proc/sys/kernel/random/boot_id        # must differ from before
cat /proc/cmdline                          # the stock command line (root=UUID=...)
sudo dmesg | grep -i -E "initramfs|unpack" # write down; "Initramfs unpacking failed" means the archive was dropped
sudo ./record-tdx -out app-v1 -nonce-text attestamp-tdx-app-v1

# 5. v2: one byte different, same file name, same entry
sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v2" -placement first -out /boot/initrd-attestamp-app.img
sudo systemctl reboot
```

After the second reboot:

```
sudo shutdown -h +240
cat /proc/sys/kernel/random/boot_id
cat /proc/cmdline                          # must equal the v1 command line
sudo dmesg | grep -i -E "initramfs|unpack"
sudo ./record-tdx -out app-v2 -nonce-text attestamp-tdx-app-v2
sha256sum app-v1/* app-v2/*

# 6. Undo the boot entry (GRUB_FORCE_PARTUUID stays off, as on the SEV-SNP instance)
sudo rm /etc/grub.d/41_attestamp_app /etc/default/grub.d/99-attestamp-app.cfg /boot/initrd-attestamp-app.img
sudo update-grub
exit
```

From your machine, one file per call, then stop:

```
gcloud compute scp attestamp-tdx-vendor:app-v1/quote.bin gcp-c3-app-v1-quote.bin --zone us-central1-a
gcloud compute scp attestamp-tdx-vendor:app-v1/record.txt gcp-c3-app-v1-record.txt --zone us-central1-a
gcloud compute scp attestamp-tdx-vendor:app-v2/quote.bin gcp-c3-app-v2-quote.bin --zone us-central1-a
gcloud compute scp attestamp-tdx-vendor:app-v2/record.txt gcp-c3-app-v2-record.txt --zone us-central1-a
sha256sum gcp-c3-app-v*                    # must equal the instance's values
gcloud compute instances stop attestamp-tdx-vendor --zone us-central1-a
```

## AMD SEV-SNP (`attestamp-snp-vendor`, `cpio.AppLast`)

Ask before starting the instance. `GRUB_FORCE_PARTUUID` is already off on this instance (since 2026-11-11), so step 1
is a check only. From your machine:

```
gcloud compute instances start attestamp-snp-vendor --zone us-central1-a
gcloud compute scp mkappinitrd attestamp-snp-vendor:mkappinitrd --zone us-central1-a
gcloud compute scp record-snp attestamp-snp-vendor:record-snp --zone us-central1-a
gcloud compute ssh attestamp-snp-vendor --zone us-central1-a
```

On the instance, the same steps as TDX with these differences:

```
sudo shutdown -h +240
chmod +x mkappinitrd record-snp
sha256sum mkappinitrd record-snp
grep -n GRUB_FORCE_PARTUUID /etc/default/grub.d/40-force-partuuid.cfg   # must show the line commented out

sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v1" -placement last -out /boot/initrd-attestamp-app.img
# step 3 (GRUB entry) exactly as for TDX, then reboot

# after the reboot
sudo shutdown -h +240
cat /proc/sys/kernel/random/boot_id
cat /proc/cmdline
sudo dmesg | grep -i -E "initramfs|unpack" # with -placement last this is the line that tests the alignment hypothesis
sudo ./record-snp -out app-v1 -nonce-text attestamp-snp-app-v1

sudo ./mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v2" -placement last -out /boot/initrd-attestamp-app.img
sudo systemctl reboot

# after the second reboot
sudo shutdown -h +240
cat /proc/sys/kernel/random/boot_id
cat /proc/cmdline
sudo dmesg | grep -i -E "initramfs|unpack"
sudo ./record-snp -out app-v2 -nonce-text attestamp-snp-app-v2
sha256sum app-v1/* app-v2/*

sudo rm /etc/grub.d/41_attestamp_app /etc/default/grub.d/99-attestamp-app.cfg /boot/initrd-attestamp-app.img
sudo update-grub
exit
```

From your machine, one file per call (`att.pb`, `report.bin`, `auxblob.bin`, `record.txt` for `app-v1` and `app-v2`,
saved as `gcp-n2d-app-v1-<file>` and `gcp-n2d-app-v2-<file>`), compare sha256 with the instance, then
`gcloud compute instances stop attestamp-snp-vendor --zone us-central1-a`.

## Afterwards, with no VM

1. Commit the files under `internal/tee/parse/tdx/testdata/` and `internal/tee/parse/snp/testdata/`, with their
   identifying information listed in each `README.md`, as for the existing fixtures.
2. Parser tests:
   - TDX: RTMR2 differs between v1 and v2; MRTD, RTMR0 and RTMR1 are equal.
   - SEV-SNP: INITRD differs and equals the "combined" sha256 that `mkappinitrd` printed (a check from outside the
     parser); MEASUREMENT, PCR4, KERNEL and CMDLINE are equal.
3. A `cmd` chain test per platform, like `TestRun_E2E_TDX_AuditorUser`: register the v1 measurements on anvil, serve
   v2 with `ra-serve --extractor fixture=...`, and `verify` must exit 1. That is T5 as code.
4. Write the measured base size mod 4 and the `dmesg` lines into `docs/build/tdx.md`, "Before or after the base
   initrd", whatever they show.
5. Move T5 in `docs/plan/test-matrix.md` from "needs VM" to "code" for each platform that was recorded.
