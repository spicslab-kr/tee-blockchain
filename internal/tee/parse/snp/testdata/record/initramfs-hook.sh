#!/bin/sh
# initramfs-tools hook for the amd-sev-snp-verity app fixtures (docs/plan/app-fixture-recording.md). Install as
# /etc/initramfs-tools/hooks/attestamp-verity. It puts into the initrd what initramfs-local-bottom.sh needs:
# veritysetup, tpm2_pcrextend, sha256sum, the TPM TCTI library tpm2-tools loads at run time, and the kernel modules.
# Follows the 2026-11 script option2-dm-verity/verity_hook.sh (outside this repository).
PREREQ=""
prereqs() { echo "$PREREQ"; }
case "$1" in prereqs) prereqs; exit 0;; esac
. /usr/share/initramfs-tools/hook-functions
manual_add_modules dm_verity loop squashfs tpm tpm_crb tpm_tis tpm_tis_core
copy_exec /usr/sbin/veritysetup /sbin
copy_exec /usr/bin/tpm2_pcrextend /bin
copy_exec /usr/bin/sha256sum /bin
for l in /usr/lib/x86_64-linux-gnu/libtss2-tcti-device.so*; do
	[ -f "$l" ] && copy_exec "$l"
done
exit 0
