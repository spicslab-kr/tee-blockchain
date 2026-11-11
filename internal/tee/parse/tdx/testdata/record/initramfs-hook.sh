#!/bin/sh
# initramfs-tools hook for the Intel TDX Option 2 recordings (docs/plan/app-fixture-recording.md). Install as
# /etc/initramfs-tools/hooks/attestamp-verity before building the base initrd with mkinitramfs. It puts into the initrd
# what initramfs-local-bottom.sh needs: veritysetup and the dm-verity, loop and squashfs modules. Unlike the SEV-SNP
# hook there is no tpm2_pcrextend: a TDX guest on GCP measures the initrd into RTMR2 and nothing is extended at run time.
PREREQ=""
prereqs() { echo "$PREREQ"; }
case "$1" in prereqs) prereqs; exit 0;; esac
. /usr/share/initramfs-tools/hook-functions
manual_add_modules dm_verity loop squashfs
copy_exec /usr/sbin/veritysetup /sbin
exit 0
