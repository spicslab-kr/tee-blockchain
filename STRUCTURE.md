# Project structure

This page lists every folder in the repository, then explains the one idea that shapes most of
`internal/`: **role packages** and **platform modules**. The [README](README.md) gives the overview;
[CONTRIBUTING](CONTRIBUTING.md) has the build and commit rules.

## Tree

```
.
├── go.mod, go.sum          Go module "attestamp" (`go 1.26.1`); the toolchain pin itself is internal/setup/build/GO_TOOLCHAIN
├── README.md               overview, install, quick start
├── CONTRIBUTING.md         build, test, layout and commit conventions
├── STRUCTURE.md            this file
├── .gitattributes          line-ending and diff rules
├── .gitmodules             path and URL of the OpenZeppelin submodule (contracts/evm/lib); the gitlink pins v5.7.0
├── .gitignore              keys, e2e work files, stray build outputs
│
├── cmd/                    one folder per binary, plus the end-to-end Go tests
│   ├── vendor/             att-vendor:       build, launch, send, self-register
│   ├── auditor/            att-auditor:      receive, build, launch, audit, registry-deploy, send
│   ├── user/               att-user:         verify (no key, no register)
│   ├── ra-responder/       att-ra-responder: runs inside the enclave and answers attestation requests
│   └── *_e2e_test.go       build the real binaries and drive them against a local anvil chain
│
├── internal/               all library code; nothing here is importable from outside the module
│   ├── tee/                talking to the enclave and reading what it says
│   │   ├── request/        send a nonce, receive an attestation document
│   │   └── parse/          verify the document's signature and nonce, extract the measurements
│   ├── setup/              getting an enclave image built and running
│   │   ├── build/          reproducible image build, reproducibility scan, toolchain and responder checks
│   │   └── launch/         start the enclave, bridge the port, measure it
│   ├── chain/              the Ethereum side: register and look up measurements
│   │   ├── register/       sign a measurement (EIP-712) and write it to the registry
│   │   └── lookup/         read registrations back (read only)
│   ├── core/               logic that is about measurements, not about a platform or a chain
│   │   ├── compare/        compare values and judge a registration (match, expiry, approver)
│   │   └── transfer/       the build-config and measurement-record files that travel between Vendor and Auditor
│   ├── cli/                the few things every command line shares (colour, progress lines)
│   └── doccheck/           no code: a test that every document reference (links, anchors, file paths) resolves
│
├── contracts/
│   └── evm/                Registry.sol (Solidity) and the OpenZeppelin submodule under lib/
│
├── e2e/                    shell scripts for runs against real instances and guests
│   ├── env.sh              shared settings (instance addresses come from an untracked env.local.sh)
│   ├── 0-Setup-*.sh        install the local tools, or upload the binaries to the instances; 99-Setup-local-stop.sh
│   │                       stops what they started (the instances themselves: e2e/cloud/aws-nitro.sh stop)
│   ├── flow1-auditor-registers/       Auditor rebuilds and registers; tamper steps included
│   ├── flow2-vendor-self-registers/   Vendor registers itself; tamper steps included
│   ├── *-check.sh, *-compare.sh       single-purpose Nitro checks (toolchain pin, build environment,
│   │                                  docker and nitro-cli, architecture, responder, debug mode, replay)
│   ├── cloud/              create, start, stop and delete the instances: aws-nitro.sh (Nitro), gcp-cvm.sh (TDX and
│   │                       SEV-SNP guests), aws-snp.sh (SEV-SNP on AWS, kept for reference); check-scripts.sh runs
│   │                       every action of the three against stand-in CLIs (also from go test ./cmd); README on
│   │                       accounts, cost, images and SSH
│   └── cvm/                scripts that run on a TDX or SEV-SNP guest: in-guest-verify.sh (the whole chain on
│                           the guest) and build-env-check.sh (T7)
│
└── docs/
    ├── build/              how to use it and the procedures: one guide per platform (aws-nitro, tdx, sev-snp),
    │                       what both sides must match (reproduce), the settings and hashes, the toolchain, the
    │                       platform comparison, runtime immutability (and what a locked guest would take),
    │                       recording test fixtures
    ├── results/            dated records: every run's report and its JSONL result lines (the Nitro baselines, the
    │                       confidential-VM sessions and the in-guest run, the trials, the 2026-11-12 audit), and
    │                       the record of the build-info gaps filled in 2026-11. Measured values are never changed;
    │                       labels, interpretation and status are updated as more is learned, each update
    │                       marked where it is made with when and what changed
    ├── status/             where things stand, as a pair: test-matrix.md (what is proven, and by what) and
    │                       build-info-gaps.md (what the repository still lacks, and the design questions)
    └── img/                the logo and the diagrams used by the README (SVGs)
```


## How a run moves through the folders

Every command is a short walk through the same few packages. The folder names below are the ones in
the tree above.

1. Vendor, `att-vendor build` and `launch`: `internal/setup/build` turns the Dockerfile into an
   enclave image and its measurements. `internal/setup/launch` starts the enclave and exposes its port.
   `cmd/ra-responder` is the program inside the enclave that answers attestation requests.
2. Vendor to Auditor, `send` and `receive`: `internal/core/transfer` writes and reads
   `build-config.json` and `measurement-record.json` through git.
3. Auditor, `att-auditor audit`: `internal/tee/request` fetches an attestation document from the
   Auditor's own enclave, `internal/tee/parse` verifies it and extracts the measurements,
   `internal/core/compare` checks them against the Auditor's build, and `internal/chain/register`
   signs and registers them in the Registry contract (`contracts/evm`).
4. User, `att-user verify`: `internal/tee/request` and `internal/tee/parse` do the same for the
   Vendor's enclave, `internal/chain/lookup` finds the registrations for that measurement, and
   `internal/core/compare` decides whether to trust them.

In flow 2 the Vendor runs step 3 itself with `att-vendor self-register`.

### Inside `internal/<feature>/`

Every feature folder under `internal/` (`tee/request`, `tee/parse`, `setup/build`, `setup/launch`,
`chain/register`, `chain/lookup`, `core/compare`, `core/transfer`) has the same two kinds of
sub-package.

```
internal/tee/request/
├── Vendor/          role package  (package vendor)
│   ├── aws/         platform module: AWS Nitro, implemented (role adapter over internal/tee/request/aws)
│   ├── tdx/         platform module: Intel TDX, implemented (role adapter over internal/tee/request/tdx)
│   ├── snp/         platform module: AMD SEV-SNP, implemented (role adapter over internal/tee/request/snp)
│   ├── gcp/         platform module: GCP Confidential Space, partial
│   └── azure/       platform module: Azure, empty
├── auditor/         role package  (package auditor), same shape
│   └── aws/ tdx/ snp/ gcp/ azure/
├── aws/, tdx/, snp/ the role-independent request code for Nitro, TDX and SEV-SNP
└── user/            role package  (package user), no platform sub-folders
```

Not every feature has every role. The table under "Which binary uses what" shows who exists where.

## The module concept

### Role packages: vendor, auditor, user

A role package holds the code one role's binary needs, and the types that belong to that role. The
three roles are Vendor (ships the software), Auditor (rebuilds it and registers it) and User
(verifies it). A role package is named after the role in lowercase (`package vendor`), and
`cmd/<role>/main.go` imports only the matching role packages.

Folder names: the Vendor folders are capitalised on disk (`Vendor`, in `tee/request`, `tee/parse`,
`setup/build`, `setup/launch`, `chain/register`, `chain/lookup`, `core/compare` and `core/transfer`)
while the `auditor` and `user` folders are lowercase. The Go package name inside is lowercase
`vendor` in all three cases, so imports read the same. Renaming the folders to `vendor` touches the
import paths in `cmd/vendor`, in the other role packages and in the tests.

### Platform modules: aws, gcp, azure

A platform module is the code that is specific to one TEE platform. It lives one level below a role
package (`internal/tee/request/Vendor/aws`) or, for logic that does not depend on the role, next to
the role packages (`internal/tee/parse/aws`). A role package picks the module by the attestation
document's format name (`aws-nitro`, `intel-tdx`, `amd-sev-snp-initrd`, `amd-sev-snp-verity`, `gcp-cs-jwt`,
`azure-maa-jwt`).

What is implemented, checked against the code:

| Platform | Implemented end to end | State in the tree |
| --- | --- | --- |
| aws (Nitro Enclaves) | Yes. | Request (NSM, vsock, proxy), document verification against the AWS Nitro root, measurement extraction, image build, enclave launch. |
| gcp (Confidential Space) | No. | `tee/request/<role>/gcp` can fetch a token from the launcher socket and has a test. `tee/parse/<role>/gcp.go` returns "not implemented" for every method. Nothing in `cmd/*` constructs either. There is no gcp folder under `setup/`. |
| azure (Confidential VM) | No. | `tee/request/<role>/azure` returns "not implemented" (it needs a vTPM library and MAA). `tee/parse/<role>/azure.go` returns "not implemented". Nothing in `cmd/*` constructs either. No azure folder under `setup/`. |
| tdx (Intel TDX on GCP) | Yes, on GCP: build, launch, request, verify, audit and the independent Auditor ran on real guests (2026-11-15/16). | `tee/parse/tdx` (shared) and `tee/parse/<role>/tdx.go`; `tee/request/tdx` (shared) and `tee/request/{Vendor,auditor}/tdx`; `att-ra-responder ra-serve --extractor tdx`; `cmd/*` parse, audit and verify `intel-tdx`. Build and launch: `setup/build/tdx` and `setup/launch/tdx` (shared) with `setup/{build,launch}/{Vendor,auditor}/tdx` adapters; `att-vendor`/`att-auditor` `build` and `launch --platform intel-tdx`. Tested in code on recorded quotes and without a guest (fake TSM, fake squashfs tools, a constructed event log), and live on GCP `c3-standard-4` guests: install, reboot and `launch --check` for both options (2026-11-15), and the Auditor's own rebuild on a second guest with `audit --vendor-tee` and a User's `verify`, both options, with a negative control (2026-11-16, `docs/results/sessions-2026-11-16/intel-tdx.md`). |
| snp (AMD SEV-SNP on GCP) | Yes, on GCP, with a known weakness: the two halves of the evidence are not bound to one VM (T10, reproduced 2026-11-16). | `tee/parse/snp` (shared) and `tee/parse/<role>/snp.go`; `tee/request/snp` (shared) and `tee/request/{Vendor,auditor}/snp`; `att-ra-responder ra-serve --extractor snp-initrd` or `snp-verity`; `cmd/*` parse `amd-sev-snp-initrd` and `amd-sev-snp-verity`. Build and launch: `setup/build/snp` and `setup/launch/snp` (shared) with `{Vendor,auditor}/snp` adapters; `build` and `launch --platform amd-sev-snp-*`. Tested in code on recorded evidence (the request side through a fake TSM and vTPM), and live on GCP `n2d-standard-2` guests: install, reboot and `launch --check` for both formats (2026-11-15), and the Auditor's own rebuild on a second guest with `audit --vendor-tee` and a User's `verify`, both formats, with a negative control (2026-11-16, `docs/results/sessions-2026-11-16/amd-sev-snp.md`). |

**aws**, **tdx** and **snp** work end to end (Nitro on AWS; TDX and SEV-SNP on GCP); what each one's evidence can and
cannot prove is compared in `docs/build/ra-comparison.md`, and what has been measured on each is in
`docs/status/test-matrix.md`. gcp and azure exist so the format names, the Parser interface and the folder layout are
already in place.

`internal/setup/build/aws` and `internal/setup/launch/aws` are shared: the Nitro image build (docker, the responder
layer, normalizing, the EIF and its PCRs, `--verify`) and launch (nitro-cli, the vsock bridge, the security-group port
report), with thin role adapters in `setup/{build,launch}/{Vendor,auditor}/aws`. Until 2026-11-17 each role had its
own full copy; the two were the same apart from one comment.
`internal/setup/build/cpio` is shared: the deterministic cpio writer for confidential-VM initrds.
`internal/setup/build/tdx` and `internal/setup/launch/tdx` are shared too: the TDX build (Option 1 and 2, `--verify`)
and launch (GRUB entry, post-boot check), with thin role adapters in `setup/{build,launch}/{Vendor,auditor}/tdx`, the
same two-layer shape as `tee/parse` and `tee/request`. `internal/setup/build/snp` and `internal/setup/launch/snp` are the
same for SEV-SNP. What the two CVM platforms do alike exists once: `setup/build/verity` (the Option 2 image, with a fake
`mksquashfs`/`veritysetup` in `verity/veritytest`) and `setup/launch/grub` (the GRUB entry and install).

`ether` plays the same part for chains: `chain/register/<role>/ether` and `chain/lookup/<role>/ether`
are the only chain modules, and they talk to any Ethereum-compatible chain.

### What every platform's parser must do

What Nitro gave and every platform had to give again: one request returns one self-contained, signed piece of
evidence; the measurement is the identity, so it must be reproducible; the nonce is inside the signature, so the
hardware proves freshness; and debug mode is detected and refused. (SEV-SNP is the exception to the first: two
documents from two signers, joined by a hash; `docs/build/ra-comparison.md`.)

Two rules hold for every platform, and a new platform's parser (`internal/tee/parse/<plat>`) needs its own version of
both:

- **Drop the values that identify the machine, not the image.** A registration names software, so a value that
  differs per host, per instance or per launcher would make the same software look different on every machine. Nitro
  discards PCR3 (the parent instance's IAM role), PCR4 (the parent instance's ID) and PCR8 (the image's signing
  certificate); SEV-SNP leaves out `CHIP_ID` (the physical processor), the TCB versions and HOST_DATA; TDX leaves out
  MROWNER, MROWNERCONFIG, MRCONFIGID and MRSEAM. The full list, with where each is enforced and tested:
  `docs/build/ra-comparison.md`, "Values left out, and why".
- **Refuse a TEE in debug mode.** A debug TEE can be inspected and changed from outside, so its values must not be
  trusted however they look. Each platform signals it differently and needs its own check: Nitro gives an all-zero PCR0
  (`aws/measure.go`, every PCR zero is refused), SEV-SNP sets POLICY bit 19 (`snp`, refused), TDX sets the DEBUG bit in
  TDATTRIBUTES (`tdx`, refused). `docs/build/ra-comparison.md`, "Debug mode".

### Build and launch on the three platforms

`build` has the same five steps on every platform, so the three read alike (`internal/setup/build/{aws,tdx,snp}`):

| step | Nitro (`aws`) | TDX (`tdx`) | SEV-SNP (`snp`) |
|---|---|---|---|
| 1 tools | install `docker`, `nitro-cli` | check `mksquashfs` and `veritysetup` (Option 2 only) and record their versions; nothing is installed | the same as TDX |
| 2 the software | `docker build` of the Vendor's Dockerfile | the application tree (`--app-dir`), read as files; a link or special file is refused | the same as TDX |
| 3 what we add | the attestation responder, layered into the image | the boot marker that lets `launch` prove the archive was unpacked; for Option 2 the squashfs + dm-verity image of the tree | Option 1: the same marker; Option 2: the image, which stays on disk |
| 4 reproducible bytes | the docker layers repacked in name order with time 0 | the cpio archive with fixed metadata (`setup/build/cpio`), **before** the Vendor's base initrd (`cpio.AppFirst`) | the archive **after** the base, zero-padded to 4 bytes (`cpio.AppLast`); without the padding the kernel drops the archive and still boots |
| 5 the measured thing | `nitro-cli build-enclave` → EIF and PCR0·1·2 | the combined initrd and its **SHA-384**, which GRUB extends into RTMR2 | the initrd's **SHA-256** (INITRD) and, for Option 2, the root hash and the PCR12 it gives |

Where the confidential VMs differ from Nitro:

- **Files, not a container.** The application is a tree that lands under `/opt/app` in the initramfs (Option 1), or a
  read-only squashfs image verified by dm-verity (Option 2): inside the initrd on TDX, covered by RTMR2; on disk on
  SEV-SNP, with the root hash on the kernel command line (`attestamp.roothash=`, so in CMDLINE) and the verifying
  initrd extending PCR12.
- **The responder is not in the measured image.** In a confidential VM it runs in the guest OS and only relays
  evidence the hardware signs, so it cannot forge any; its bytes stay out of the measurement.
- **The build cannot produce the registered values.** MRTD, RTMR0 and RTMR1 (TDX), and MEASUREMENT, PCR4 and KERNEL
  (SEV-SNP), come from the machine's firmware, boot loader and kernel, and the command line names the root disk. So
  `launch` is two runs with a reboot between them: **install** puts the initrd in `/boot` and makes a GRUB entry from
  the stock one the default (`setup/launch/grub`, shared); **check**, after the boot, verifies fresh evidence, requires
  the measured initrd to be the built one (and on SEV-SNP Option 2 PCR12 to be the predicted value) and the boot marker
  to be there, and writes the values to register. It is Nitro's `launch --build-measurements` and the measuring step
  in one. TDX replays the CC event log itself (`setup/launch/tdx`); SEV-SNP's parser already replays the vTPM log.
- **The base initrd is the Vendor's binary.** `mkinitramfs` writes a random seed, so the Auditor uses the Vendor's
  published base initrd and reproduces only the application part (`docs/build/tdx.md`, `docs/build/sev-snp.md`). Other
  ways are open design questions (`docs/status/build-info-gaps.md`).
- **Format strings.** TDX's two options move only RTMR2, so both register `intel-tdx`. SEV-SNP's two options register
  different numbers of values, and the measurement hash covers the list of names, so they are two formats:
  `amd-sev-snp-initrd` (5 values) and `amd-sev-snp-verity` (6).

**Shell or Go.** What decides a measured value is in reviewed, tested Go: the dm-verity image (`setup/build/verity`;
its fixed squashfs flags, times, block size, salt and UUID decide the root hash; it still calls `mksquashfs` and
`veritysetup`, because no Go writer is known to give the same bytes) and the GRUB entry (`setup/launch/grub`; it decides
the GRUB commands that go into RTMR2 and CMDLINE). The scripts they were ported from stay in `testdata` as the record of
how the fixtures were made, and a test fails if the verity script's parameters and the Go constants drift apart
(`setup/build/verity`). The initramfs hook
and local-bottom scripts stay shell: they run inside the initramfs, under busybox `sh`, and belong to the Vendor's base
initrd, not to `build`.

### The shared pieces

Logic that does not depend on the role exists once:

- `internal/tee/parse/aws` holds the real Nitro parsing (CBOR decoding, certificate chain and
  signature check, PCR extraction, the embedded AWS root certificate). Each role's
  `tee/parse/<role>/aws.go` is a thin adapter that converts to and from that role's own types.
- `internal/tee/request/aws`, `internal/setup/build/aws` and `internal/setup/launch/aws` hold the Nitro request (NSM,
  vsock, the proxy), image build and launch; each role's `<role>/aws` folder is a thin adapter (the build and launch
  ones only copy fields, as there are no role types to convert: that is on purpose, to keep the tdx and snp shape). The TDX and SEV-SNP
  code has the same two layers (`tee/{parse,request}/{tdx,snp}`, `setup/{build,launch}/{tdx,snp}`).
- `internal/core/compare/compare.go`, `check.go` and `types.go` hold the comparison and the trust
  decision. Each role's `core/compare/<role>` is a thin delegate that converts its own
  `Measurements` type into the shared `Items`.

`internal/setup/build` (root files: architecture, toolchain pin, responder build check) is shared by
all binaries that build; `internal/cli` is shared by every command.

### Why role code is copied per role

Each binary must contain only its own role's commands, and the User's binary has no `register` at
all. Go types are identified by package: `tee/parse/vendor.Measurements` and
`tee/parse/user.Measurements` are different types even when their fields match. So one shared
implementation cannot be imported by three roles without every binary linking every role's
commands and types. Instead, each role package is a copy (or a close copy) with its own types, and
the role-neutral logic is pulled out into the shared pieces above wherever Go allows it.

**Identical across roles (apart from the role name):**

| Folder | Identical between |
| --- | --- |
| `chain/register` (all of it, including the generated `registry.go`) | Vendor, auditor |
| `setup/build/<role>/aws` and `setup/launch/<role>/aws` | Vendor, auditor |
| `setup/build/<role>/tdx` and `setup/launch/<role>/tdx` | Vendor, auditor |
| `setup/build/<role>/snp` and `setup/launch/<role>/snp` | Vendor, auditor |
| `cmd/<role>/snp.go` | Vendor, auditor, apart from the next-step hint |
| `cmd/<role>/tdx.go` | Vendor, auditor, apart from the next-step hint (the Vendor registers, the Auditor audits) |
| `tee/request` (`request.go`, `nonce.go`, `tcp.go`, `wire.go`, `aws/*`, `gcp/*`, `azure/*`, `tdx/*`, `snp/*`) | Vendor, auditor; the user copy of `request.go`, `nonce.go`, `tcp.go`, `wire.go` is the same too |
| `tee/parse` (`parse.go`, `aws.go`, `gcp.go`, `azure.go`, `tdx.go`, `snp.go`) | Vendor, auditor, user |
| `chain/lookup` (`lookup.go`, `ether/reader.go`) | Vendor, auditor |
| `core/compare/<role>` | Vendor, auditor |
| `core/transfer`: `build_config.go`, `codec.go`, `digest.go`, `measurement_record.go`, `registration.go`, `git_common.go` | Vendor, auditor |

**Different, because the roles do different things:**

| File | How it differs |
| --- | --- |
| `core/transfer/<role>/transport.go` and `git.go` | Opposite directions. The Vendor sends the build config and receives the measurement record; the Auditor receives the build config (clone) and sends the measurement record. |
| `core/transfer/auditor/build_env_check.go` | Only the Auditor has it: the OS, CPU architecture and Go version check, the Amazon Linux release (receive) and the docker and nitro-cli versions (build --config). |
| `chain/lookup/user` (`lookup.go`, `ether/reader.go`) | The User's reader differs from the Vendor and Auditor ones. |
| `core/compare/user/compare.go` | The User has no register package, so it recomputes the measurement hash itself. |
| Tests in `core/transfer`, `chain/lookup/*/ether`, `core/compare/user` | Follow the code above. |

The User has no `setup`, no `chain/register` and no `core/transfer`. It also has no platform
sub-folders, because it only needs to ask and verify.

Three tests sit directly in a feature folder because they cross roles:
`chain/register/ether_test.go`, `core/transfer/vendor_auditor_test.go` and
`tee/request/user_to_vendor_test.go`.

### Why parse adapters are files and the others are folders

A platform's role adapter is one file in the role package under `tee/parse` (`tee/parse/<role>/aws.go`), but a
sub-folder under `tee/request`, `setup/build` and `setup/launch` (`tee/request/<role>/aws/`). The difference is
deliberate and is not to be made uniform:

- **What a binary links.** `tee/parse/<role>` imports `tee/request/<role>` for its types (`RADoc`, `Nonce`), so every
  binary that parses links the request role package. `att-user` links `tee/request/auditor` that way, but not
  `tee/request/auditor/aws` (`go list -deps ./cmd/user`). Were the Nitro request code a file in the role package, every
  binary that only needs the types would link the NSM ioctl, vsock and the proxy too. A sub-folder is linked only by
  the binaries that import it.
- **Names.** The platform folders reuse the same names, and the folder keeps them apart (`aws.Build`, `tdx.Build`):
  `Extractor` under `tee/request` (with `Format` on Nitro and TDX; SEV-SNP has two formats), `Build` and `Result`
  under `setup/build`, and under `setup/launch` `Launch` and `Result` on Nitro but `Install`, `Installed`, `Check` and
  `Measured` on TDX and SEV-SNP. In one package they would need prefixes, as the parse adapters have (`AWSParser`,
  `TDXFormat`).
- **Why parse is files.** The parse role package is itself the place that picks a parser by format, every binary
  needs every parser (a User verifies any platform), and each adapter is a short type conversion, so one package with
  one file per platform costs nothing.

### Which binary uses what

The table is read from the imports of each `cmd/*` package (`go list -deps ./cmd/<role>`).

| Module | Job | att-vendor | att-auditor | att-user | att-ra-responder |
| --- | --- | :-: | :-: | :-: | :-: |
| `tee/request/<role>` and `<role>/aws`, over `tee/request/aws` (shared) | Request an attestation document; the responder side runs inside the enclave | yes | yes | yes (role only) | yes (Vendor copy, with aws, tdx and snp, and the shared aws, tdx and snp) |
| `tee/parse/aws` (shared) | Verify a Nitro document and extract measurements | yes | yes | yes | no |
| `tee/parse/<role>` | Role-typed adapter over the shared parser | yes | yes | yes | no |
| `setup/build` (root) | Architecture, toolchain pin, responder build check | yes | yes | no | no |
| `setup/build/aws` (shared) and its `<role>/aws` adapters | Reproducible image build, scan, verify | yes | yes | no | no |
| `setup/launch/aws` (shared) and its `<role>/aws` adapters | Start the enclave, bridge the port, measure | yes | yes | no | no |
| `setup/build/tdx`, `setup/launch/tdx` (shared) and their `<role>/tdx` adapters | Build the TDX initrd; install it under GRUB and check the boot | yes | yes | no | no |
| `setup/build/snp`, `setup/launch/snp` (shared) and their `<role>/snp` adapters | Build the SEV-SNP initrd or image; install it and check the boot | yes | yes | no | no |
| `setup/build/verity`, `setup/launch/grub` (shared) | The Option 2 image, and the GRUB install, for both TDX and SEV-SNP | yes | yes | no | no |
| `core/transfer/<role>` | Build-config and measurement-record exchange, over git or files | yes | yes | no | no |
| `chain/register/<role>` and `/ether` | Sign and register a measurement; deploy the registry | yes | yes | no | no |
| `chain/lookup/<role>` and `/ether` | Read registrations | yes | yes | yes | no |
| `core/compare` (shared) and `core/compare/<role>` | Compare values, judge expiry and approver | yes | yes | yes | no |
| `cli` | Colour and progress output | yes | yes | yes | yes |

Two details the table hides:

- `att-user` does not link any role's commands, but it does link `chain/register/auditor/ether`
  (the generated Registry binding) and, through it, `chain/lookup/auditor`, `tee/parse/auditor` and
  `tee/request/auditor`. `chain/lookup/user/ether/reader.go` imports that binding directly.
- `att-ra-responder` is built from the Vendor request package only, and `build` layers it into the
  enclave image.

## Folder reference

| Path | What lives there |
| --- | --- |
| `cmd/<role>/` | `main.go` (flags and dispatch), plus `chain.go`, `compare.go`, `parse.go`, `quiet.go` wiring for that role; vendor and auditor add `arch.go`, `tdx.go` and `snp.go` (the CVM build and launch), vendor adds `self_register.go`, user adds `verify.go`, auditor adds `audit.go` |
| `internal/setup/build/GO_TOOLCHAIN` | The pinned Go version, read by `build` and by CONTRIBUTING |
| `internal/tee/parse/aws/` | Shared Nitro parser, `root.pem`, and a recorded document under `testdata/` |
| `internal/tee/request/testdata/` | A recorded Nitro attestation document used by tests |
| `internal/chain/*/<role>/ether/` | Go side of the registry contract; `registry.go` is generated by `go generate` from `contracts/evm/Registry.sol` |
| `contracts/evm/` | `Registry.sol`; `lib/openzeppelin-contracts` is a git submodule |
| `e2e/` | Shell scripts, not run by `go test`; the Go end-to-end tests are in `cmd/` |
| `docs/build/` | Guides and procedures a reader follows; the other docs folders point here for "how" |
| `docs/results/` | Dated records, cited as evidence by `docs/status/` and the guides. **Measured values (hashes, PCRs, initrds, exit codes, the JSONL lines) are never changed.** Labels, interpretation and status are updated as more is learned, and each update is marked where it is made, in italics, with its date, its commit and, for replaced text, what it said before. A later run adds a new file. Paths to other documents follow when a file moves (those are the only unmarked edits; `git log --follow` shows them) |
| `docs/status/` | `test-matrix.md` (what is proven, and by what) and `build-info-gaps.md` (what the repository still lacks, and the design questions): the current state as a pair, kept apart from the dated records so they are not lost among them, and updated as runs and fixes land |
| `docs/img/` | `logo.svg`, `flow-auditor.svg`, `flow-vendor.svg` and `verify.svg` |
