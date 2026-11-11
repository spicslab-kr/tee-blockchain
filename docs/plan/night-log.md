# Night log, 2026-11-10 → 11

An unattended overnight run. Times are UTC (KST = UTC+9). Every decision, change to the machine or
the cloud, and every blocker is written down here in the order it happened.

## Environment found at the start

| Item | State |
|---|---|
| Local Go | `go1.26.5`; the pin is `go1.26.1`. `.claude/settings.json` sets `GOTOOLCHAIN=go1.26.1`, but the Bash shell did not inherit it (`go env GOTOOLCHAIN` = `auto`). |
| gcloud | Installed (Cloud SDK 592.0.0) under `C:\Program Files (x86)\Google\Cloud SDK`, **not on Git Bash's PATH**. Authenticated; default project is `project-d9ca3c62-d377-4835-9b4`. Every gcloud call tonight prepends that folder to `PATH` for the one command; nothing was changed permanently. |
| aws | Installed under `C:\Program Files\Amazon\AWSCLIV2`, also not on Git Bash's PATH. Authenticated (root login session). |
| anvil | Not installed. |
| git | The working copy *is* a repository (2 commits, no remote). |

Correction to the environment check earlier in the same session: I reported `aws` and `gcloud` as
"not installed". They are installed; I had looked for the wrong file names in their default folders.

## Timeline

### 14:40 — AWS quota (read only)

`aws service-quotas list-requested-service-quota-change-history --service-code ec2`:
eu-north-1 has one request, "Running On-Demand Standard (A, C, D, H, I, M, R, T, Z) instances" → 16,
status **`CASE_CLOSED`** (created 2026-11-10 10:44 UTC). Not `PENDING` as the brief said. us-east-1
has no request. No AWS resource was created or changed.

### 14:45 — Step 0, TDX fixture

- `attestamp-tdx-vendor` (us-central1-a, c3-standard-4, TDX) was already **RUNNING**; I did not start it.
- Port 22: the rule `default-allow-ssh` (0.0.0.0/0, tcp:22) already exists. **No firewall rule was created.**
- First `gcloud compute ssh`: the existing key `~/.ssh/google_compute_engine` was used; gcloud
  manages it in project metadata. Set the safety timer: `sudo shutdown -h +240` (fires 18:38 UTC).
- The files were not in my login user's home but in `/home/bagss3366/` (the browser-SSH user).
  `sha256(quote.bin)` on the instance = `228da7b4…8a4d`, as expected.
- Copied both to `/tmp/` on the instance with mode 644 (`sudo install`), then fetched each with
  `gcloud compute scp` (PuTTY's pscp refuses two remote sources in one call, so one call per file).
  The base64 fallback was not needed.
- Local checks, all passed:
  - size 8000 bytes
  - sha256 `228da7b4e87f2fd21c7732d91bfe709f5fa81850f8216768b5b1d48fb46b8a4d`
  - header (little endian): version = 4, attestation key type = 2, TEE type = `0x00000081` (TDX)
  - `gcp-c3-quote.nonce` = 128 hex chars + newline = `sha512("attestamp-tdx-fixture-0001")`, recomputed locally
- Confirmed on the instance: `Ubuntu 24.04.5 LTS`, kernel `7.0.0-1017-gcp`, machine type `c3-standard-4`.
- 14:52 **stopped** `attestamp-tdx-vendor` (status `TERMINATED`). Nothing else tonight needs the TDX VM.
  The `/tmp` copies on its disk are harmless and were left there.

### 15:00 — Gate before starting

- `gofmt -l .` listed `internal/chain/register/{Vendor,auditor}/register.go`. `gofmt -w` on both made the working
  copy equal to HEAD again (HEAD was already formatted; only the working copy had drifted), so nothing was committed.
- **Decision: run every Go command with `GOTOOLCHAIN=go1.26.1`.** `.claude/settings.json` asks for it and the pin
  is the project's rule. Go downloaded the 1.26.1 toolchain into the module cache on first use (this is the only
  "install" Go did). With it, `go vet ./...` and `go test ./...` passed. The cmd e2e tests that need anvil skipped.

### 15:05 — A 1.1, TDX parser

- **go-tdx-guest, V4/V5 support (checked in the module source):**
  - `v0.3.1` (latest tag, 2024-02-16): `abi.QuoteToProto` and `verify.TdxQuote` handle **QuoteV4 only**.
  - `main` (pseudo-version `v0.3.2-0.20261109212116-fdf839f63af3`, 2026-11-09): handles **QuoteV4 and QuoteV5**,
    and adds `rtmr`, `gce` and `verify/dice` packages.
  - **Decision: v0.3.1.** Our fixture is version 4, a tagged release is a fixed point for reproducible builds, and it
    pulls fewer modules. Moving to V5 later means a newer library and a V5 fixture.
- Library behaviour relied on: with `GetCollateral=false` and `CheckRevocations=false`, `verify.TdxQuote` checks the
  PCK chain, the quote signature, the QE report signature and the QE report-data hash, offline. **Decision:** keep it
  offline (like Nitro); the TCB level from Intel PCS is therefore not judged. Written in `verify.go` and the doc.
- **Decision: check the chain ourselves before calling the library**, so a wrong root is `ErrChain` and a tampered
  body is `ErrSignature`, the same split as Nitro. The quote's own root is never trusted.
- **Root certificate:** `internal/tee/parse/tdx/root.pem` = go-tdx-guest's `verify/trusted_root.pem`
  (Intel SGX Root CA, sha256 `44:A0:19:6B:…:74:D3`, valid 2018-05-21 → 2049-12-31). It is byte-for-byte the third
  certificate of the PCK chain inside our real quote. I tried to fetch it independently from Intel; the URL I used
  (`certificates.trustedservices.intel.com/Intel_SGX_Provisioning_Certification_RootCA.der`) returned `NoSuchKey`,
  and I did not keep guessing URLs. **Still to do: compare with Intel's published copy.** `.gitignore` gained
  `!internal/tee/parse/tdx/root.pem`.
- PCK leaf in the fixture: valid 2026-10-15 → 2033-10-15. So `Parser.Now` zero means *now*: a quote has no time.
- **D4 decision — register MRTD, RTMR0, RTMR1, RTMR2** (names `MRTD`, `RTMR0`..`RTMR2`, same `<NAME><n>` shape as
  Nitro's `PCR<n>`). Reason: RTMR values are extended by the firmware, boot loader and kernel themselves, so RTMR2
  alone can be forged by a changed kernel that extends the expected value; only MRTD and RTMR0–1 would show that.
  This is the same reason Nitro registers PCR0–2 together. Excluded: RTMR3 (runtime, zero in the fixture),
  MROWNER (non-zero on GCP, per instance), MROWNERCONFIG, MRCONFIGID, MRSEAM. Cost: a firmware update (MRTD) or a
  kernel update (RTMR1) changes the registered value; RTMR0 probably depends on the VM shape — **unconfirmed**.
- The PCR→RTMR mapping (PCR1,7→RTMR0; 2–6→RTMR1; 8–15→RTMR2) is the TDVF design. Neither go-tdx-guest version
  documents it, and I have not confirmed it on GCP. The code comment says so.
- **Debug:** TDATTRIBUTES bit 0. `Parse` refuses a debug TD with `ErrBadDocument` (Nitro refuses through the
  all-zero PCR0 path in `Parse`; `Info.Debug` alone is only printed by the CLI, nothing refuses on it).
- **Nonce:** REPORTDATA is 64 bytes and `tee/request` nonces are 32. `CheckNonce` requires REPORTDATA = nonce
  followed by zeros. The fixture uses a full 64-byte nonce. The Phase 2 TDX requester must write the 32-byte nonce
  into the first half and zeros into the rest.
- Names, all taken from the aws package: `Format`, `RootPEM()`, `Parser`, `Err*` (same five), `Measurement(s)`,
  `Info`, `decode`, `loadRoots`. New private names: `imageRTMRs` (the counterpart of `imagePCRs`; TDX calls them
  RTMRs, not PCRs), `tdAttributesDebug`, `measurementSize`, `reportDataSize`, `pckChain`.
- Commits: `a0478d4` (fixture), `59d5e9a` (parser).

### 15:30 — A 1.2, adapters

`internal/tee/parse/{Vendor,auditor,user}/tdx.go`: `TDXFormat`, `TDXRootPEM()`, `TDXParser`, `tdxTranslate`, named
after `AWSFormat`, `AWSRootPEM()`, `AWSParser`, `awsTranslate`. The three copies differ only in the role name, as the
aws ones do. Commit `9efeb31`.

### 15:40 — A 1.3, transfer maps

- `TEEInfoKeys["intel-tdx"] = instance_name, project_id, zone, image`: the GCP counterparts of aws-nitro's
  `instance_id, region, ami_id`.
- `MeasurementNames["intel-tdx"] = MRTD, RTMR0, RTMR1, RTMR2`.
- **Decision:** an intel-tdx build config and measurement record require `build_env` (os, arch, go), `image.path`
  and `image.sha256`, like aws-nitro. The application initrd will contain att-ra-responder, whose bytes change with
  the Go toolchain. New helpers `needsBuildEnv` and `imageKind`.
- Commit `84c712d`.

### 15:50 — A 1.4/1.5, and a gap found in chain/lookup

- **Found:** `chain/lookup/*/ether.Reader.FindRecordKeys` always asked the registry under the constant
  `DefaultPlatform = "aws-nitro"`. The record key includes the platform, so an intel-tdx registration could never be
  found. `docs/plan/cvm-port.md` lists `chain/lookup` as "unchanged"; that is not true.
  **Fix:** `Reader.Platform` (empty keeps aws-nitro). Commit `8f537ec`.
- **Found:** `verify`, `audit` and `self-register` passed `--format` to `ra-request` but not to `ra-parse`, so
  parsing always used aws-nitro. `ra-serve --extractor fixture=` always labelled the fixture aws-nitro.
- CLI changes (commit `ad4d66a`): intel-tdx case in `ra-parse` (all roles); `--format` passed to `ra-parse`;
  `lookup --platform`; `audit` and `self-register` `--platform` now defaults to `--format`; `ra-serve --format`.
  `ra-parse` prints "timestamp none in the document" for TDX instead of year 1.
- Tests (commit `f34f8df`): `TestCmd_E2E_TDX_ServeRequestParse` **passes**. `TestRun_E2E_TDX_AuditorUser` (audit
  registers, verify says trusted, a wrong MRTD is not registered, lookup under aws-nitro finds nothing) is written but
  **skipped: anvil is not installed.**
- **anvil:** I downloaded Foundry v1.8.0 (2026-08-27; v1.9.0 was only 8 days old) for win32_amd64 into the scratchpad
  and checked its sha256 against the release's `.sha256` file
  (`e31241db9bc6ebdccaf1bcee9ee57e5896120166cfc5711570d280beb051982e`). Extracting `anvil.exe` into
  `~/.foundry/bin` was **refused by the permission classifier** (external code). Nothing was installed.
  To finish: install anvil, then `go test -run TDX ./cmd/`.

### 16:15 — B, deterministic cpio writer

- `internal/setup/build/cpio`: `Entry`, `Write`, `Bytes`, `FromDir`, `Placement` (`AppFirst`, `AppLast`), `Join`. No aws
  counterpart exists for any of these; the names describe what they hold. `AppFirst` (TDX) and `AppLast` (SEV-SNP)
  carry the reason in their comments: on TDX the appended archive failed to decompress (cvm-port.md 3.2); on SEV-SNP
  the 2026-11 runs appended and booted. Why they differ is not established, and the docs say so.
- Tests: same input → same bytes; reversed input order → same bytes; one byte or the execute bit changed → different
  bytes; header fields read back independently; duplicates and `\` in names refused.
- **mkcpio.py equality, proved two ways:**
  - `testdata/mkcpio-golden-app.cpio`: the `opt/app/app` layout, written by `mkcpio.py` on **WSL 2 Ubuntu**
    (Python 3.14.4, umask 022). WSL was already installed; nothing was installed for this.
  - `testdata/mkcpio-golden-flat.cpio`: written by `mkcpio.py` on Windows Python 3.14.6. This is only valid for a tree
    with no subdirectories and only executable files: on Windows `os.path.relpath` uses `\` and every file passes
    `os.access(X_OK)`. This was checked: both files came out `0100755`.
  - `TestWrite_MatchesMkcpioLive` (with `ATTESTAMP_MKCPIO` set) **passed** against the script on Windows.
- `.gitattributes` gained `*.bin -text`, `*.cpio -text` (and later `*.pb -text`) so fixtures are never converted.
  The committed TDX quote was checked to be byte-identical (`git cat-file` → same sha256).
- Commits `987cb50`, `88fc9f6`.

### 16:20 — C, research delegated

A background agent read the AMD ABI spec (56860 rev 1.59), the VLEK spec (58369), the TCG TPM 2.0 spec (r1.59),
go-sev-guest v0.15.0 and go-tpm v0.9.8, and fetched AMD's KDS chains. It ran no cloud command and edited nothing. Its
findings, each with a source, are written up in `docs/build/sev-snp.md`, "Time and freshness". Not obtained: AMD's
KDS spec (57230), so what it says about VCEK lifetime is marked "not confirmed".

### 16:35 — D1, SEV-SNP instance and evidence

- **Created `attestamp-snp-vendor`** (n2d-standard-2, us-central1-a, SEV_SNP) with `e2e/cloud/gcp-cvm.sh snp create
  vendor`, as the brief asked. The firewall rule `attestamp-ra` already existed; **no firewall rule was created**.
  Set `sudo shutdown -h +240` on it. Instances running at the same time: 1 (TDX was stopped).
- Guest: Ubuntu 24.04.5, kernel `7.0.0-1017-gcp` (the 2026-11 scripts hard-code `7.0.0-1014-gcp`), `/dev/sev-guest`,
  `/dev/tpm0`, ConfigFS TSM. No tpm2-tools, no Go, no `snpguest`, no `gotpm` on the image.
- **Blocked:** getting `gotpm`. The published module `github.com/google/go-tpm-tools/cmd` (latest v0.4.5) does not
  resolve (`verifier/v0.4.4` tag is missing), and cloning the repository to build it was **refused by the permission
  classifier** (external code). `snpguest` was therefore not attempted either.
- **Decision:** record the evidence without those tools. I wrote a small recorder (now committed as
  `internal/tee/parse/snp/testdata/record/main.go`): the go-tpm-tools client library (the same module proxy path
  `verify_att` uses) for an RSA GCE-AK attestation over a fixed nonce, and ConfigFS TSM (no tool) for the SNP report,
  REPORT_DATA = SHA-512 of the attestation — the 2026-11 binding.
- Recorded on the first try at 17:23 UTC. Fixed nonce: SHA-256 of `attestamp-snp-fixture-0001`.
  Report: VERSION 5, POLICY `0x30000` (not debug), SIGNING_KEY 0 (VCEK). The host returned a certificate table.
- **Identifying data:** `gcp-n2d-att.pb` carries the project ID, zone, instance name, numeric instance ID and project
  number in the Google-signed AK certificate. I stopped before committing it, as CLAUDE.md rule 4 and the
  record-fixture command require.

### 17:30 — E started, then stopped by the user

- Copied `build_app.sh`, `app_main.go`, `mkcpio.py` to the instance. Running `build_app.sh` through `gcloud.cmd` failed
  twice (Windows `cmd` splits a quoted program path when an argument is also quoted). The user then stopped the turn.
  `build_app.sh` never ran on the instance (no result file, no process).
- The user added shell-command rules to CLAUDE.md (one command per call, no loops or variable assignments, gcloud and
  aws by name). Commits `31f55a3`, `87040bf`.
- Commit `feee2c1` (`.claude/settings.json`: gcloud compute ssh and scp no longer ask) was **not made by me**; it
  appeared during the pause.

### 2026-11-11 — after the user's go-ahead

- **Stopped `attestamp-snp-vendor`** after checking that nothing ran on it (`pgrep` found nothing).
- **SNP fixture committed** on the user's decision: the Nitro fixture already carries an EC2 instance ID and its test
  asserts it, and without a committed fixture the tests do not run in CI. `testdata/README.md` lists every identifier
  in each file. Commit `4bfdb53`.

### D2/D3, SEV-SNP parser

- **Dependencies:** go-tpm-tools v0.4.11 (vTPM quote, event log, GRUB parsing; the version `verify_att` used) and
  go-sev-guest v0.15.0 (report, certificate table, AMD chain and signature). go-tpm-tools requires go-tdx-guest
  `v0.3.2-0.20250814004405-ffb0869e6f4d`, so **my v0.3.1 choice for TDX was raised by Go's version selection**. That
  revision still decodes QuoteV4 only and takes `Options.Now` as a `*TimeSet`; the TDX parser was adapted in the same
  commit and its tests pass.
- **What the recorded evidence showed** (read with a throwaway test, removed): both chains verify offline; the
  guest booted **without an initrd** — GCP's Ubuntu image sets `GRUB_FORCE_PARTUUID` and GRUB logs "attempting
  initrdless boot". This is the cvm-port.md 3.3 problem, now confirmed on GCP.
- **D3a, container:** one CBOR map `{report, certs, vtpm}`, core deterministic encoding, unknown or duplicate keys
  refused. Reasons: one byte string on the wire; CBOR already used for Nitro; no base64; the host's certificate table
  lets the AMD chain be checked offline.
- **D3b, format strings:** two, `amd-sev-snp-initrd` and `amd-sev-snp-verity`, because `measurementHash` covers the
  name list and the two options register different numbers of values.
- **Values decided:** MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD (+ PCR12 for verity), SHA-256 bank. The brief framed
  Option 1 as one value and Option 2 as two; I added the boot-chain values for the same reason as TDX's D4 (a changed
  GRUB or kernel could otherwise report the expected initrd). SHA-256 because the verity hook extends only that bank
  and the 2026-11 Auditor scripts computed SHA-256.
- **Root:** `internal/tee/parse/snp/root.pem` is Google's "EK/AK CA Root" (go-tpm-tools
  `server/ca-certs/gcp_ek_ak_ca_root.crt`, converted from DER; sha256 `59:47:59:59:…:BE:B2`). Our AK chains to it, not
  to the older `tpm_ek_root_1.cer` that go-tpm-tools' `GceEKRoots` uses. AMD roots: go-sev-guest's embedded ones.
  `.gitignore` gained `!internal/tee/parse/snp/root.pem`.
- **Names:** `FormatInitrd`/`FormatVerity` (two formats, so `Format` alone would not do), `Encode` (the Vendor side
  needs to pack; no aws counterpart), `MeasurementNames` (exported so `core/transfer` can be tested against it); role
  adapters `SNPInitrdFormat`, `SNPVerityFormat`, `SNPRootPEM()`, `SNPParser`, `snpTranslate`.
- **Tested on the recording:** verify passes; wrong cloud root → `ErrChain`; before the AK was issued → `ErrChain`;
  tampered report → `ErrSignature`; a vTPM attestation the report does not cover → `ErrSignature`; nonce; debug policy
  refused; both formats refuse the initrdless boot; MEASUREMENT, PCR4, KERNEL, CMDLINE asserted (CMDLINE also
  recomputed with `sha256sum`). **Not testable on this recording:** INITRD and PCR12 extraction.
- Wiring: adapters (3 roles), `core/transfer` keys and names (with a test tying them to the parser), `ra-parse` case,
  `cmd/snp_e2e_test.go`. Commits `6d512e0` … `5de41a2`.

### Docs

`docs/build/tdx.md` (new), `docs/build/sev-snp.md` (rewritten: values, D3, time and freshness, variables, instance,
procedure, same-VM limit, not done), README Platform section, STRUCTURE.md platform table, cvm-port.md status and the
`chain/lookup` correction. Commits `9c92d45`, `e84e79f`.

### E, the seven SEV-SNP tests — not run

Not run on a new instance, by the user's instruction after the pause (only work that needs no VM). The scripts need
`snpguest` and `gotpm`, which could not be installed (above). `e2e-work/results/amd-sev-snp.jsonl` was not written.

## A person must install these

| what | why | command |
|---|---|---|
| anvil (Foundry) | `TestRun_E2E_TDX_AuditorUser` and every other anvil test skip without it | Foundry v1.8.0 for Windows is already downloaded and checked (sha256 `e31241db…982e`) at `C:\Users\bags3\AppData\Local\Temp\claude\640ae8d0b7d67827\31cf8d4a-0cc0-465b-bf27-e1c70ff9dce1\scratchpad\foundry-dl\foundry.zip`. Extract `anvil.exe` into `%USERPROFILE%\.foundry\bin\` (where `cmd/register_e2e_test.go` looks), then `go test -run TDX ./cmd/` |
| `gotpm` on the SEV-SNP instance | the 2026-11 scripts call `~/gotpm attest --key gceAK` | on any machine with Go: `git clone --branch v0.4.11 https://github.com/google/go-tpm-tools`, `cd go-tpm-tools/cmd`, `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o gotpm ./gotpm`, then `gcloud compute scp gotpm attestamp-snp-vendor:gotpm --zone us-central1-a` |
| `snpguest` on the SEV-SNP instance | the scripts call `~/snpguest report`, `certificates`, `fetch ca`, `verify attestation` | on the instance: `curl -L -o ~/snpguest https://github.com/virtee/snpguest/releases/download/v0.11.0/snpguest` and `chmod +x ~/snpguest`. Which version the 2026-11 runs used is unknown; `userverify_grub.py` expects the output line `VEK signed the Attestation Report` |
| apt packages on the instance | the verity and in-VM check scripts | `sudo apt-get install -y tpm2-tools squashfs-tools cryptsetup-bin` (not blocked; not attempted because E was not run) |

`verify_att` (the scripts' vTPM verifier) is already cross-compiled at
`C:\Users\bags3\AppData\Local\Temp\claude\640ae8d0b7d67827\31cf8d4a-0cc0-465b-bf27-e1c70ff9dce1\scratchpad\vmtools\verify_att`;
copy it to `~/verify_att` on the instance.

## To run E on a new instance

Replace the hard-coded values in the 2026-11 scripts with this instance's (read from the recorded event log of
`attestamp-snp-vendor`; they belong to that instance's disk and change for a new one):

| in the scripts | 2026-11 value | `attestamp-snp-vendor` |
|---|---|---|
| `search --fs-uuid` (`gcp_boot_option1.sh`, `gcp_boot_option2.sh`) | `6a4d7abf-…9962` | `653dd231-d061-4b9e-bc8b-6d2ac57dffe4` |
| `root=PARTUUID=` (same, and `vendor_publish.sh`) | `10c90fd7-…cf47` | `27c09ec4-48e8-4dfd-be36-678080b236c7` |
| kernel (same, `auditor_publish.sh`) | `7.0.0-1014-gcp` | `7.0.0-1017-gcp` |
| `/home/chang/` (`vendor_evidence.sh`) | | `/home/bags3/` |
| `efibootmgr -n 0002` / `-n 0003` | the UEFI boot entry for GRUB | read with `efibootmgr` on the instance; not checked |
| VM IPs (`run_round.sh`, `relay_attack.sh`) | | read with `gcloud compute instances list` after each start; do not write them into a committed file |

Then: two instances (vendor, auditor), `build_app.sh 1.22.2` on both (T1 same hash), `build_app.sh` with another Go
version (T2), the same build as a second Linux user so the path differs (T3), then the Option 1 and Option 2 boot,
tamper and check scripts (T4–T7). After each reboot, check that `/proc/sys/kernel/random/boot_id` changed. Write one
JSONL line per test to `e2e-work/results/amd-sev-snp.jsonl` (format in `docs/plan/test-matrix.md`).

## Final state

- `gcloud compute instances list` at the end:
  `attestamp-snp-vendor` (us-central1-a, n2d-standard-2) **TERMINATED**,
  `attestamp-tdx-vendor` (us-central1-a, c3-standard-4) **TERMINATED**. Both are kept (stop, not delete).
- Cloud changes made: one instance created (`attestamp-snp-vendor`); two instances stopped. No firewall rule, quota
  request or other GCP/AWS setting was changed. The SSH key gcloud manages in project metadata was already there.
- AWS: read only. The vCPU request in eu-north-1 is `CASE_CLOSED`, not `PENDING`.
- Local: nothing installed. Go 1.26.1 was fetched into the module cache by `GOTOOLCHAIN`.
- Gate at the end: `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` all `ok`.

# 2026-11-11, daytime (the user present)

Times are UTC.

## SEV-SNP evidence with an initrd

- 03:51 started `attestamp-snp-vendor`; `sudo shutdown -h +240`. The external IP changes on every start; it was read
  from the start output and not written down.
- `GRUB_FORCE_PARTUUID` is set in `/etc/default/grub.d/40-force-partuuid.cfg`, not in `/etc/default/grub`. Kept the
  original as `/root/40-force-partuuid.cfg.orig`, commented the line out, ran `update-grub`, rebooted. The first
  `systemctl reboot` never ran (the SSH connection dropped before it; `boot_id` was unchanged), the second did.
  New `boot_id`; serial console shows the initramfs unpacked; the event log has GRUB's `initrd` command. Steps and
  how to undo them: `docs/build/sev-snp.md`, "The boot setting changed on `attestamp-snp-vendor`".
- Recorded once more with the fixed nonce (still SHA-512). The parser accepted it, and its INITRD equalled
  `sha256sum /boot/initrd.img-7.0.0-1017-gcp` on the instance. Stopped the VM (about 20 minutes running).
- This recording was then **discarded** because of the SHA3-512 change below, and the evidence was recorded again.

## Decision 2026-11-11: the SEV-SNP binding is SHA3-512

Decided by the project lead the same day: REPORT_DATA = **SHA3-512** of the vTPM attestation instead of SHA-512. Both
are 64 bytes, the size of REPORT_DATA.

- Repository: `internal/tee/parse/snp` (`bindingSum`, Go's standard `crypto/sha3`; no new dependency) and
  `testdata/record/main.go`.
- 04:28:50 started `attestamp-snp-vendor` again, `shutdown -h +240` (fires 08:30), checked that it still booted with
  the initrd (`/proc/cmdline` has `root=UUID=…`), copied the rebuilt recorder (sha256 `2c6ea569…bf41` on both sides),
  recorded with nonce text `attestamp-snp-fixture-0001`, fetched the four files (sha256 equal on both sides), stopped
  the VM at 04:34:45 (GCP's `lastStopTimestamp`). REPORT_DATA `82ded258…142e` equals `openssl dgst -sha3-512` of `att.pb` on Windows and on the
  instance (OpenSSL 3.0.13), and Python's `hashlib.sha3_512`.
- **Fixtures, on the user's decision to keep both:** the new recording is `gcp-n2d-*`; the 2026-11-10 one was renamed
  `gcp-n2d-initrdless-*`. It is the only evidence of an initrdless boot and of the old binding, and a signed event log
  cannot be made up in a test. Tests: every `amd-sev-snp-initrd` value asserted against a value read outside the
  parser; verity refused without a root hash; initrdless refused; SHA-512 binding refused (`VerifySig`) and its
  REPORT_DATA shown to be SHA-512; the cmd test now expects five values from `ra-parse`.
- **Still untested on real evidence: PCR12** (no recording has the verity hook) and an initrd carrying an application.

### Changes outside the repository: `C:\Users\bags3\Desktop\SEV-SNP-vTPM-코드-11-10\SEV-SNP-vTPM-코드\`

Scope, on the user's decision: every file of the **GCP** SNP+vTPM pair that makes or checks the binding. The AWS
NitroTPM experiment (`awscollect.sh`, `awscollect2.sh`, `go-verifier/aws`, `awsverify.py`) was **not** changed. Before
each change the original was copied next to it as `<file>.sha512.bak` (`cp -p`), so results from 2026-11-10 and earlier
can be tied to the SHA-512 versions. Korean comments were left as they are, including ones that still say SHA-512
(`relay-test/relay_attack.sh` line 16). The shell scripts now need `openssl` with SHA3 (OpenSSL 1.1.1 or later; the
GCP Ubuntu 24.04 image has 3.0.13). Both Go programs pass `go vet` for linux/amd64 after the change.

Lines quoted exactly, before → after:

`vendor/vendor_evidence.sh`, line 7
```
before: sha512sum att.pb | cut -d' ' -f1 | xxd -r -p > rd.bin
after:  openssl dgst -sha3-512 -binary att.pb > rd.bin
```

`user/in-vm-check/collect_grub.sh`, line 10
```
before: sha512sum ev_$V/att.pb | cut -d' ' -f1 | xxd -r -p > ev_$V/rd.bin
after:  openssl dgst -sha3-512 -binary ev_$V/att.pb > ev_$V/rd.bin
```

`user/in-vm-check/collect_verity_grub.sh`, line 10
```
before: sha512sum ev_$V/att.pb | cut -d' ' -f1 | xxd -r -p > ev_$V/rd.bin
after:  openssl dgst -sha3-512 -binary ev_$V/att.pb > ev_$V/rd.bin
```

`relay-test/relay_attack.sh`, line 18 (only this part of the line changed; the rest is unchanged)
```
before: sha512sum ../att_from_x.pb | cut -d' ' -f1 | xxd -r -p > rd.bin
after:  openssl dgst -sha3-512 -binary ../att_from_x.pb > rd.bin
```

`user/user_verify.py`, line 24
```
before: check('SNP 보고서 사용자 데이터 = SHA-512(vTPM 증명)', hashlib.sha512(att).digest() == rep[0x50:0x90])
after:  check('SNP 보고서 사용자 데이터 = SHA3-512(vTPM 증명)', hashlib.sha3_512(att).digest() == rep[0x50:0x90])
```

`user/in-vm-check/userverify_grub.py`, line 17, and `user/in-vm-check/userverify_grub2.py`, line 17 (identical)
```
before: check('SNP 보고서 사용자 데이터 = SHA-512(vTPM 증명)', hashlib.sha512(att).digest() == rep[0x50:0x90])
after:  check('SNP 보고서 사용자 데이터 = SHA3-512(vTPM 증명)', hashlib.sha3_512(att).digest() == rep[0x50:0x90])
```

`user/verify_att/main.go`, lines 5, 36, 37
```
before: 	"crypto/sha512"
after:  	"crypto/sha3"

before: 	sum := sha512.Sum512(raw)
after:  	sum := sha3.Sum512(raw)

before: 	fmt.Println("SHA-512(att.pb) =", hex.EncodeToString(sum[:]))
after:  	fmt.Println("SHA3-512(att.pb) =", hex.EncodeToString(sum[:]))
```

`user/go-verifier/gcp/main.go`, lines 17, 212, 295, 373, 374, 381
```
before: 	"crypto/sha512"
after:  	"crypto/sha3"

before: 	check(bytes.Equal(r.GetReportData(), wantReportData), "Report Data = SHA-512(vTPM 증명)", short(r.GetReportData()))
after:  	check(bytes.Equal(r.GetReportData(), wantReportData), "Report Data = SHA3-512(vTPM 증명)", short(r.GetReportData()))

before: 	sum := sha512.Sum512(raw)        (line 295, before verifySNP(snp, sum[:]))
after:  	sum := sha3.Sum512(raw)

before: 	step("[3] 증명의 SHA-512를 Report Data에 넣어 SNP 보고서 받기")
after:  	step("[3] 증명의 SHA3-512를 Report Data에 넣어 SNP 보고서 받기")

before: 	sum := sha512.Sum512(raw)        (line 374, before snpReport(sum))
after:  	sum := sha3.Sum512(raw)

before: 	line("SHA-512(증명) = %s…", hex.EncodeToString(sum[:])[:32])
after:  	line("SHA3-512(증명) = %s…", hex.EncodeToString(sum[:])[:32])
```

`README.md`, line 71
```
before: | `vendor_evidence.sh` | User 난수로 vTPM 증명을 받고, 그 SHA-512를 Report Data로 SNP 보고서를 받아 묶음 (GCP) |
after:  | `vendor_evidence.sh` | User 난수로 vTPM 증명을 받고, 그 SHA3-512를 Report Data로 SNP 보고서를 받아 묶음 (GCP) |
```

Backups made (10 files): `vendor/vendor_evidence.sh.sha512.bak`, `relay-test/relay_attack.sh.sha512.bak`,
`user/in-vm-check/collect_grub.sh.sha512.bak`, `user/in-vm-check/collect_verity_grub.sh.sha512.bak`,
`user/go-verifier/gcp/main.go.sha512.bak`, `user/user_verify.py.sha512.bak`,
`user/in-vm-check/userverify_grub.py.sha512.bak`, `user/in-vm-check/userverify_grub2.py.sha512.bak`,
`user/verify_att/main.go.sha512.bak`, `README.md.sha512.bak`.

None of the changed scripts has been run since the change.

## Decision 2026-11-11: SEV-SNP on GCP only, AWS for Nitro only

Decided by the user the same day. Each platform runs on one cloud: Nitro on AWS, TDX on GCP, SEV-SNP on GCP. No AWS
SEV-SNP work from now on. Applied to:

- `docs/plan/cvm-port.md`: cloud placement stated at the top; Phase 4b is GCP only and AWS SEV-SNP is marked "decided
  against"; D1(c) asks only about GCP; D6 keeps the AWS reproduction as a record.
- `docs/build/sev-snp.md`: new "Only GCP" section (the AWS NitroTPM path, snapshot → AMI re-registered with TPM
  support, is "not our path"); VCEK in the body, VLEK moved to a footnote; one line that the SHA3-512 binding applies
  to the GCP path and an AWS path would have to match; the "AWS evidence" row removed from "Not done yet".
- `e2e/cloud/aws-snp.sh`: kept, with a header comment that it is reference only.
- `e2e/cloud/README.md`: the AWS SEV-SNP step removed from the order; its section renamed "Not used".
- The 2026-11 AWS SEV-SNP results are kept everywhere they appear, as the record that AWS gave the same results.

Also added to `docs/build/sev-snp.md`, "Not done yet", on the user's instruction: the 2026-10/11 shell scripts are
scaffolding outside the repository; the final implementation is Go in `internal/setup/build/snp` and
`internal/setup/launch/snp`; until then, any report of the seven tests writes its procedure into that document.

## Decision 2026-11-11: code under MIT

- `LICENSE` is now the MIT License, matching `contracts/evm/Registry.sol` (`SPDX-License-Identifier: MIT`), which
  had disagreed with the Apache-2.0 `LICENSE`. The text was copied from a MIT-licensed dependency in the module cache
  (`fxamacker/cbor` v2.9.3) and differs from it only in the copyright line.
- **The copyright line is a placeholder**, `Copyright (c) [yyyy] [name of copyright owner]`, on the user's
  instruction to leave it as it was: the Apache `LICENSE` named no owner either (only its appendix template did). It
  still has to be filled in.
- Documentation and images stay CC BY 4.0 (`LICENSE-CC-BY` kept), on the user's decision.
- Also changed: the README license section and `CONTRIBUTING.md` ("code you contribute is licensed as MIT").

## OpenZeppelin: pinned upstream, missing in this copy

The build-information audit found that this working copy has `.gitmodules` but no submodule gitlink in HEAD
(`git ls-tree -r HEAD contracts` lists only `README.md` and `Registry.sol`) and an empty
`contracts/evm/lib/openzeppelin-contracts`, so `Registry.sol` could not be recompiled here.

Resolved with the user: the original repository's gitlink points at `cab19933c33c2ad1d4c7a84864a3601dddfd16f3`, whose
commit message is "Release v5.7.0 (#6615)". This working copy is a zip extract, so the submodule contents and the
gitlink are missing. A proper clone (`git clone --recursive`, or `git submodule update --init --recursive`) has them.
The original repository was pinned; the problem existed only in this copy.

Written into `contracts/README.md` ("OpenZeppelin version") and `STRUCTURE.md` (`.gitmodules` holds the path and
URL; the gitlink is what pins the version). In the list of gaps for "everything needed to build is on GitHub", this
item is **resolved**.
