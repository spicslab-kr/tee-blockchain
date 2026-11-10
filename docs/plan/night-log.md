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
