# What each platform gives the verifier

A side-by-side view of the evidence Attestamp checks on its three platforms, at the moment a User or an Auditor
verifies it. Everything here is read from the parsers in `internal/tee/parse/{aws,tdx,snp}` and from the evidence
recorded on real Nitro instances and CVM guests in their `testdata/` folders. A row that is not settled says so.

## Which one to use

Four questions decide it. Each answer is backed by a section further down, and only measured results or the structure
of the evidence are used.

| question | Nitro | TDX | SEV-SNP |
|---|---|---|---|
| Must the evidence also cover the program that hands it out (`att-ra-responder`)? | **yes, it does**: the responder is inside the EIF, in PCR0 and PCR2 | no: the responder runs from the guest's disk; nothing proves which program answered (its answer is still hardware-signed) | no, as TDX |
| Must the reference values survive changes to this repository's code? | **no**: a change to code the responder can run, or to the lines that code sits on, moves PCR0 and PCR2, so every value needs its commit. Code it never runs moved them too (`1967749d`, measured) until the empty build ID of 2026-11-17 (below) | **yes**: the values follow the application, the image and the VM; they move with the machine type (RTMR0, measured), not with the zone | **yes**, as TDX; MEASUREMENT moves with the vCPU count (measured), not with memory or zone |
| Must the evidence come from one VM, provably? | **yes**, one signed document | **yes**, one quote | **no**: a report from another VM, requested over this VM's vTPM attestation, verifies and finds this VM's registration (T10, measured) |
| Who has to be trusted for the registered values? | one signer: AWS | one signer: Intel | two: AMD for MEASUREMENT (the firmware only), and Google's vTPM, run by the hypervisor, for PCR4, KERNEL, CMDLINE, INITRD (and PCR12) |

In one line each:

- **Nitro** when the evidence must cover everything that runs, the program that answers included. You give up stable
  reference values: they move with this repository's code and with the CPU architecture, and need an AWS parent
  instance and a Docker image.
- **TDX** when you want a whole VM, one signer and one document, and reference values that move only with what you
  build and the VM you run it on. You give up any proof of which program produced the quote, and the guest's operating
  system outside the initrd is not measured; the Auditor's own rebuild is what catches a boot without the application
  (the quote alone cannot show it).
- **SEV-SNP** when you want a whole VM on AMD, and the verifier's own parser should refuse a boot without an initrd
  (the evidence carries the event log). You give up the same-VM guarantee (T10) and a single root of trust: four of the
  five values rest on Google's hypervisor-run vTPM, not on AMD.

On every platform the same registration rule holds (`audit` requires the Auditor's own build), and freshness comes
from the nonce; only Nitro's document also carries a timestamp.

## At a glance

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
| Option 2 (squashfs + dm-verity application image) | n/a | the image goes inside the initrd, covered by RTMR2: the same format and the same four names (`docs/build/tdx.md`, "Option 1 and Option 2"; built and booted on guests since 2026-11-15) | the image sits on disk; a boot-time hook extends PCR12, so `amd-sev-snp-verity` has a sixth value |
| Is `att-ra-responder` measured? | **yes**: `build` layers it into the EIF, so its bytes are in PCR0 and PCR2, and its Go version changes them (`toolchain-pin-check.sh`) | **no**: it runs from the guest's disk after the boot; the measurement covers the initrd `att` writes. Measured 2026-11-16: the Go version moves RTMR2 only when the Go program is the application inside the initrd | **no**, as on TDX: the vTPM measures the initrd `att` writes (and for verity the image's root hash), not the responder |
| What the toolchain pin protects | the responder's bytes, so the PCRs | that the Vendor and the Auditor run the same `att`, which writes the measured initrd (and Option 2 image) | the same as TDX |
| What the evidence carries about the boot, and so what the parser can check | the responder is inside the EIF, so PCR2 covers it: the image is all there is to boot | **the quote only**: the CC event log stays on the guest, so the parser sees RTMR2 as one hash and cannot tell that no initrd was loaded (`launch --check`, on the guest, can). It is the Auditor's own build that catches it | **a container**: the vTPM attestation carries the event log, so the parser itself refuses an initrdless boot (exit 7) |
| Size of each value | 48 bytes (SHA-384) | 48 bytes (SHA-384) | MEASUREMENT 48 bytes; the others 32 bytes (vTPM SHA-256 bank, GRUB's SHA-256 file digests) |
| Signed by | AWS | Intel | MEASUREMENT by AMD; the rest by Google's vTPM |
| Code | `aws/measure.go`, `imagePCRs` | `tdx/measure.go`, `imageRTMRs` | `snp/measure.go`, `initrdNames`, `verityNames` |
| Asserted on recorded evidence | yes (`aws/measure_test.go`, also against the `nitrite` library) | yes (`tdx/measure_test.go`) | yes, the five initrd values (`snp/measure_test.go`), each checked against a value read outside the parser. PCR12 too, on the Option 2 recordings (`snp/app_fixture_test.go`, `TestAppFixtures_Option2ChangesCMDLINEAndPCR12`) |

**This is why the checks differ by platform.** What a verifier can refuse depends on what the evidence carries, not on
what the code tries: the same initrdless boot is refused in SEV-SNP's parser, only by the Auditor's comparison (and
`launch --check` on the guest) on TDX, and has no counterpart on Nitro. Carrying the CCEL in TDX evidence would move
TDX's check into the parser; it changes the wire format and is planned for after publication
(`docs/status/build-info-gaps.md`, Q2).

**The registration rule is the same on every platform, the platforms are not.** Since 2026-11-16 `audit` refuses to
register without the Auditor's own build values on Nitro, TDX and SEV-SNP alike, because the trust model (the Auditor
rebuilds) does not depend on the platform. What the refusal protects against does: on Nitro the enclave runs one EIF,
all of it covered by PCR0, so a TEE's own report always names exactly what runs and there is no concrete gap like the
next one; on TDX and SEV-SNP the guest's operating system on its disk, outside the initrd, is not measured, so a boot
that measured no application at all can still produce a genuine report (`docs/results/sessions-2026-11-16/tdx-initrdless.md`).

### What covering the responder costs: a code change no one runs still moves Nitro's values

A measurement covers the whole binary, so it moves when **any package linked into it** changes, even code that never
runs. That is not intuitive, and it happened: commit `1967749d` added one function, `TEENoun`, to `internal/cli`.
`att-ra-responder` imports `internal/cli` but never calls that function, and the responder's bytes still changed,
`5573b538…` (built at `1967749d~1`) → `c61c988e…` (built at `1967749d`), both go1.26.1 `CGO_ENABLED=0 -trimpath
-buildvcs=false` linux/amd64 (2026-11-16). Every PCR0 and PCR2 recorded before it, the clean trial's at `6a92a1f`
included, became stale at once.

*(Found and changed 2026-11-17.)* The cause was not the function but Go's **build ID**, a hash of every input of the
build that Go writes into the binary: built with `-ldflags=-buildid=` (an empty build ID) the responder is the same file
with and without that function (`215423b8…`). Since then every build passes `-ldflags=-buildid=` and `build` refuses a
responder that carries a build ID, so code the responder never runs, and comments that move no line of code, no longer
move Nitro's values. A change to code it can run, or to the lines that code sits on, still does
(`docs/status/build-info-gaps.md`, Q1).

**This is Nitro only.** On Nitro the responder is layered into the EIF, so PCR2 (and PCR0) cover it. On TDX and SEV-SNP
the responder runs from the guest's disk and is not measured, so the same change leaves RTMR2 and INITRD where they
were.

The asymmetry is a trade-off, and this is one case of each side:

| | Nitro | TDX and SEV-SNP |
|---|---|---|
| what the measurement covers | wide: the whole image, the program that produces the evidence included | narrow: the initrd (the application), not the program that produces the evidence |
| cost | reference values go stale easily: any change to code the responder can run moves them (until 2026-11-17, any change to a package it links) | the responder is not covered: which program answered the request is not in the evidence. Its answer is signed by the hardware, so it cannot forge values, but nothing proves which responder ran |
| consequence for records | every value needs the commit it was measured at (`docs/results/nitro-baseline-2026-11-16.md`) | values follow the application and the image, not the repository's code |

Whether the responder should link `internal/cli` at all was an open design question; since the empty build ID it no
longer moves the values, and dropping it to shrink the code inside the measured EIF is left for after publication
(`docs/status/build-info-gaps.md`, Q1).

## Roots of trust

Who vouches for each registered value. A value is only as trustworthy as the key that signed it.

| format | registered values | root of trust |
|---|---|---|
| `aws-nitro` | PCR0, PCR1, PCR2 | AWS Nitro attestation PKI, one signature |
| `intel-tdx` | MRTD + RTMR0–2 | Intel (PCK chain to the Intel SGX Root CA), one signature |
| `amd-sev-snp-*` | MEASUREMENT | AMD (VCEK) |
| | PCR4, KERNEL, CMDLINE, INITRD, and PCR12 for verity | Google's vTPM attestation key: a software root of trust, run by the hypervisor |

**Intel TDX** has runtime-extendable measurement registers, RTMR0–RTMR3, inside the TD ("runtime extendable
measurement register", [IETF draft-kdyxy-rats-tdx-eat-profile-02](https://www.ietf.org/archive/id/draft-kdyxy-rats-tdx-eat-profile-02.html),
§3.3). The guest extends them after launch, and the TDX module covers them with the same Intel-rooted quote signature
as MRTD. Google's documentation: "You can verify the Intel TDX module quote on its own to cover the entire boot chain"
([attestation overview](https://docs.cloud.google.com/confidential-computing/confidential-vm/docs/attestation-overview)).

**AMD SEV-SNP** has no equivalent.

- MEASUREMENT is a SHA-384 chain built during `SNP_LAUNCH_UPDATE` (each update is `DIGEST_NEW := SHA-384(PAGE_INFO)`)
  and finalized at `SNP_LAUNCH_FINISH`. It does not change afterwards: the report calls it "the measurement calculated
  at launch" (AMD 56860 rev. 1.58).
- Kernel, initrd and userspace measurements therefore come from the cloud's vTPM, which on Google Cloud is run by the
  hypervisor. Google's attestation overview (link above) puts it this way:
  - SEV-SNP's root of trust for measurement is "AMD Secure Processor + VM instance firmware".
  - "Bootloader, kernel, and userspace measurements: Shielded VM vTPM".
  - That vTPM is part of a "Software root of trust (hypervisor dependent)" and is "managed by the host hypervisor".

**Measured on our own fixtures.** The two recordings in `internal/tee/parse/snp/testdata` come from the same guest,
one booted with an initrd (`gcp-n2d-*`) and one without (`gcp-n2d-initrdless-*`).

- They carry the identical MEASUREMENT, `f67900851e25a378f04b4d4b5f1ea4410c11deb74e97ddcc7e3363a33f6e1dbb7880d304005387bc98e9cbdceec4ea28`
  (`xxd -s 0x90 -l 48` on both report files, 2026-11-11).
- HOST_DATA (C0h), FAMILY_ID (10h), IMAGE_ID (20h) and ID_KEY_DIGEST (E0h) are all zero in both reports (`xxd` at
  those offsets).
- So on GCP the AMD signature identifies the firmware and says nothing about the workload: a different kernel
  command line, a different initrd, or none at all, gives the same AMD-signed value.
- Since 2026-11-15 this holds on all eight reports of that guest (adding six boots with an application, and
  AUTHOR_KEY_DIGEST, also zero), and is a code test: `TestReports_IdentifyFirmwareNotVM`. The same reports show that
  REPORT_ID is new on every boot and CHIP_ID changes with the host. It is also why the same-VM relay of the last
  section can pass (see there).

**Consequence for Attestamp.** An `intel-tdx` measurement set has one root of trust; an `amd-sev-snp-*` set has two.
This is the same gap that `internal/tee/parse/snp/doc.go` records as the same-VM problem (test-matrix row T10).

**What would close it.**

- An SVSM at VMPL0 can host a vTPM inside the guest, bringing those PCRs under the launch measurement. See e-vTPM
  (Narayanan et al., ACSAC 2023, arXiv:2303.16463) and SNPGuard (Wilke and Scopelliti, SysTEX'24, arXiv:2406.01186).
- Google Cloud's documentation offers no SVSM: the attestation overview does not mention one, and neither do the
  release notes. So this is not available to us. A Google community blog post (June 2026) mentions an SVSM vTPM
  "preview"; it is not in Google's documentation and was not tried.

## Debug mode

A debug TEE lets the host read and change the guest's memory, so its measurements prove nothing.

| | Nitro | TDX | SEV-SNP |
|---|---|---|---|
| Signal | PCR0 all zero | TDATTRIBUTES bit 0 (TUD.DEBUG) | POLICY bit 19 (DEBUG) |
| What the parser does | `Parse` drops every all-zero PCR and fails if all three are zero; `Info.Debug` reports PCR0 all zero | `Parse` refuses (`ErrBadDocument`); `Info.Debug` reports it | `Parse` refuses (`ErrBadDocument`); `Info.Debug` reports it; a POLICY that does not parse counts as debug |
| Can our cloud launch one? | yes: `nitro-cli run-enclave --debug-mode` | **no**, on GCP (checked 2026-11-16): the Compute API's `ConfidentialInstanceConfig` has the technology type and nothing else in `v1`, plus a paravisor interrupt and vTPM mode in `alpha` (discovery documents, revision 20261106); no field in either API is named for debug, and `gcloud compute instances create` (SDK 592.0.0) has no such flag | **no**, on GCP, for the same reason |
| Tested by | **live, 2026-11-15** (`e2e/debug-mode-check.sh`): a real debug enclave's document, genuinely signed, had all-zero PCRs and was refused (exit 7). Not a code test: a debug document is signed, so it cannot be made up, and the recorded one carries an instance ID and is not committed (test-matrix row T8) | `TestParse_DebugTDIsRefused` (bit set in a copy of the fixture). Live: not applicable on GCP; every live quote so far passed this check, i.e. had the bit clear | `TestParse_DebugGuestIsRefused` (bit set in a copy of the fixture). Live: not applicable on GCP; every live report so far passed this check, i.e. had the bit clear |

The CVM check stays although GCP cannot launch a debug guest: the same evidence can come from another host (a KVM
host of one's own sets the SEV-SNP guest policy and the TD attributes itself), and nothing stops GCP from adding the
option later. A refusal tested only on a flipped bit is the most this platform allows.

## Values left out, and why

The rule on every platform: register what describes the image, leave out what describes the machine, the launcher or
the run. Otherwise the same image would register differently on every instance or guest.

| Platform | Left out | Why | Where |
|---|---|---|---|
| Nitro | PCR3 | the IAM role of the parent instance | `aws/measure.go` comment |
| | PCR4 | the parent instance's ID; non-zero on a real instance | same; `aws/measure_test.go` asserts it is dropped |
| | PCR8 | the enclave image's signing certificate | `aws/measure.go` comment |
| TDX | MROWNER | set by whoever launches the TD; assumed to differ per guest, not measured (non-zero in our fixture) | `tdx/measure.go`; `tdx/measure_test.go` asserts it is excluded and that the fixture's is non-zero |
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
| Same-VM guarantee | **yes, by construction**: one document, one signer, produced inside the enclave's own platform | **yes, by construction**: one quote over one TD report | **no. Reproduced on 2026-11-16 (T10):** VM B's AMD-signed report, requested over SHA3-512 of VM A's vTPM attestation, verifies, yields A's values and finds the registration made for A (below) |
| What would close it | n/a | n/a | boot files in the SNP launch measurement itself, or a vTPM inside the guest's trust boundary; neither is available to us on GCP today (`docs/build/sev-snp.md`, "Same VM"). A narrower idea is below, as a thing to study, not a fix |

"By construction" is a statement about the document format: Nitro and TDX each return **one** signed object, so there
are no two halves to mix and no binding to satisfy. It has not been tested by a relay experiment on those two; there
is nothing to relay.

### The SEV-SNP relay, measured (T10, 2026-11-16)

Two guests of one image, session 3 of 2026-11-16 (`docs/results/sessions-2026-11-16/amd-sev-snp.md`). VM A ran the
approved application, which the Auditor had registered; VM B ran a one-byte-changed one, which nobody registered. All
four documents were checked with `att-user ra-parse --format amd-sev-snp-initrd` against the same nonce:

| document | made how | result |
|---|---|---|
| A's own pair | A's report over A's vTPM attestation | **exit 0**, A's values (INITRD `11cd710e…`) |
| B's own pair | B's report over B's vTPM attestation | **exit 0**, B's values (INITRD `db9c9779…`); **not registered** |
| naive mix | B's **own** report, put next to A's vTPM attestation | **exit 5**, refused at the binding: `REPORT_DATA is not SHA3-512 of the vTPM attestation` |
| relay (`record-snp -relay-att`) | B's report **requested over** SHA3-512 of A's vTPM attestation | **exit 0**, `signature ok`, `nonce ok`, **A's** values; `lookup` finds the Auditor's registration for A |

**The binding works, and the relay passes it.** The naive mix shows the binding check doing its job: a report that was
not requested over these exact vTPM bytes is refused. The relay passes because B **did** request its report over those
bytes. REPORT_DATA = SHA3-512(vTPM attestation) proves only that whoever asked for the report held that vTPM blob at
the time; it does not prove that the blob came from the same VM's vTPM. Each half is genuine, each signature checks,
and the binding holds. Nothing in the evidence connects B's chip to A's vTPM.

Where the five compared values come from in the relay:

| value | from | signed by | in the relay |
|---|---|---|---|
| MEASUREMENT | the SNP report | AMD | **B's**. Equal to A's (`f6790085…`), as on every guest of the same machine type and firmware, so it cannot tell them apart |
| PCR4, KERNEL, CMDLINE, INITRD | the vTPM event log, replayed against the vTPM quotes | Google's AK | **A's** |
| (the nonce) | the vTPM quotes' `extraData` | Google's AK | A's quote; so a live relay needs A alive to sign the verifier's fresh nonce, in real time |

So four of the five values, and the nonce, come from A; the only value from B is one that every VM of that shape
shares. This is a property of SEV-SNP as GCP offers it, two documents from two signers joined by a hash, and not a
defect of this verifier. It is recorded, not patched. The old out-of-repository scripts had reported the same; this is the first
reproduction in this repository. The recordings are kept locally, not committed (they carry instance identifiers), so
T10 is a live result, like Nitro's T8: `docs/build/app-fixture-recording.md`, "Reproducing T10", says where they are
and how to make them again.

What else was seen: A's and B's reports carry different `CHIP_ID`s (the VMs ran on different hosts), and neither
`CHIP_ID` appears in the vTPM evidence. The structural absence was already measured on 2026-11-11 (MEASUREMENT
identifies only the firmware; HOST_DATA, FAMILY_ID, IMAGE_ID, ID_KEY_DIGEST all zero; pinned by
`TestReports_IdentifyFirmwareNotVM`); the relay now shows its consequence.

### A mitigation to study, not adopted

A's vTPM evidence does name A: the AK certificate Google signs carries A's instance (the numeric instance ID in its
subject, and the GCE instance-information extension, OID 1.3.6.1.4.1.11129.2.1.21, with the project and instance). B's
report has nothing to compare that with, but a verifier could **pin the instance**: register, next to the values, the
AK certificate's instance ID, and accept evidence only from that instance.

Not adopted, and not tested:

- **It would change what a registration means.** Today a registration says "these measurements are trusted", and any
  TEE that attests them is trusted: the Vendor can replace its VM and keep its registration. Pinning the instance makes
  *which VM* part of the registry key, so every new VM needs a new registration, which conflicts with the goal that
  changing the TEE needs nothing but the TEE.
- **It is not known that it would stop the relay.** It ties the vTPM half to one instance, not B's AMD report. By
  reasoning only (not tried): in the relay above the vTPM half *is* A's own, so a pin on A's instance would accept it
  as well, as long as A is alive to answer; what pinning would rule out is evidence from some other VM's vTPM. Whether
  that narrows the attack to anything useful has not been worked out, and no pinned verifier has been run against the
  recordings.
- It would also put instance identifiers into the registry, which is public.
