# Runtime immutability: what a measurement can and cannot say about a running confidential VM

A measurement says what was loaded at boot. Whether the guest is still running only that, later, is a separate
question, raised in the project's review, and this page is the answer so far. It is split into
three layers. Each says what is **confirmed** (measured by us, or stated in a cited source) and what is only
**reasoned** from confirmed pieces and not built.

## Layer 1: the application is immutable at run time. CONFIRMED.

Option 2 mounts the application as a read-only squashfs image on a dm-verity target.

- dm-verity checks blocks when they are read, not only at boot. The kernel's documentation
  (`Documentation/admin-guide/device-mapper/verity.rst`, "Theory of operation") says: "After instantiation, all hashes
  will be verified on-demand during disk access", and "If they cannot be verified up to the root node of the tree, the
  root hash, then the I/O will fail."
- So a change to the backing image is caught when a changed block is read: the read fails.
- Two qualifications from the same document:
  - the check happens "on first read into the page cache", so a block already in memory is not read from the disk
    again;
  - the option `check_at_most_once` would verify each data block only once and, in the document's words, "only offline
    tampering of the data device's content will be detected, not online tampering".
- Our verifying script (`internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh`) opens the image with
  `veritysetup open` and no such option.
- **Measured 2026-11-11** on SEV-SNP: in both Option 2 recordings the image verified at boot (`status ok`), the
  application was mounted read-only on `/opt/app-verity`, and PCR12 matched the expected digest for the root hash on
  the kernel command line (`internal/tee/parse/snp/testdata/gcp-n2d-opt2-v*-boot.txt`).
- **Not measured:** a change made to the image while the guest runs. That is the tamper case, step 5 of "What building
  layers 2 and 3 takes" below.

## Layer 2: the rest of the guest is immutable. NOT BUILT.

Locking the application does not stop someone who can log in from running a different binary next to it. It needs:

- the root file system itself as a dm-verity, read-only image;
- no `sshd`, and no other login;
- every writable path on `tmpfs`;
- no outbound network.

None of this exists yet. Today we log in to both guests over SSH to record evidence, which is exactly what layer 2
removes. That is why the responder serving evidence over TCP had to come first: it is the only way to get evidence out
of a guest nobody can log in to, and it exists (`att-ra-responder ra-serve --extractor tdx|snp-initrd|snp-verity`).

## Layer 3: proving layer 2 from the measurement. REASONED, NOT TESTED.

The verity root hash of the root file system goes on the kernel command line. The command line is measured (CMDLINE
on SEV-SNP, part of RTMR2 on Intel TDX), so a verifier sees that hash. The hash covers the whole root file system, so
an image with no `sshd` has no `sshd`, and read-only plus dm-verity means none can be added while it runs.

This chain has not been built or booted. It is reasoning from facts we have confirmed separately, not a measured
result:

- the command line is in the measurement: measured on SEV-SNP (CMDLINE changed with the root hash in the Option 2
  recordings); on TDX, RTMR2 changed with the initrd, and the TDVF design puts the command line there too (not changed
  separately in our recordings);
- dm-verity refuses blocks that do not match the root hash: kernel documentation, layer 1;
- an image without `sshd` contains no `sshd`: true by construction of the image, to be checked by inspecting the image
  the root hash names.

**What this would still not cover:**

- an attack on the memory of a process that is already running;
- code fetched over the network. This is why outbound traffic has to be blocked, and that firewall configuration has
  to live inside the verity-covered root to be provable.

## What building layers 2 and 3 takes. NOT STARTED.

The answer to "how far does the guarantee go, and what would take it further": a locked guest, built and then tested
the way layer 1 was. In order:

| # | step | passes when |
|---|---|---|
| 1 | Build a locked root image: the root file system as a squashfs + dm-verity image with no `sshd` and no other login, writable paths on `tmpfs`, and outbound traffic blocked by a firewall configuration that lives inside the image; `att-ra-responder` started at boot with `--extractor tdx` or `snp-verity` and `--listen tcp:0.0.0.0:5000` | two builds give the same root hash (the Option 2 settings: squashfs times 0, fixed salt and UUID, `umask 022`) |
| 2 | Boot it, with the root image's verity root hash on the kernel command line (`root=/dev/mapper/...`, set up by the initrd against that hash) | the guest boots; SSH is refused; the responder answers from another machine |
| 3 | Record evidence from the locked guest through the responder, not over SSH | a fixture recorded with `att-user ra-request` from the verifier's machine |
| 4 | A test in which the verifier reads the root hash out of the measured command line (CMDLINE on SEV-SNP, the command-line event behind RTMR2 on TDX) and compares it with the hash of the image it was given | the hash in the evidence equals the published image's; a different image gives a different measurement |
| 5 | **The tamper case:** break the verity image on purpose (one data block of the application image, and separately of the root image) and boot or read it | the change shows: PCR12 records `fail` (SEV-SNP Option 2), or the read fails with an I/O error, or the boot stops |

Steps 4 and 5 turn layer 3 from reasoning into a measured result; until they pass, this page keeps calling it reasoned.

Where step 5 starts from: the TDX boot of 2026-11-12 with v2's image under v1's root hash recorded `fail`, mounted
nothing, and **went on to a normal login** (`docs/build/app-fixture-recording.md`, "TDX Option 2 mounted at boot").
That is enough for a verifier, since the measurement differs from every registered value, but a locked guest must stop
the boot instead. Still to do: the same on SEV-SNP, a changed data block under an unchanged root hash, and the root
image. The 2026-11 script that flipped one byte at offset 5000 for this is no longer available.

When the guest is locked, the responder moves into the locked root image; today it runs from the guest's disk and is
not measured (`docs/build/ra-comparison.md`).

## Nitro, for comparison

Nitro gets layer 2 structurally: an enclave has no persistent storage, no external network and no interactive access
(AWS's description of Nitro Enclaves); the only way in or out is the vsock to the parent instance. Debug mode is
visible in the evidence because PCR0 reads all zero (`internal/tee/parse/aws/measure.go` treats it that way; no debug
enclave has been recorded, test-matrix row T8).

A confidential VM is a full virtual machine and has none of that by default: a disk, a network, and SSH are all there
unless the image takes them away.
