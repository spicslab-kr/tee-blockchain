#!/bin/sh
# initramfs-tools local-bottom script for the amd-sev-snp-verity app fixtures (docs/plan/app-fixture-recording.md).
# Install as /etc/initramfs-tools/scripts/local-bottom/attestamp-verity.
#
# It runs after the real root is mounted at ${rootmnt}. It reads the expected root hash from the kernel command line
# (attestamp.roothash=, the argument internal/tee/parse/snp requires), verifies the whole application image against it
# with dm-verity, opens it and mounts it read-only on ${rootmnt}/opt/app-verity, and extends vTPM PCR12 (SHA-256 bank)
# with SHA-256 of "attestamp-verity-<ok|fail>:<root hash>". So PCR12 records both the outcome and which image was
# checked, and stays zero if this script never ran. The status and any error output go to /run, which initramfs-tools
# moves onto the real root, so they can be read after boot.
#
# The expected PCR12 (checked against the recordings in internal/tee/parse/snp/app_fixture_test.go):
#   PCR12 = SHA-256( 32 zero bytes || SHA-256("attestamp-verity-ok:" || root hash) )
# Record: the logic and the PCR12 value follow a 2026-11 script that is no longer available.
PREREQ=""
prereqs() { echo "$PREREQ"; }
case "$1" in prereqs) prereqs; exit 0;; esac
RH=""
for p in $(cat /proc/cmdline); do
	case $p in attestamp.roothash=*) RH=${p#attestamp.roothash=};; esac
done
[ -z "$RH" ] && exit 0
modprobe loop 2>/dev/null
modprobe dm_verity 2>/dev/null
modprobe squashfs 2>/dev/null
modprobe tpm_crb 2>/dev/null
modprobe tpm_tis 2>/dev/null
sleep 1
export TPM2TOOLS_TCTI=device:/dev/tpm0
D=${rootmnt}/var/lib/attestamp
STATUS=fail
if /sbin/veritysetup verify "$D/app.img" "$D/app.hash" "$RH" 2>/run/attestamp-log; then
	if /sbin/veritysetup open "$D/app.img" attestamp-app "$D/app.hash" "$RH" 2>>/run/attestamp-log; then
		mkdir -p "${rootmnt}/opt/app-verity"
		if mount -t squashfs -o ro /dev/mapper/attestamp-app "${rootmnt}/opt/app-verity" 2>>/run/attestamp-log; then
			STATUS=ok
		fi
	fi
fi
echo "$STATUS" > /run/attestamp-status
DIG=$(printf 'attestamp-verity-%s:%s' "$STATUS" "$RH" | sha256sum | cut -d' ' -f1)
/bin/tpm2_pcrextend "12:sha256=$DIG" 2>>/run/attestamp-log
exit 0
