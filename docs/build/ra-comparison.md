# What each platform gives the verifier

A side-by-side view of the evidence Attestamp checks on its three platforms, at the moment a User or an Auditor
verifies it. Everything here is read from the parsers in `internal/tee/parse/{aws,tdx,snp}` and from the evidence
recorded on real instances in their `testdata/` folders. A row that is not settled says so.

| | AWS Nitro Enclaves | Intel TDX | AMD SEV-SNP |
|---|---|---|---|
| Cloud we use | AWS | GCP (`c3-standard-4`) | GCP (`n2d-standard-2`) |
| Format string(s) | `aws-nitro` | `intel-tdx` | `amd-sev-snp-initrd`, `amd-sev-snp-verity` |
| Recorded evidence | `aws/testdata/fixture-my-web.cbor` | `tdx/testdata/gcp-c3-quote.bin` | `snp/testdata/gcp-n2d-*` |

## Signers and documents

| | Nitro | TDX | SEV-SNP |
|---|---|---|---|
| Documents per request | **1**: a COSE_Sign1 document (CBOR) | **1**: a quote (version 4) | **2**: an SNP report and a vTPM attestation, carried in one CBOR container with the host's certificate table |
| Who signs | AWS: the leaf certificate carried in the document, ES384 (ECDSA P-384 + SHA-384) | the Quoting Enclave's attestation key (ECDSA P-256, key type 2); the QE's own report is signed by the machine's PCK | report: the chip's VCEK (AMD). vTPM quotes: Google's attestation key (AK) |
| Chain checked to | AWS Nitro Enclaves root (`aws/root.pem`) | Intel SGX Root CA (`tdx/root.pem`) | AMD ARK (embedded in go-sev-guest) **and** Google "EK/AK CA Root" (`snp/root.pem`) |
| How the parts are tied | n/a | the QE report's REPORT_DATA covers the attestation key (checked by go-tdx-guest) | REPORT_DATA = SHA3-512 of the vTPM attestation bytes (`snp.bindingSum`) |
| Code | `aws/verify.go` | `tdx/verify.go` | `snp/verify.go` |

## Nonce and time

| | Nitro | TDX | SEV-SNP |
|---|---|---|---|
| Where the nonce is | the document's `nonce` field, inside the signed payload | REPORTDATA (64 bytes) in the TD report: the nonce, then zeros up to 64 bytes | the vTPM quotes' `extraData`. The SNP report does not carry it; it covers it through the binding |
| Check | `aws/nonce.go` | `tdx/nonce.go` | `snp/nonce.go` (all quotes must carry the same value) |
| Timestamp in the evidence | **yes**: `timestamp`, milliseconds since the epoch (the fixture's is 2026-09-21/22, `aws/measure_test.go`) | **no** | **no**: neither the report nor the vTPM attestation has one; the TPM `clock` is a counter (see `docs/build/sev-snp.md`, "Time and freshness") |
| Certificates judged at | the document's timestamp (a Nitro leaf lives about three hours) | verification time (`Parser.Now`, default now) | verification time (`Parser.Now`, default now) |
| Freshness comes from | the nonce | the nonce | the nonce |

## What is registered

`measurementHash` covers the sorted list of `name=value` lines, so the names and their number are part of the
on-chain value.

| | Nitro | TDX | SEV-SNP |
|---|---|---|---|
| Names | PCR0, PCR1, PCR2 | MRTD, RTMR0, RTMR1, RTMR2 | initrd: MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD. verity: the same and PCR12 |
| Count | 3 | 4 | 5 or 6 (two format strings, so each list is fixed) |
| Option 2 (squashfs + dm-verity application image) | n/a | the image goes inside the initrd, covered by RTMR2: the same format and the same four names (`docs/build/tdx.md`, "Option 1 and Option 2"; not built yet) | the image sits on disk; a boot-time hook extends PCR12, so `amd-sev-snp-verity` has a sixth value |
| Size of each value | 48 bytes (SHA-384) | 48 bytes (SHA-384) | MEASUREMENT 48 bytes; the others 32 bytes (vTPM SHA-256 bank, GRUB's SHA-256 file digests) |
| Signed by | AWS | Intel | MEASUREMENT by AMD; the rest by Google's vTPM |
| Code | `aws/measure.go`, `imagePCRs` | `tdx/measure.go`, `imageRTMRs` | `snp/measure.go`, `initrdNames`, `verityNames` |
| Asserted on recorded evidence | yes (`aws/measure_test.go`, also against the `nitrite` library) | yes (`tdx/measure_test.go`) | yes, the five initrd values (`snp/measure_test.go`), each checked against a value read outside the parser. PCR12 has no recording yet |

## Debug mode

A debug TEE lets the host read and change the guest's memory, so its measurements prove nothing.

| | Nitro | TDX | SEV-SNP |
|---|---|---|---|
| Signal | PCR0 all zero | TDATTRIBUTES bit 0 (TUD.DEBUG) | POLICY bit 19 (DEBUG) |
| What the parser does | `Parse` drops every all-zero PCR and fails if all three are zero; `Info.Debug` reports PCR0 all zero | `Parse` refuses (`ErrBadDocument`); `Info.Debug` reports it | `Parse` refuses (`ErrBadDocument`); `Info.Debug` reports it; a POLICY that does not parse counts as debug |
| Tested by | **not tested**: `aws/measure_test.go` only checks that the real document reports `Debug` false. A debug document is signed, so it cannot be made up, and none has been recorded (test-matrix row T8) | `TestParse_DebugTDIsRefused` (bit set in a copy of the fixture) | `TestParse_DebugGuestIsRefused` (bit set in a copy of the fixture) |

## Values left out, and why

The rule on every platform: register what describes the image, leave out what describes the machine, the launcher or
the run. Otherwise the same image would register differently on every instance.

| Platform | Left out | Why | Where |
|---|---|---|---|
| Nitro | PCR3 | the IAM role of the parent instance | `aws/measure.go` comment |
| | PCR4 | the parent instance's ID; non-zero on a real instance | same; `aws/measure_test.go` asserts it is dropped |
| | PCR8 | the enclave image's signing certificate | `aws/measure.go` comment |
| TDX | MROWNER | set by whoever launches the TD; differs per GCP instance (non-zero in our fixture) | `tdx/measure.go`; `tdx/measure_test.go` asserts it is excluded and that the fixture's is non-zero |
| | RTMR3 | extended by the guest at run time, not by the boot chain | same |
| | MROWNERCONFIG, MRCONFIGID | set by the launcher | same |
| | MRSEAM | the TDX module, i.e. the platform | `tdx/measure.go` comment |
| SEV-SNP | CHIP_ID | identifies the physical AMD processor of the host | `snp/measure.go` (never read); present in `gcp-n2d-report.bin` (`snp/testdata/README.md`) |
| | TCB versions | the host's firmware patch level, not the guest | same |
| | HOST_DATA | set by the host at launch | same |
| | every PCR except PCR4 (and PCR12 for verity) | the kernel, command line and initrd are taken from the event log instead (KERNEL, CMDLINE, INITRD); the others describe firmware configuration or nothing we use | `snp/measure.go` comment |

## Do both halves come from the same VM?

| | Nitro | TDX | SEV-SNP |
|---|---|---|---|
| Same-VM guarantee | **yes, by construction**: one document, one signer, produced inside the enclave's own platform | **yes, by construction**: one quote over one TD report | **no**. A vTPM attestation from approved VM X, hashed into an SNP report requested on VM Y, verifies. Reproduced on GCP (and on AWS before AWS was dropped for SEV-SNP) in the 2026-11 runs, test-matrix row T10 |
| What would fix it | n/a | n/a | boot files in the SNP launch measurement itself, or a vTPM inside the guest's trust boundary; neither is available to us on GCP today (`docs/build/sev-snp.md`, "Same VM") |

"By construction" is a statement about the document format: there is only one signed object, so there is nothing to
mix. It has not been tested by a relay experiment the way SEV-SNP was.
