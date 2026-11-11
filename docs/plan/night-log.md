# Night log, 2026-11-10 → 11

An unattended overnight run. Times are UTC (KST = UTC+9). Every decision, change to the machine or
the cloud, and every blocker is written down here in the order it happened.

## Environment found at the start

| Item | State |
|---|---|
| Local Go | `go1.26.5`; the pin is `go1.26.1`. `.claude/settings.json` sets `GOTOOLCHAIN=go1.26.1`, but the Bash shell did not inherit it (`go env GOTOOLCHAIN` = `auto`). |
| gcloud | Installed (Cloud SDK 592.0.0) under `C:\Program Files (x86)\Google\Cloud SDK`, **not on Git Bash's PATH**. Authenticated; default project is `<project-id>`. Every gcloud call tonight prepends that folder to `PATH` for the one command; nothing was changed permanently. |
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
- The files were not in my login user's home but in the home of the user that had used the browser SSH.
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
| anvil (Foundry) | `TestRun_E2E_TDX_AuditorUser` and every other anvil test skip without it | Foundry v1.8.0 for Windows is already downloaded and checked (sha256 `e31241db…982e`) in the session's scratch folder (`foundry-dl/foundry.zip`). Extract `anvil.exe` into `%USERPROFILE%\.foundry\bin\` (where `cmd/register_e2e_test.go` looks), then `go test -run TDX ./cmd/` |
| `gotpm` on the SEV-SNP instance | the 2026-11 scripts call `~/gotpm attest --key gceAK` | on any machine with Go: `git clone --branch v0.4.11 https://github.com/google/go-tpm-tools`, `cd go-tpm-tools/cmd`, `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o gotpm ./gotpm`, then `gcloud compute scp gotpm attestamp-snp-vendor:gotpm --zone us-central1-a` |
| `snpguest` on the SEV-SNP instance | the scripts call `~/snpguest report`, `certificates`, `fetch ca`, `verify attestation` | on the instance: `curl -L -o ~/snpguest https://github.com/virtee/snpguest/releases/download/v0.11.0/snpguest` and `chmod +x ~/snpguest`. Which version the 2026-11 runs used is unknown; `userverify_grub.py` expects the output line `VEK signed the Attestation Report` |
| apt packages on the instance | the verity and in-VM check scripts | `sudo apt-get install -y tpm2-tools squashfs-tools cryptsetup-bin` (not blocked; not attempted because E was not run) |

`verify_att` (the scripts' vTPM verifier) is already cross-compiled in the session's scratch folder
(`vmtools/verify_att`); copy it to `~/verify_att` on the instance.

## To run E on a new instance

Replace the hard-coded values in the 2026-11 scripts with this instance's (read from the recorded event log of
`attestamp-snp-vendor`; they belong to that instance's disk and change for a new one):

| in the scripts | 2026-11 value | `attestamp-snp-vendor` |
|---|---|---|
| `search --fs-uuid` (`gcp_boot_option1.sh`, `gcp_boot_option2.sh`) | `6a4d7abf-…9962` | `653dd231-d061-4b9e-bc8b-6d2ac57dffe4` |
| `root=PARTUUID=` (same, and `vendor_publish.sh`) | `10c90fd7-…cf47` | `27c09ec4-48e8-4dfd-be36-678080b236c7` |
| kernel (same, `auditor_publish.sh`) | `7.0.0-1014-gcp` | `7.0.0-1017-gcp` |
| the 2026-11 author's home directory (`vendor_evidence.sh`) | | the login user's home on the instance (`~`) |
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

### Changes outside the repository: the folder of 2026-11 SEV-SNP scripts (`SEV-SNP-vTPM-코드/`, kept locally, not in git)

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

## Non-ASCII swept out of everything inside the Nitro EIF

Nitro PCR baseline must be taken AFTER this commit.
Changing files inside the EIF changes PCR0 and PCR2
(PCR1 is unaffected), and therefore the registered
measurementHash. No baseline existed at the time of this change,
so nothing had to be re-registered.

What goes into the EIF from this repository, from `layerResponder` in `internal/setup/build/{Vendor,auditor}/aws/build.go`
(lines 217-243): `/att-ra` (the `att-ra-responder` binary), `/att-entrypoint.sh` (`entrypoint.sh`, embedded) and
`/att-app-cmd` (written from the application image's command), plus the ENTRYPOINT set in the image config. There is no
`e2e/remote/` folder in this repository. The application's own files come from its Dockerfile, outside this repository.

`go list -deps ./cmd/ra-responder` links four packages of ours: `cmd/ra-responder`, `internal/cli`,
`internal/tee/request/Vendor`, `internal/tee/request/Vendor/aws` (plus `fxamacker/cbor` and `x448/float16`).

Non-ASCII found in those files (ripgrep `[^\x00-\x7F]`; the Bash `grep -P` refused to run in this shell's locale):

| file | what | done |
|---|---|---|
| `internal/setup/build/{Vendor,auditor}/aws/entrypoint.sh` lines 2-5 | Korean comments | translated; commands untouched; both copies byte-identical (`cmp`) and ASCII (`file`) |
| `internal/tee/request/Vendor/wire.go` lines 9-10 | `·` as a separator in a comment | replaced by `|`; the `auditor` and `user` copies changed the same way (role duplication) |
| `internal/cli/progress.go` lines 25-26, 79-80, 82-83, 87 | `✓`, `✗`, `·` | **left as they are.** They are program output (`Done` prints `✓`, `Fail` prints `✗`), not comments, and the comments next to them quote that output. The README documents `✓ matches`. Changing them is not a comment change |

Outside the EIF and left alone: `build.go`, `lint.go`, `verify.go` in `internal/setup/build/{Vendor,auditor}/aws` print
`→`, `…`, `·`, `✓`, `✗` from `att-vendor` and `att-auditor`; none of it is copied into the image.

Checked: the responder built with `-buildvcs=false -ldflags=-buildid=` is byte-identical before and after the `wire.go`
change (sha256 `187295fb…12a3` both), so that comment change does not change the code. The EIF change in this commit
comes from `entrypoint.sh`.

**Found while checking, not changed here:** a plain `go build -trimpath ./cmd/ra-responder` embeds the git commit
(`vcs.revision`), its time (`vcs.time`), whether the tree was modified (`vcs.modified`) and a module version derived from
them (`go version -m` shows `v0.0.0-20261111052755-e8ec754a88fd`, `+dirty` when modified). So two builds of identical
code from different commits, or from a dirty tree, give different responder bytes and therefore different Nitro
PCR2. Builds that use the one released `att-ra-responder` on both sides are not affected. Reported to the user; a fix
(for example `-buildvcs=false`) changes how the binaries are built and was not made in this commit.

## Decision 2026-11-11: `-buildvcs=false`, enforced

On the user's decision. Enforced in code, not only written down:

- `internal/setup/build/responder.go`: `ReadResponder` reads the `vcs` build setting into `ResponderInfo.VCS`;
  `CheckResponder` refuses a responder that has one, right after the `-trimpath` check, with "att-ra-responder was built
  with version control stamping: the git commit and commit time are baked into it, so the same source built from a
  different commit, or from a tree with uncommitted changes, gives different bytes and different measurements" and the
  rebuild command, which now reads `go build -trimpath -buildvcs=false`.
- `-buildvcs=false` right after `-trimpath` in every `go build` in the repository: `internal/setup/build/arch.go`
  (`BuildResponderForArch`), `e2e/0-Setup-local-upload.sh`, `e2e/arch-measurement-compare.sh`, `e2e/build-env-check.sh`,
  `e2e/toolchain-pin-check.sh`, `e2e/responder-check.sh` (also on its deliberate no-`-trimpath` build, so that build is
  refused only for `-trimpath`), and the build commands in `README.md`, `CONTRIBUTING.md`, `cmd/README.md` and
  `internal/tee/parse/snp/testdata/README.md`. The test helpers that build the `att-*` binaries without `-trimpath`
  (`cmd/e2e_helpers_test.go` and similar) were left as they are; their binaries are not layered into an image.
- Tests: a rule case for a stamped responder; the real-build test now requires `BuildResponderForArch` to produce no
  stamp, and builds one with `-trimpath -buildvcs=true` that must be refused naming the stamping (skipped when the source
  tree has no `.git` directory to stamp from).
- `CLAUDE.md` rule 3 is now "Always `-trimpath -buildvcs=false`".
- The measurement is written up in `docs/build/aws-nitro.md`. Measured in a local clone of this repository with
  go1.26.1, `GOOS=linux GOARCH=amd64 go build -trimpath ./cmd/ra-responder`:

  | build | sha256 |
  |---|---|
  | commit `72d62f9` | `c46d29198432e72d5cc37a33f6094c9a70d723a098659d8f89230a09ddf079a0` |
  | commit `e8ec754` (responder packages, `go.mod`, `go.sum` identical to `72d62f9`) | `c2e433f2cce9c3c36d3e1c673aa8dc99c38cc007812de5dac7ef187fdcfee44f` |
  | `e8ec754` plus one uncommitted line in `README.md` | `11802de5912dfbe8652074168b6f68301461d69d8a3d76988aff830a141c714f` |
  | `72d62f9`, and the dirty `e8ec754`, both with `-buildvcs=false` | `82aba5f6433c915e2b6580828ef2fa25af8cdc02c25498ba65b832e3020b9207` (identical) |

  A first attempt with `git worktree` checkouts gave five identical files: Go put no stamp in them at all (`go version -m`
  showed no `vcs` lines and module version `(devel)`). The worktree's `.git` is a file rather than a directory; that this
  is the cause is an inference. The clone, with a real `.git` directory, was stamped.

**Nitro PCR baseline: baseline after both.** The entrypoint translation (`a379a2c`) and this change both change what
goes into the EIF (the entrypoint bytes, and the responder bytes, which no longer carry the commit), so PCR0 and PCR2
change twice. Take the Nitro baseline only after both commits.

## Byte order, GRUB_FORCE_PARTUUID, cpio placement, SEV-SNP runtime measurement

Three research tasks were delegated to background agents (read-only, no cloud commands); their findings were checked
against the code, the fixtures or the source before being written down.

- **Byte order** (`docs/build/tdx.md` and `docs/build/sev-snp.md`, "Byte order").
  - No library reads a field in the wrong order.
  - Eight new tests pin the order against the recorded evidence: `internal/tee/parse/{tdx,snp}/endian_test.go`. Each
    refusal test was checked to fail for the intended reason:
    - POLICY bit 59 → "mbz range policy[0x1a:0x3f]"
    - SIGNER_INFO bit 26 → "mbz range data[0x48:0x4C]"
    - R → "ECDSA verification failure"
    - GUID in EFI order → "missing VCEK certificate"
  - Gaps in the specs:
    - AMD 56860 rev. 1.58 states little endian only for signatures and keys.
    - Intel's DCAP document types TDATTRIBUTES as a byte array without saying which byte holds bit 0. The fixture pins
      it: read LE only bit 28 is set; read BE a reserved bit would be.
    - Both libraries verify signatures over re-serialised fields, so a passing signature does not pin byte order.
- **`GRUB_FORCE_PARTUUID`** (`docs/build/tdx.md`). Set by Ubuntu's `livecd-rootfs` for non-generic kernels, to skip
  the initramfs for boot speed, with a fallback after a panic.
  - Migration concern: partly valid. The fixed PARTUUID goes stale if the root partition's GUID changes; live
    migration does not apply to these VMs.
  - Docker concern: not supported by what was found.
- **cpio placement** (`docs/build/tdx.md`, "Before or after the base initrd").
  - The kernel accepts an uncompressed cpio after a compressed segment only at a 4-byte-aligned offset (checked by me
    in `init/initramfs.c` on GitHub master: `if (*buf == '0' && !(this_header & 3))`).
  - Ubuntu's initrd is not padded to 4 bytes, and neither is `cpio.Join` (inferred).
  - **Important, inferred:** an appended archive at an unaligned offset is dropped with one log line while the base
    still boots, and GRUB still measures the combined file. So the 2026-11 SEV-SNP "booted" results, and a matching
    INITRD, do not prove the application was unpacked. Not yet tested; the checks to run are in the doc.
  - `cpio.Join` was not changed (padding would change the measured value).
- **SEV-SNP runtime measurement** (`docs/build/sev-snp.md`). SNP has no guest-extendable register (ABI rev. 1.58); the
  kernel's measurement-register interface is TDX-only (6.16); GCP's vTPM is hypervisor-managed and Google-signed. A
  hardware-rooted runtime register needs an SVSM vTPM, which on GCP appears only as a "preview" in a community blog.
- **Exact GCP images** recorded in both docs: `ubuntu-2404-noble-amd64-v20261105` for both instances; disks 64 GB
  (`pd-balanced` TDX, `pd-standard` SEV-SNP); Secure Boot off, vTPM on.

## anvil installed; every chain test ran for the first time

- On the user's instruction, `anvil.exe` alone was extracted from the Foundry v1.8.0 zip downloaded on 2026-11-10.
  Its sha256 still matched the release's (`e31241db…982e`). Installed to `%USERPROFILE%\.foundry\bin\anvil.exe`,
  where `cmd/register_e2e_test.go` looks. `anvil --version`: 1.8.0, commit `61ae26af36320d4fa1020f7db53785885e29eeb5`,
  built 2026-08-26.
- `go test -run TDX ./cmd/`: `TestCmd_E2E_TDX_ServeRequestParse` and **`TestRun_E2E_TDX_AuditorUser` pass**.
- `go test -v ./cmd/`: all 26 tests pass, none skipped. The Nitro chain tests that had been skipping for lack of anvil
  ran too (`TestRun_E2E_AuditorVendorUser`, `TestRegister_E2E_DeployAndRegister`, `TestLookup_E2E_FindAddressAndRead`,
  `TestCompare_E2E_PlainAndChainModes` and the rest).
- `README.md` ("Testing") records the anvil version; `docs/build/tdx.md` no longer says the chain test has not run.

## Other work since the last log entry

The plan, test matrix and slide changes were committed separately and are not repeated here:

- `docs/plan/test-matrix.md` filled with what code proves and what needs an instance (`bb3fe50`)
- the 2026-10-23 slide changes collected in `docs/plan/cvm-port.md` (`49858b3`)
- the AWS quota could not be read: the `aws` session had expired, and `aws login` needs a person

The Phase 2 design (`tee/request` for TDX and SEV-SNP) was approved by the user; implementation is paused until the
four items below are done.

## Four items, 2026-11-11 (no instance started for any of them)

1. **Option 2 on TDX** (`c221d4d`).
   - `docs/build/tdx.md` now describes it: the squashfs + dm-verity image goes inside the initrd, so RTMR2 covers it
     and `intel-tdx` needs no second format.
   - `docs/build/sev-snp.md` and `docs/build/ra-comparison.md` cross-reference it.
   - Phase 3.4 is still in "Not done yet".
   - The instruction pointed at a "Reproducibility" section of `sev-snp.md`; there is none, so the reference is to
     "Variables", which holds the squashfs and dm-verity settings.
2. **Roots of trust** (`d61c217`). `docs/build/ra-comparison.md` gained a "Roots of trust" section.
   - Re-checked myself:
     - MEASUREMENT is identical in both SEV-SNP recordings (`xxd -s 0x90 -l 48` on each report).
     - HOST_DATA (C0h), FAMILY_ID (10h), IMAGE_ID (20h) and ID_KEY_DIGEST (E0h) are zero in both.
     - The SHA-384 launch digest is updated during `SNP_LAUNCH_UPDATE` and finalized by `SNP_LAUNCH_FINISH`
       (56860 rev. 1.58 text).
   - **Correction:** Google's attestation overview does **not** contain "Hybrid hardware and software attested" or
     "Hardware attested" (checked twice). The section quotes what the page does say ("Root of trust for
     measurement: AMD Secure Processor + VM instance firmware", "Bootloader, kernel, and userspace measurements:
     Shielded VM vTPM", "Software root of trust (hypervisor dependent)", and for TDX "You can verify the Intel TDX
     module quote on its own to cover the entire boot chain").
   - An earlier paragraph in `sev-snp.md` attributed "software-attested instead of hardware" to that page. The phrase
     is not on it, and the paragraph was corrected.
   - The IETF draft supports only "runtime extendable measurement register" for RTMR0-3 (§3.3).
   - "Google Cloud does not offer an SVSM" is written as "Google's documentation offers none", with the June 2026
     community-blog "preview" mention kept.
   - RTMR index conventions were not written, as instructed.
3. **cpio alignment** (`a3bdff1`). **Not measured.** No base initrd from the TDX run or the 2026-11 SEV-SNP runs is
   on the local machine (searched the scratchpad and the folder of 2026-11 scripts). `docs/build/tdx.md` now lists what
   to measure, where and with which command, and what the hypothesis expects.
4. **App fixtures: prepared only, NOT RUN.**
   - Tools (`5282c9e`): `internal/tee/parse/tdx/testdata/record` (TDX quote recorder, standard library only) and
     `internal/setup/build/cpio/testdata/mkappinitrd` (dummy one-file application archive and `cpio.Join`). Both
     compile and vet for linux/amd64. `mkappinitrd` was run locally on a 10-byte fake base to check the order of
     `first` and `last`. Neither has run on a guest.
   - The SEV-SNP recorder now also builds inside the module.
   - Procedure (`8e42e38`): `docs/plan/app-fixture-recording.md`, two recordings per platform with the application
     changed by one byte, and why that makes T5 a permanent code test. `docs/plan/test-matrix.md` T5 points at it.
   - **No instance was started; no fixture was recorded.**

## App-fixture recording, all four cells (2026-11-11, the user present)

Instances, started on the user's instruction to record, stopped after reporting each one:

- `attestamp-snp-vendor` ran 07:00:39–08:18:20 UTC;
- `attestamp-tdx-vendor` ran 08:31:47–08:52:50 UTC.

A safety stop (`shutdown -h +240`) was set after every boot. Both instances are TERMINATED. `GRUB_FORCE_PARTUUID` is
now off on both; every other boot change was undone. On the SEV-SNP guest `tpm2-tools` was installed with apt.

**Taken from the 2026-11 scripts outside the repository** (read as reference only; nothing copied; their hard-coded
home paths, disk UUIDs, PARTUUIDs, kernel version and IP addresses were not brought in):

| script | what was taken | where it is now |
|---|---|---|
| `option2-dm-verity/mkverity.sh` | the `mksquashfs` arguments (`-noappend -no-xattrs -all-root -mkfs-time 0 -all-time 0 -noI -noD -noF -noX -no-fragments -no-duplicates -processors 1 -b 4096`), the 4096-byte padding, `veritysetup format` with its fixed `--salt` and `--uuid` | `internal/setup/build/cpio/testdata/make-verity-image.sh` (adds `umask 022` and explicit modes) |
| `option2-dm-verity/verity_hook.sh` | which binaries and modules the verifying initrd needs (`veritysetup`, `tpm2_pcrextend`, `sha256sum`, the TCTI library; `dm_verity loop squashfs tpm*`) | `internal/tee/parse/snp/testdata/record/initramfs-hook.sh` |
| `option2-dm-verity/verity_boot.sh` | the local-bottom logic: root hash from the command line, `veritysetup verify` then `open`, mount on `${rootmnt}/opt/app-verity`, extend PCR12 (SHA-256) with SHA-256 of `attestamp-verity-<status>:<root hash>`, status in `/run` | `internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh` (same PCR12 value) |
| `option2-dm-verity/gcp_setup_option2.sh` | the order: image to `/var/lib/attestamp`, hook and script into `/etc/initramfs-tools`, `mkinitramfs -o` a separate verifying initrd | `docs/plan/app-fixture-recording.md` |
| `option2-dm-verity/gcp_boot_option2.sh` | a GRUB entry with `attestamp.roothash=` appended (its hard-coded entry was replaced by one made from the stock entry) | `internal/setup/build/cpio/testdata/make-grub-entry.sh` |
| `auditor/auditor_exp_verity.py` | the expected-PCR12 formula, used to check both recordings | `app_fixture_test.go`, the `*-boot.txt` files |
| `option2-dm-verity/verity_tamper.sh` | nothing; the tamper case was not recorded | — |

**Findings:**

- **SEV-SNP Option 1 as appended in 2026-11 loses the application.** Base initrd 50,100,283 bytes (mod 4 = 3); the
  unpadded append gave `Initramfs unpacking failed: invalid magic at start of compressed archive` and no marker.
  INITRD still changed. Padded to 4 bytes it worked. `cpio.Join(AppLast)` now pads (`3c57d4c`). The slide claim is
  corrected (`c4b77cf`).
- **The marker had to move from `/run` to `/dev`** (`56ec365`): `/init` sources `conf/conf.d` before mounting `/run`.
- **TDX:** only RTMR2 moved in four recordings; the assumed mapping holds for RTMR2. `AppFirst` unpacked on all four
  boots. TDX with `AppLast` was not tested.
- **SEV-SNP Option 2:** first real `amd-sev-snp-verity` evidence; PCR12 matches the formula.
- The dm-verity image was byte-identical on both guests (T1).
- The two guests have the same distribution base initrd, byte for byte.

**Committed:**

- SEV-SNP: six recordings, with boot logs (`eb9a7a2`). Both Option 1 pairs are kept, on the user's decision.
- TDX: four recordings (`e67f0a3`).
- Tests: parser tests per pair; the mixed-recording binding test (`e3f1150`); the T5 chain test on anvil for all four
  cells (`bfb0313`).
- Docs: measured results replace the assumptions (`c625e26`).
- Not done: TDX Option 2 image mounting at boot, the SEV-SNP tamper case, and Phase 2 (approved, paused).

## Phase 2 implemented; runtime immutability designed (2026-11-11, evening)

- The slides' "RTMR[2] alone" was settled with the user as a contrast (one value moves on TDX), not a registration
  rule; `intel-tdx` keeps four names (`docs/build/tdx.md`, `c587015`).
- **Phase 2 code** (`60de6b0`):
  - `internal/tee/request/{tdx,snp}` plus thin adapters for Vendor and auditor;
  - `att-ra-responder --extractor tdx | snp-initrd | snp-verity`;
  - `--listen` still defaults to `127.0.0.1`.
  - go-configfs-tsm v0.3.3 and go-tpm v0.9.8 moved from indirect to direct dependencies. They were already in the
    module graph; nothing new was downloaded.
- **Tests, with no hardware.** go-configfs-tsm's `faketsm` returns each recorded quote or report only for the
  REPORTDATA it was made for, and the output verifies with the parsers. The SEV-SNP request code encodes the container
  itself rather than importing `parse/snp`, and a test pins it byte-for-byte to `parse/snp.Encode`. This keeps verifier
  code out of the TEE binary.
- **Responder size.** The Linux `att-ra-responder` grew from 4.7 MB to 12.5 MB: go-tpm-tools' client links the
  go-sev-guest and go-tdx-guest clients.
- **Not done:** Phase 2.3, the live run on a guest.
- **Nitro baseline.** The responder is in the Nitro EIF too, so its PCR2 changes again. Take the baseline after
  `a379a2c`, `e6ef9b4` and `60de6b0`.
- **Docs:**
  - `docs/build/runtime-immutability.md` (new): layer 1 confirmed (kernel dm-verity documentation quoted, our
    measurement), layer 2 not built, layer 3 reasoned and not tested; Nitro for comparison.
  - `docs/plan/cvm-port.md`: why Phase 2 comes first (layer 2 removes `sshd`), the firewall narrowing, and Phase 5
    (lock the guest, with the tamper case).
  - README, STRUCTURE, tdx.md, sev-snp.md updated.
