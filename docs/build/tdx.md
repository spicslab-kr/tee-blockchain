# Intel TDX: what is measured, what changes it, and what is not done yet

**Status.** The verifying side works on a recorded quote: `ra-parse`, `audit` and `verify` accept `intel-tdx`
(`internal/tee/parse/tdx`). Requesting a quote from a running guest, building the image and launching it are not
implemented. The audit-and-verify chain test (`TestRun_E2E_TDX_AuditorUser`) is written but has not run yet, because
`anvil` was not installed on the machine it was written on.

Statements are marked **confirmed** (read from our recorded quote, a spec, a library's source or an official page),
**observed** (seen in our runs) or **not confirmed**.

## The quote

One request returns one signed document, like Nitro: a TDX quote (version 4 on GCP, confirmed). It is the TD report
signed by an attestation key; Intel's Quoting Enclave vouches for that key with a report signed by the platform's PCK
certificate, and the PCK certificate chains to the Intel SGX Root CA. The PCK chain is inside the quote.

`ra-parse` checks, in order:

1. The quote decodes as version 4 with TEE type `0x81` (TDX).
2. The PCK certificate chains, through the intermediate carried in the quote, to the Intel SGX Root CA embedded as
   `internal/tee/parse/tdx/root.pem`. The root at the end of the quote's own chain is never trusted.
3. go-tdx-guest checks the quote signature, the Quoting Enclave report signature and the hash that binds the
   attestation key to that report.
4. REPORTDATA carries the nonce sent.
5. TDATTRIBUTES.DEBUG (bit 0) is clear. A debug TD is refused.

Intel's collateral (TCB info, QE identity) and revocation lists come from Intel's PCS over the network and are **not**
fetched, so the check runs offline like the Nitro one. The cost: the platform's TCB level is not judged, and a quote
from a machine with an out-of-date TDX module still verifies.

The root certificate is the same one go-tdx-guest embeds and the one inside our real quote (sha256 fingerprint
`44:A0:19:6B:…:74:D3`, valid 2018-05-21 → 2049-12-31, confirmed). It has not been compared with a copy downloaded from
Intel.

## The values

| name | what it covers |
|---|---|
| MRTD | the TD's initial contents: the virtual firmware (TDVF) GCP launches |
| RTMR0 | the firmware's configuration |
| RTMR1 | the boot loader and kernel |
| RTMR2 | the kernel command line and the initrd, where the application archive is |

The firmware extends what a TPM would put in PCR1 and 7 into RTMR0, PCR2–6 into RTMR1 and PCR8–15 into RTMR2. That
mapping is the TDVF design; **it has not been confirmed on GCP** by changing each part in turn.

Why all four, not RTMR2 alone: the firmware, boot loader and kernel extend RTMR2 themselves, so a changed kernel could
extend the expected RTMR2 while running something else, and only MRTD and RTMR0–1 would show it. Nitro registers PCR0,
1 and 2 together for the same reason.

Left out, because they describe the machine or whoever launched it, not the image:

| field | why |
|---|---|
| MROWNER, MROWNERCONFIG, MRCONFIGID | set by whoever launches the TD; MROWNER is non-zero in our GCP quote (confirmed). That it differs per instance is the project's working assumption, not measured |
| RTMR3 | extended at run time by the guest; zero in our quote (confirmed) |
| MRSEAM, MRSIGNERSEAM | the TDX module, i.e. the platform |

## Nonce and time

REPORTDATA is 64 bytes and a `tee/request` nonce is 32. `CheckNonce` requires REPORTDATA to be the nonce followed by
zeros; a 64-byte nonce fills all of it. The TDX request side, when written, must put the nonce first and zeros after.

A TDX quote has **no timestamp** (confirmed: the quote format has no time field). The PCK chain is therefore judged at
verification time. That costs little: the PCK leaf in our quote is valid 2026-10-15 → 2033-10-15 (confirmed), not the
three hours of a Nitro leaf. Freshness comes from the nonce.

## Variables

Nothing below has been measured by changing it on TDX; the rows are what the structure above implies, or what the
SEV-SNP runs measured for the same files.

| variable | which value | note |
|---|---|---|
| GCP's TDVF version | MRTD | GCP may update it; every registration would change |
| VM shape (vCPUs, memory) | RTMR0 probably | the firmware's configuration includes the ACPI tables. **Not confirmed** |
| shim, GRUB, kernel | RTMR1 | OS image updates |
| kernel command line | RTMR2 | it carries the root PARTUUID, which differs per disk |
| base initrd | RTMR2 | `mkinitramfs` writes a random seed, so the Vendor's base initrd is used as published (as on SEV-SNP) |
| application archive | RTMR2 | Go version, `-trimpath`, the cpio writer: the same as on SEV-SNP (`docs/build/sev-snp.md`). The archive is written by `internal/setup/build/cpio`, byte-identical to the scripts' `mkcpio.py` |
| archive placement | RTMR2 | the application archive goes **before** the base initrd (`cpio.AppFirst`). Appended after the compressed base initrd, the guest failed to decompress it (docs/plan/cvm-port.md, Phase 3.2). On SEV-SNP the archive is appended and the guests booted; why the two differ is not established |
| `GRUB_FORCE_PARTUUID` | RTMR2 | GCP's Ubuntu 24.04 image boots its stock entry without an initrd when this is set (confirmed on the SEV-SNP image of the same family). An application in an initrd is then never loaded. Boot a custom GRUB entry instead (cvm-port.md, Phase 3.3) |

## The instance

`e2e/cloud/gcp-cvm.sh tdx create <roles…>` creates it.

| setting | value | why |
|---|---|---|
| machine type | `c3-standard-4` | TDX on GCP runs on C3 (Sapphire Rapids) or C4 (script header, checked against Google's docs 2026-11) |
| confidential type | `--confidential-compute-type=TDX` | |
| CPU platform | none | the script header says TDX must be created **without** `--min-cpu-platform` |
| maintenance | `--maintenance-policy=TERMINATE` | these VMs cannot live-migrate |
| image | `ubuntu-os-cloud/ubuntu-2404-lts-amd64` | must carry `TDX_CAPABLE`; the script checks it |
| zone | `us-central1-a` (default) | |

Our recorded guest: Ubuntu 24.04.5, kernel `7.0.0-1017-gcp`, ConfigFS TSM provider `tdx_guest` (confirmed).

## From scratch: record and check one quote

1. `bash e2e/cloud/gcp-cvm.sh tdx create vendor`, then on the instance `sudo shutdown -h +240` as a safety stop.
2. Record a quote with ConfigFS TSM, as in `internal/tee/parse/tdx/testdata/README.md`.
3. Copy `quote.bin` back (`gcloud compute scp`, one file per call) and stop the instance.
4. `att-user ra-parse --doc quote.bin --format intel-tdx --nonce <128 hex chars>` prints MRTD and RTMR0–2.

## Not done yet

| what | why |
|---|---|
| `tee/request/<role>/tdx`, a `--extractor` in `cmd/ra-responder` | Phase 2; needs a running guest |
| `setup/build` and `setup/launch` for TDX | Phase 3: build the archive, put it first, boot a custom GRUB entry once, read the values back |
| the audit-and-verify chain test against anvil | written (`cmd/tdx_e2e_test.go`), skipped where anvil is not installed |
| TCB level, CRLs | collateral is not fetched (see above) |
| quote version 5 | the go-tdx-guest revision in `go.mod` decodes version 4 only; version 5 needs a newer one and its own fixture |
| the PCR→RTMR mapping and the VM-shape dependence | not confirmed on GCP |
