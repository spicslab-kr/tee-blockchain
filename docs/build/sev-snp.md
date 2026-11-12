# Reproducible build on AMD SEV-SNP: what changed the hashes in our tests

Attestamp does not support SEV-SNP yet. This page records what we found while testing it on AWS and
GCP SEV-SNP instances, so that the same mistakes are not made when it is implemented.

On these clouds the SEV-SNP measurement covers only the firmware. The application's hash comes from
the vTPM, which records the boot files: either an initrd with the application appended, or a
dm-verity disk image whose root hash is in the boot settings. So the Vendor and the Auditor must
produce the same initrd or the same disk image, byte for byte.

## Variables we found

Each row changed a hash in our tests, with the same source code.

| variable | what changed | how we kept it fixed |
|---|---|---|
| Go version | Go 1.22.2 and 1.23.4 gave different application binaries | the same Go version on both sides |
| build path | without `-trimpath`, two folders gave two different binaries | `-trimpath` |
| cpio tool | GNU cpio 2.13 (Amazon Linux 2023) and 2.15 (Ubuntu) packed the same files into different bytes (directory link counts) | our own cpio writer: sorted names, time 0, owner 0, fixed inode numbers |
| distribution base initrd | `mkinitramfs` and `dracut` build a different file every time (it contains a random seed) | none possible. The Auditor uses the base initrd the Vendor publishes and rebuilds only the application part |
| folder permissions (umask) | the same files in a squashfs image gave a different image and root hash under umask 022 (folders 755) and umask 002 (folders 775) | build under the same umask. Our script fixes file modes but not folder modes, so this is still open |
| squashfs timestamps | `mksquashfs` writes the build time by default | `-mkfs-time 0 -all-time 0`, file times set to 0 |
| dm-verity salt and UUID | `veritysetup format` picks them at random | fixed `--salt` and `--uuid` |

With all of the above fixed, the application, its cpio archive and the dm-verity image hashed the
same on GCP (Ubuntu 24.04, 26.04) and AWS (Amazon Linux 2023), across squashfs-tools 4.4, 4.6.1
and 4.7.5.

## Checking

This list is what we found, not everything that can differ. There is no `--verify` for SEV-SNP yet.
Until there is, build twice and on two machines, and compare the hashes of the application, the
cpio archive and the disk image before registering anything.

## Proving the code has not changed since boot

A measurement taken at boot says nothing about code changed afterwards. Our test closes that gap
with a VM that has no way in after boot: the application is in the measured initrd, the kernel
starts it directly, and there is no disk, shell, SSH or cloud agent. See
[e2e/sev-snp](../../e2e/sev-snp/README.md) for the code, how to run it and what we saw on GCP and AWS.
