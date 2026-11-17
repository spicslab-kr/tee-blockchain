# Settings we change, and the hashes we use

Every setting this project changes on a machine, or relies on, in one place: what it does, why it has to have this
value, and what breaks if it is changed. Then every hash, where it is used, and who chose it. A claim without
evidence here says **not checked**.

The values both sides must match to get the same measurements are a separate list: [reproduce.md](reproduce.md).

## On the TDX and SEV-SNP guests

### `GRUB_FORCE_PARTUUID`, commented out

| | |
|---|---|
| what it does | Google's Ubuntu images set it in `/etc/default/grub.d/40-force-partuuid.cfg`; with it set, GRUB boots the stock entry **without any initrd**, the kernel finding the root partition by PARTUUID. Commenting it out and running `update-grub` makes the stock entry load the image's initrd and name the root by file-system UUID |
| why this value | the application is in the initrd (Option 1), or the dm-verity check runs from it (Option 2); without an initrd neither is ever loaded |
| what breaks if it is set | the boot measures no application: SEV-SNP evidence is refused by every format (exit 7); on TDX the quote alone cannot show it, `launch` refuses to install and `launch --check` refuses the boot, and `audit` requires the Auditor's own build since 2026-11-16 (`docs/results/sessions-2026-11-16/tdx-initrdless.md`). Seen on 2026-11-16: a guest whose step 13 had put it back refused `launch` |
| side effect | the command line loses `panic=-1`, which only the initrdless entry adds (measured in the recordings) |
| set by | "Getting started" step 4 in `tdx.md` and `sev-snp.md`; undone in step 13 |

### The boot entry `attestamp`

| | |
|---|---|
| what it does | `launch` copies the built initrd to `/boot/initrd-attestamp.img`, writes `/etc/grub.d/41_attestamp` (the stock entry with only its title and initrd changed) and makes it the default in `/etc/default/grub.d/99-attestamp.cfg` (`GRUB_DEFAULT=attestamp`) |
| why this value | the entry is the stock one, so the kernel, its command line and the root UUID stay those of the image and both sides get the same RTMR2 / CMDLINE; only the initrd is ours |
| what breaks if it differs | another entry, title aside, gives other GRUB commands or another command line: TDX RTMR2 and SEV-SNP CMDLINE move. If the default falls back to the stock entry (for example GRUB's failed-boot detection), the stock initrd is measured and `launch --check` refuses |
| set by | `att-* launch --reboot`; undone in step 13 |

### `attestamp.roothash=` on the kernel command line (SEV-SNP Option 2)

| | |
|---|---|
| what it does | carries the dm-verity root hash to the hook in the initrd; the hook opens the image only if it matches |
| why this value | the root hash is the one `build` printed; it is also in CMDLINE, so the evidence names the image |
| what breaks if it differs | CMDLINE and PCR12 move; another image does not open. On TDX the root hash travels inside the initrd instead (RTMR2) |

### The application archive and where it goes in the initrd

| | |
|---|---|
| what it does | `build` writes the application as a cpio archive with fixed metadata (sorted names, time 0, owner 0, fixed inode numbers; `internal/setup/build/cpio`) and joins it to the base initrd: **before** it on TDX (`cpio.AppFirst`), **after** it on SEV-SNP, with the base zero-padded to a multiple of 4 bytes (`cpio.AppLast`) |
| why this value | fixed metadata gives the same bytes on every machine (golden tests pin them). The SEV-SNP padding: the kernel finds an archive after a compressed one only at a 4-byte boundary |
| what breaks if it differs | without the padding the kernel **drops the archive and still boots**, and INITRD still changes with the application (measured in 2026-11; `sev-snp.md`, "Correction: SEV-SNP Option 1 as appended in 2026-11"); `launch --check` catches it through the boot marker. AppLast on TDX: **not checked** (`docs/status/build-info-gaps.md`, Still missing) |

### The dm-verity image (Option 2)

| | |
|---|---|
| what it does | `build` packs the application into a read-only squashfs image and makes a dm-verity hash tree over it (`veritysetup format --hash=sha256`, 4096-byte blocks); the guest opens it read-only and mounts it on `/opt/app-verity` |
| why these values | reproducibility: `mksquashfs -mkfs-time 0 -all-time 0`, file times 0, `umask 022` and mode 755 on every file and folder, and a **fixed** `--salt` and `--uuid` (`internal/setup/build/verity/verity.go`; `veritysetup` picks random ones otherwise). Read-only plus dm-verity: a changed block fails to read |
| what breaks if it differs | a random salt or UUID, another umask, or a build time gives another image and root hash on every build (the 2026-11 record; `sev-snp.md`, "Variables"). On a root-hash mismatch the image is not opened, the hook writes `fail` to `/run/attestamp-status` and **the boot continues** (measured on TDX, 2026-11-12); RTMR2 / PCR12 then differ from any registered value and `launch --check` refuses. Stopping the boot instead is part of locking the guest, not done (`docs/build/runtime-immutability.md`, "What building layers 2 and 3 takes"). The same tamper case on SEV-SNP: **not checked** |

### The SEV-SNP Option 2 hook and PCR12

| | |
|---|---|
| what it does | the hook in the Vendor's `base-initrd-v.img` extends PCR12 with SHA-256 of `attestamp-verity-<ok\|fail>:<root hash>` (`internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh`), so PCR12 = SHA-256(32 zero bytes ‖ that digest) |
| why this value | PCR12 is otherwise unused at boot; the string records both the outcome and the image, and PCR12 stays zero if the hook never ran |
| what breaks if it differs | another string or PCR gives values `build` does not predict; `launch --check` compares PCR12 with the prediction. Needs `tpm2-tools` in the base initrd (installed by hand on the Vendor's guest, `5.6-1build4`) |

### `SEPT_VE_DISABLE` (TDX)

| | |
|---|---|
| what it is | bit 28 of the TD attributes, as Intel's TDX module specification (A.3.4) numbers them. **We do not set it**: GCP launches the TD, and the bit is set in GCP's quotes |
| what we do with it | nothing, except as a test anchor: read little endian, the recorded quote's TD attributes have exactly bit 28 set and no reserved bit, which pins the byte order the parser uses for bit 0, DEBUG (`internal/tee/parse/tdx/endian_test.go`). DEBUG is the only attribute bit checked: a debug TD is refused |
| what it does inside the TDX module, and what would change if it were clear | **not checked** here |

### The GCP VM settings `gcp-cvm.sh` uses

| setting | what it does | why | if changed |
|---|---|---|---|
| `--confidential-compute-type=TDX` / `SEV_SNP` | makes the VM a TD or an SEV-SNP guest | the platform | another evidence format; GCP offers no debug variant (`ra-comparison.md`, "Debug mode") |
| `--maintenance-policy=TERMINATE` | the VM stops for host maintenance instead of migrating | these VMs cannot live-migrate (`sev-snp.md`, "The instance") | **not checked** |
| `--min-cpu-platform="AMD Milan"` (SEV-SNP) | puts the guest on Milan or newer | the CPU platform `gcp-cvm.sh`'s header names for SEV-SNP on N2D | MEASUREMENT by structure; **not checked** on another CPU |
| `--image=<exact version>` | the boot disk's image | kernel, root UUID and base initrd come with it | the values move (`reproduce.md`) |
| `--tags=attestamp-ra` | names the VM for the two firewall rules below | both rules target this tag only | no SSH (with port 22 closed to the internet) and no RA port |
| Shielded VM Secure Boot | left at GCP's default, **off** on our guests | not chosen | **not checked** |

## Network access

| setting | what it does | why this value | what breaks if it differs |
|---|---|---|---|
| GCP `default-allow-ssh` **deleted** (2026-11-16, project of the recordings) | port 22 is no longer open to `0.0.0.0/0` for any VM of the project's default network | the guests need SSH only from their operator | plain `gcloud compute ssh` times out (measured); every VM in that network loses SSH from the internet, not only these |
| GCP `attestamp-ssh-iap`: tcp:22 from `35.235.240.0/20`, tag `attestamp-ra` | SSH only through Google's IAP proxy | IAP's documented range; the operator's own address does not matter | without it (and with the rule above deleted) no SSH at all; `--tunnel-through-iap` is needed on every `ssh`/`scp` (measured, `e2e/cloud/README.md`) |
| GCP `attestamp-ra`: tcp:5000 from one `/32` | lets a verifier outside the guest reach the responder | one address, never `0.0.0.0/0` | another address cannot reach port 5000; not needed when the chain runs on the guest |
| AWS security group: tcp:22 and tcp:5000 from one `/32` | the same for the Nitro parent instances | as above | `aws-nitro.sh status` says when this machine's address is not allowed |

## Building

The Go toolchain and the four build settings (`CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid=`)
are in [reproduce.md](reproduce.md) and `CONTRIBUTING.md`, "Build". The fourth, since 2026-11-17: `-ldflags=-buildid=`
leaves Go's build ID out of the binary. That ID is a hash of every input of the build, so with it a change to any
package `att-ra-responder` links moved Nitro's PCR0 and PCR2 even when no code it can run changed (`38b49e7`); without
it, code it never runs and comments that move no line of code leave the responder as it is (measured,
`docs/status/build-info-gaps.md`, Q1). `build` refuses a responder that carries one, read from the ELF note
`.note.go.buildid`, because Go does not record `-ldflags` in a `-trimpath` binary's build info. One
setting changes the image itself: on Nitro, `build` and `pack` empty field 3 of `/etc/shadow` and `/etc/shadow-` (the
day of the last password change), because it carried the build day into PCR0 and PCR2 (`4d70d74`; `aws-nitro.md`,
"Two builds compared").

## Hashes

| where | algorithm | chosen by | why |
|---|---|---|---|
| Nitro PCR0–2, and the document's `digest` field | SHA-384 | AWS | the platform; the parser refuses a document whose `digest` is not `SHA384` |
| Nitro document signature | ECDSA P-384 with SHA-384 (ES384) | AWS | the platform |
| TDX MRTD, RTMR0–3 | SHA-384 | Intel | the platform |
| TDX: what GRUB measures of the initrd into RTMR2 | SHA-384 of the file (`initrd_sha384` in `build-value.json`) | the platform's event log | `build` records it so `launch --check` can compare the boot's event with the build |
| TDX quote signature | ECDSA P-256 (the QE's attestation key) | Intel | the platform |
| SEV-SNP MEASUREMENT | a SHA-384 chain over the launch pages | AMD | the platform |
| SEV-SNP report signature | ECDSA P-384 (the chip's VCEK) | AMD | the platform |
| SEV-SNP PCR4, PCR12, KERNEL, INITRD, CMDLINE | SHA-256 | the vTPM bank we read | the parser takes the SHA-256 quote among those the attestation carries (`snp/measure.go`); GRUB's file digests in that bank are SHA-256. Why this bank: the verity hook extends only the SHA-256 bank, and the 2026-11 Auditor scripts computed their expected values with SHA-256 (`snp/measure.go`) |
| SEV-SNP binding: REPORT_DATA = hash of the vTPM attestation | **SHA3-512** | **us** (project decision, 2026-11-11; SHA-512 before) | it must be 64 bytes, the size of REPORT_DATA. Why SHA3-512 rather than SHA-512: **not recorded**. Evidence bound with SHA-512 is refused (`TestVerifySig_SHA512BindingIsRefused`) |
| SEV-SNP PCR12 extension | SHA-256 of `attestamp-verity-<ok\|fail>:<root hash>` | us (the hook) | the PCR12 bank is SHA-256 |
| dm-verity hash tree and root hash | SHA-256 (`--hash=sha256`), fixed salt | us, made explicit | `veritysetup`'s own default is the same; the salt is fixed for reproducibility |
| the nonce | not a hash: 32 random bytes from the OS (`crypto/rand`) | us | TDX puts it in REPORTDATA followed by zeros; Nitro in the document's `nonce`; SEV-SNP in the vTPM quotes' `extraData` |
| `measurementHash`, the registered value | **keccak256** of the measurements as sorted `name=<hex>\n` lines | us | Ethereum's native hash: the contract, its EIP-712 signatures and `recordKey` all use keccak256, and the value is compared on chain. Computed in four places that must stay identical (`CONTRIBUTING.md`, "The measurement hash") |
| `recordKey` | keccak256(abi.encode(approver, keccak256(platform), measurement)) | us, in `Registry.sol` | one namespace per approver and platform |
| the registration signature | EIP-712 typed-data digest (keccak256), secp256k1 | Ethereum | what `ecrecover` checks on chain |
| `build-config.json`, `measurement-record.json` and the EIF in the hand-off | SHA-256 | us | file integrity between the Vendor and the Auditor (`internal/core/transfer/*/digest.go`) |
| file hashes quoted in the documents (binaries, initrds, fixtures) | SHA-256 | us | the common `sha256sum`; TDX initrds are also quoted by SHA-384, the value RTMR2 uses |
