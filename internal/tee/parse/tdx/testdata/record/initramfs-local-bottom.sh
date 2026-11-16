#!/bin/sh
# initramfs-tools local-bottom script for the Intel TDX Option 2 recordings (docs/build/app-fixture-recording.md).
# Install as /etc/initramfs-tools/scripts/local-bottom/attestamp-verity before building the base initrd.
#
# On TDX the application image, its dm-verity hash tree and its root hash travel inside the initrd, in the archive
# placed before the base (/opt/app/app.img, /opt/app/app.hash, /opt/app/roothash); RTMR2 already covers all three. This
# script runs after the real root is mounted at ${rootmnt}: it verifies the whole image against the root hash, opens it
# with dm-verity and mounts it read-only on ${rootmnt}/opt/app-verity. The image file stays open through the loop
# device after the initramfs is freed.
#
# The outcome goes to /run/attestamp-status (ok or fail) and any error output to /run/attestamp-log; initramfs-tools
# moves /run onto the real root, so both can be read after boot. It follows the SEV-SNP script in
# internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh, without the PCR12 extension (there is no vTPM in the
# TDX trust boundary; the image is already measured).
PREREQ=""
prereqs() { echo "$PREREQ"; }
case "$1" in prereqs) prereqs; exit 0;; esac
D=/opt/app
[ -f "$D/app.img" ] && [ -f "$D/app.hash" ] && [ -f "$D/roothash" ] || exit 0
RH=$(cat "$D/roothash")
modprobe loop 2>/dev/null
modprobe dm_verity 2>/dev/null
modprobe squashfs 2>/dev/null
STATUS=fail
if /sbin/veritysetup verify "$D/app.img" "$D/app.hash" "$RH" 2>/run/attestamp-log; then
	if /sbin/veritysetup open "$D/app.img" attestamp-app "$D/app.hash" "$RH" 2>>/run/attestamp-log; then
		if mount -t squashfs -o ro /dev/mapper/attestamp-app "${rootmnt}/opt/app-verity" 2>>/run/attestamp-log; then
			STATUS=ok
		fi
	fi
fi
echo "$STATUS $RH" > /run/attestamp-status
exit 0
