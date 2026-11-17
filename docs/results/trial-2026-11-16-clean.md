# Trial 2026-11-16: Nitro, TDX and SEV-SNP end to end, and whether they agree

- **How far:** all three to the end. AWS Nitro Enclaves: e2e flow 1 (build → launch → audit → verify, then swap →
  refuse). Intel TDX: `docs/build/tdx.md` steps 1–13, **Option 1 and Option 2**. AMD SEV-SNP: `docs/build/sev-snp.md`
  steps 1–13, **Option 1 (`amd-sev-snp-initrd`) and Option 2 (`amd-sev-snp-verity`)**. Then, on all three, the same
  one-byte change, the same empty-registry check and the same `e2e/replay-check.sh`.
- **Same results?** **Yes, every verdict.** On all three (and both CVM options) a registered value gives `verify`
  exit 0 with `✓ matches`; one byte changed in the application, and an empty registry, each give exit 1 with the same
  two lines; a replayed document gives exit 6. The User's commands are the same with only `--format` different.
  **Not** the same: everything the Vendor and Auditor type (commands, flags, number of steps, reboots, who rebuilds),
  and SEV-SNP evidence from a guest booted without our initrd is refused (exit 7) where TDX's is accepted.
- **Biggest blocker:** `bash e2e/0-Setup-local-upload.sh` exits 1 **with no output at all** unless all five of
  `VENDOR_HOST AUDITOR_HOST ARM64_HOST AUDITOR_ADDR VENDOR_ADDR` are declared in the environment: `e2e/env.sh` line 14
  runs `declare -p` on all five, which fails under the script's `set -euo pipefail` (F3).

Run on 2026-11-15/16 (UTC 18:40–21:10) at commit `6a92a1f`, following the documents alone,
without the earlier sessions' knowledge, in a WSL Ubuntu 24.04.5 on the development machine, with the
repository as it was. **No tracked file was changed**; this file is the only one written into the repository. The
documented steps themselves wrote gitignored files (`e2e-work/`, `pem2.pem`, `auditor.key`, `deployer.key`, `att-*`)
and some files that are **not** gitignored: `linux/`, `quote.bin*`, `evidence.cbor*`, `measurements.json` (F14). They
are still there.

Marks: **[outside]** knowledge or action not in the documents; **[stuck]** cannot go on as written; **[finding]** a
result the documents do not predict. F-numbers are findings, D-numbers differences between platforms.

## 1. The comparison table

Every cell was run; nothing is inferred. "JSONL" means a line in the results contract of `docs/status/test-matrix.md`.
All `verify` calls ran on my machine (WSL) against the instance's public address and a local anvil 1.8.0.

| check | Nitro (`aws-nitro`) | TDX (`intel-tdx`) | SEV-SNP (`amd-sev-snp-initrd`) |
|---|---|---|---|
| **normal flow: `verify` passes** | **yes**, exit 0, `✓ matches  nginx 1.31.6 (amd64) by 0x7099…` (`flow1/3-User-verify.sh`, JSONL). Again with `--format aws-nitro` written out: exit 0 | **yes**, exit 0, `✓ matches  hello 1.0.0 (amd64) by 0x7099…` (step 12). `in-guest-verify.sh`: 11/11 PASS | **yes**, exit 0, `✓ matches  hello 1.0.0 (amd64) by 0x7099…` (step 12). `in-guest-verify.sh`: 11/11 PASS |
| **app changed → refused** | **yes**, exit 1, `✗ nobody`, twice: (a) the script's swap to a **whole different image** (`flow1/4,5`, JSONL `T5`), PCR0 `2aacdf41…` → `9f1ebbec…`; (b) **one byte** changed in the nginx app folder (`# vim:sw=2` → `sw=4` in `30-tune-worker-processes.sh`), rebuilt, relaunched **[outside]**: PCR0 → `c0263871…`, PCR2 → `8aeabedf…`, PCR1 unchanged | **yes**, exit 1, `✗ nobody`. **One byte** in `app/hello.txt` (`p`→`P`), steps 6–9 again (`tdx.md` step 12 suggests it): only RTMR2 moved, `0f36ec64…` → `f36c5202…` | **yes**, exit 1, `✗ nobody`. One byte in `app/hello.txt`, steps 6–9 again: only INITRD moved, `11cd710e…` → `db9c9779…` |
| **unregistered → refused** | **yes**, exit 1, `✗ nobody`: the genuine enclave against a freshly deployed empty registry **[outside]** | **yes**, exit 1, the same test **[outside]** | **yes**, exit 1, the same test **[outside]** |
| **same command, only `--format` differs?** | User side **yes** (`verify`, `ra-request`, `ra-parse`, `replay-check.sh`) once `--format aws-nitro` is written out; the scripts and README leave it out because it is the default. Vendor/Auditor side **no** (D4–D9) | User side **yes**. Vendor side **no** | User side **yes**. Vendor side **no**; and TDX and SEV-SNP differ from each other too (D4, D5, D8) |
| **exit code and output in the same shape?** | **yes** for `verify` (0 / 1, the same lines word for word) and `ra-parse` replay (6). 3 values, 4,503–4,504-byte document, `ra-parse` shows `module` and `timestamp` | **yes**. 4 values, 8,000 bytes, `module` empty, `timestamp none in the document` | **yes** for the exit codes and the verdict lines, but the values are printed in **another order** in `verify`/`audit` than in `ra-parse`/`launch --check`, and `PCR4` is misaligned (D12). 5 values, 68,353 bytes |
| *extra:* **replay refused (T9), same script** | **yes**, `replay-check.sh`, exit 6, all 4 checks pass (JSONL) | **yes**, `replay-check.sh` with `TEE_ADDR=… RESULTS_PLATFORM=intel-tdx`, all pass (JSONL) | **yes** on our boot, all pass (JSONL). On the stock boot the two fresh documents were refused with **exit 7** ("GRUB loaded no initrd"), the replay still with exit 6 (D18) |

### Option 2 (`docs/build/*.md` "Which option")

| check | TDX `intel-tdx`, `--option 2` | SEV-SNP `amd-sev-snp-verity` |
|---|---|---|
| normal flow: `verify` passes | **yes**, exit 0, `✓ matches  hello 1.0.0 (amd64)`; `/run/attestamp-status` `ok 5dff1f1d…`, `hello.txt` under `/opt/app-verity/opt/app`, `dmsetup` `verity V` | **yes**, exit 0; `/run/attestamp-status` `ok`, `/run/attestamp-log` empty, `hello.txt` mounted; PCR12 in the evidence = the PCR12 `build` predicted |
| app changed (one byte) → refused | **yes**, exit 1, `✗ nobody`; root hash `5dff1f1d…` → `15e18e5e…`, RTMR2 `ca9a7abc…` → `de2d3b8b…` | **yes**, exit 1; root hash the same new `15e18e5e…`, CMDLINE `d12176cc…` → `ac98ccae…`, PCR12 `33784785…` → `35850d87…`, INITRD unchanged |
| unregistered → refused | **yes**, exit 1 | **yes**, exit 1 |
| same command, only `--format`? | User side yes, the same `intel-tdx` as Option 1 (the option does not show in the format) | User side yes, with `amd-sev-snp-verity` (the option **is** the format) |
| output shape | as Option 1 | 6 values (Option 1's five and PCR12), 68,605 bytes |

### Every difference found

| # | where | Nitro | TDX | SEV-SNP |
|---|---|---|---|---|
| D1 | what the procedure is | e2e **scripts** (`flow1/1..5`) run from my machine over SSH | 13 **typed steps** (`tdx.md`) | 13 typed steps (`sev-snp.md`) |
| D2 | machines | **2** instances (Vendor, Auditor) | **1** guest | 1 guest |
| D3 | who produces the registered value | the **Auditor rebuilds** on its own instance and audits its own enclave; the Vendor then launches the Auditor's EIF | the Auditor registers **the Vendor guest's own** `out/measurements.json`; no independent rebuild ("The Auditor's own guest" step 5 has never been run) | the same as TDX |
| D4 | build | `att-auditor build --app-dir <dir> --out-dir out` (no `--verify` in the script) | `att-vendor build --platform intel-tdx --app-dir app --base-initrd base-initrd.img --option 1 --verify` | `att-vendor build --platform amd-sev-snp-initrd --app-dir app --base-initrd base-initrd.img --verify` — **no `--option`**: the option is the format string |
| D5 | launch | `launch --eif out/alpine-slim.eif` (Vendor: `+ --build-measurements …`); starts the enclave at once and self-checks | `sudo att-vendor launch --platform intel-tdx --initrd out/initrd-app.img --reboot`, then after the reboot `launch --platform intel-tdx --check` | `sudo att-vendor launch --platform amd-sev-snp-initrd --build-value out/build-value.json --reboot`, then `--check`. **`--build-value` here, `--initrd` on TDX** |
| D6 | audit | `att-auditor audit --config build-config.json --tee-addr … --build-measurements … --rpc … --registry … --signing-key-file …`; format by default; service, version, arch from `--config` | `… --format intel-tdx … --service hello --version 1.0.0 --arch amd64` (no `--config`) | as TDX, `--format amd-sev-snp-initrd` |
| D7 | verify, as documented | `att-user verify --tee-addr <ip>:5000 --rpc … --registry …` (**no `--format`**) | `… --format intel-tdx …` | `… --format amd-sev-snp-initrd …` |
| D8 | responder | layered into the EIF by `build`; nothing to start | started by hand after every boot, `ra-serve --extractor tdx` | `ra-serve --extractor snp-initrd` / `snp-verity`: the extractor name is not the format name |
| D9 | reboots | none | 2 on the normal path (launch, undo) and 1 more for the "app changed" test | the same |
| D10 | "app changed" test in the docs | a different image built by `pack` (`flow1/4`) | one byte, by hand (`tdx.md` step 12) | the same as TDX |
| D11 | values registered | PCR0, PCR1, PCR2 | MRTD, RTMR0, RTMR1, RTMR2 (both options) | 5 (Option 1), 6 with PCR12 (Option 2) |
| D12 | value order printed | the same everywhere | the same everywhere | `ra-parse`, `launch --check`: MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD; `audit`, `verify`: sorted by name (CMDLINE, INITRD, KERNEL, MEASUREMENT, PCR4), with `PCR4  5b65…` aligned differently from the other four |
| D13 | `ra-parse` header | `module <enclave id>`, `timestamp <time>` | `module` empty, `timestamp none in the document` | the same as TDX |
| D14 | results (JSONL) | every flow script writes `e2e-work/results/aws-nitro.jsonl` | the getting-started steps write **none**; `in-guest-verify.sh` writes lines with `"platform":""` (F10); `replay-check.sh` with `RESULTS_PLATFORM=intel-tdx` writes correct ones | the same as TDX |
| D15 | keys | `NET=local` writes anvil's accounts to `e2e-work/anvil-keys/` | step 11 writes `auditor.key` and `deployer.key` into the repository root | the same as TDX |
| D16 | stopping | `aws-nitro.sh stop` stops only its `TAG` | `gcp-cvm.sh tdx stop` stops **every** VM labelled `platform=tdx`; `PREFIX` does not narrow it (F12) | the same as TDX |
| D17 | wording | "enclave" | "enclave" (it is a VM) | "enclave" (it is a VM) |
| D18 | evidence from a guest not booted into our initrd | n/a | **accepted**: `ra-parse` exit 0 on the stock initrdless boot, and those values could be registered | **refused**: exit 7, `GRUB loaded no initrd, so the application archive was never measured (an initrdless boot …)`, after `signature ok` and `nonce ok` |
| D19 | `replay-check.sh` | documented in `e2e/README.md` | works with `TEE_ADDR=<ip>:5000 RESULTS_PLATFORM=intel-tdx`, which only the script's header says | the same, `RESULTS_PLATFORM=amd-sev-snp-initrd` |

### Built from the JSONL files

58 lines from: `e2e-work/results/aws-nitro.jsonl`, `intel-tdx.jsonl` and `amd-sev-snp-initrd.jsonl` (written by the flow
scripts and `replay-check.sh`), the two `results.jsonl` that `in-guest-verify.sh` wrote on the guests (platform filled
in by hand, F10), and the lines I wrote in the same contract for the checks no script makes (`"script":"manual …"`).
**56 have `result` = `expected`. The 2 that do not** are SEV-SNP's `own-nonce-a/b` from the stock boot (D18); the same
checks on our boot pass.

Committed: the 20 lines the e2e scripts wrote on this machine (`flow1/*` and `replay-check.sh`), unchanged, in
`docs/results/trial-2026-11-16-clean/<platform>.jsonl`. The other 38 (the two guests' `in-guest-verify.sh` files and the
manual lines) are not in the repository; the table below is their record here.

| platform | test | check | result | lines | written by |
|---|---|---|---|---|---|
| aws-nitro | T4 | auditor-audit-registers, vendor-checks-own-enclave, user-verify | accepted | 3 | `flow1/1`, `flow1/2`, `flow1/3` |
| aws-nitro | T5 | user-verify-after-swap | rejected | 1 | `flow1/5` |
| aws-nitro | T9 | own-nonce-a/b, two-documents-differ, replay-refused | accepted, different, refused | 4 | `replay-check.sh` |
| aws-nitro | T4 | user-verify-explicit-format, user-verify-before-one-byte-change | accepted | 2 | manual |
| aws-nitro | T5 | user-verify-after-one-byte-change | rejected | 1 | manual |
| aws-nitro | – | user-verify-empty-registry | rejected | 1 | manual |
| intel-tdx ¹ | –, T9, T4 | request ×2, document verifies ×2, nonce checked, two differ, replay refused, registry deployed, audit, verify exit, verify matches | all as expected | 11 | `in-guest-verify.sh` |
| intel-tdx | T9 | own-nonce-a/b, two-documents-differ, replay-refused | as expected | 4 | `replay-check.sh` |
| intel-tdx | T4 | user-verify (Option 1, Option 2) | accepted | 2 | manual, `tdx.md` step 12 |
| intel-tdx | T5 | user-verify-after-one-byte-change (Option 1, Option 2) | rejected | 2 | manual, `tdx.md` step 12 |
| intel-tdx | – | user-verify-empty-registry (Option 1, Option 2) | rejected | 2 | manual |
| amd-sev-snp-initrd ¹ | (the same 11 checks) | | all as expected | 11 | `in-guest-verify.sh` |
| amd-sev-snp-initrd | T9 | own-nonce-a/b on the **stock** boot | **rejected** (expected accepted) | 2 | `replay-check.sh` |
| amd-sev-snp-initrd | T9 | two-documents-differ, replay-refused on the stock boot; all four on our boot | as expected | 6 | `replay-check.sh` |
| amd-sev-snp-initrd | T4 / T5 / – | user-verify / after-one-byte-change / empty-registry | accepted / rejected / rejected | 3 | manual, `sev-snp.md` step 12 |
| amd-sev-snp-verity | T4 / T5 / – | user-verify / after-one-byte-change / empty-registry | accepted / rejected / rejected | 3 | manual, `sev-snp.md` step 12 |

¹ `platform` is empty in the file (F10); filled in from the guest the file came from.

## 2. Where the documents were not enough

### Steps that failed as written, and what made them pass

| step | what was wrong | how I got past it | what the documents should have said |
|---|---|---|---|
| Nitro: create (`e2e/cloud/README.md` "Order" 1) | `bash e2e/cloud/aws-nitro.sh create` → exit 1, `InvalidKeyPair.Duplicate … could not create key pair attestamp-e2e`. The account already has that key pair (and two stopped `attestamp-e2e` instances); a fresh clone has no `pem2.pem` | **[outside]** read the script for its variables; `KEY_NAME=trial1116-nitro SG_NAME=trial1116-nitro TAG=trial1116-nitro bash e2e/cloud/aws-nitro.sh create` | that `KEY_NAME`, `SG_NAME` and `TAG` exist, and what to do when the key pair exists but its `.pem` is not on this machine |
| (same) | **[finding] F1** the failed create **left an empty `pem2.pem`** (the redirect `> "$PEM_OUT"` creates it before `aws` fails; `chmod 400` never runs). A rerun would print "pem2.pem already exists, reusing it" and never create the key | **[outside]** found it with `git status --ignored` and removed it (the settings deny reading `pem2.pem`, so I could not look inside) | write to a temporary file and move it only on success |
| (same) | **[finding] F2** the run ends with exit **126**: `exec "$0" status` → `Permission denied`; the cloud scripts are mode `100644` in git and are run as `bash e2e/cloud/…`. The instances **were** created | ran `TAG=… bash e2e/cloud/aws-nitro.sh status` myself | `exec bash "$0" status`, or mode 755. `gcp-cvm.sh` line 121 has the same: every GCP create of this trial also ended with 126 |
| Nitro: `0-Setup-local-upload.sh` | **[stuck] [finding] F3** exit 1 **with no output**, addresses given on the command line as `e2e/README.md` documents. `e2e/env.sh` line 14, `env_given=$(declare -p VENDOR_HOST AUDITOR_HOST ARM64_HOST AUDITOR_ADDR VENDOR_ADDR 2>/dev/null \| grep …)`, fails when any of the five is undeclared, and the upload script's `set -euo pipefail` ends it there, before the EXIT trap is set. Reproduced alone: `bash -c 'set -euo pipefail; . e2e/env.sh; echo REACHED'` → nothing, exit 1; with the five declared → `REACHED`. `env.local.sh` does not help: it is read after that line. Since `425a033` | **[outside]** declared the missing three as empty: `ARM64_HOST= AUDITOR_ADDR= VENDOR_ADDR=` before the command | a fix in `env.sh` (`… \|\| true`); until then, that workaround. The flow scripts use `set -uo pipefail` without `-e` and are not affected |
| Nitro: flow 1 step 4 | try 1: exit 255, only `Connection closed by <ip> port 22`; `net_retry` did not retry although its pattern lists that message. `ssh` right after worked | ran it again | — (not reproduced) |
| Nitro: flow 1 step 4 | try 2: **[finding] F4** exit 1, `Cannot connect to the Docker daemon`. In flow 1 the Vendor never runs `build`, so docker (pulled in as a dependency, `docker-25.0.18`) is installed but `inactive` and `disabled`; step 4 calls `sudo docker build` itself. (On the Auditor, which ran `build`, docker was `enabled` and came back after a stop/start) | **[outside]** `sudo systemctl start docker` on the Vendor; try 3 passed | step 4 should start docker (as `build` does), or say it needs it |
| Nitro: anvil (`e2e/README.md` "Running without a public testnet" 1) | the README only links `https://getfoundry.sh`; the commands are in `tdx.md` step 1. Running the downloaded `foundryup` installer was **refused by my own tool-permission check** (not a fault of the documents) | **[outside]** `foundry_v1.8.0_linux_amd64.tar.gz` from the GitHub release, checked against the release's `foundry_v1.8.0_linux_amd64.sha256` and `toolchain.md`'s `8c8560de…ac57` (equal), unpacked into `~/.foundry/bin` | the release-archive route beside `foundryup`, with the checksum file's exact name (my first guess, `…tar.gz.sha256`, is a 404) |
| TDX/SNP step 3: `gcp-cvm.sh <plat> create vendor` | **[stuck]** `attestamp-tdx-vendor already exists, skipping`, then exit 126 (F2); no VM made. The project already had `attestamp-{tdx,snp}-{vendor,auditor}` (TERMINATED). Ran this only for TDX; for SNP the outcome was clear from it | **[outside]** `PREFIX=trial1116-tdx …` and `PREFIX=trial1116-snp …` (`PREFIX=…` is named only in passing under "Names") | that an existing name is silently reused, and how to choose another |
| TDX/SNP step 3: first `gcloud compute scp` | **[finding] F5** `gcloud` had no SSH key in this WSL: it generated `~/.ssh/google_compute_engine` and ran **"Updating project ssh metadata"**, a project-wide change that every VM in the project, the existing ones included, picks up | let it | one line saying this happens on first use |
| TDX/SNP step 7: knowing the new boot is up | **[finding] F6** `get-serial-port-output … \| tail -5` cannot tell the old boot's `login:` line from the new one, and the command returns only the newest part of the log, so counting `login:` lines also failed: my first wait gave a false positive, the second never ended | **[outside]** compared `/proc/sys/kernel/random/boot_id` before and after, over `gcloud compute ssh`. The new boot answered 55–166 s after the reboot command | the `boot_id` comparison; or only the doc's other option, `uptime` + `/proc/cmdline`, which works |
| TDX/SNP step 9 | the line ends in `&`; sent through `gcloud compute ssh --command`, a background process holding stdin can keep the session open | **[outside]** added `< /dev/null` | — (not needed when typing in a terminal) |
| TDX/SNP step 13: stop | **[finding] F12** `bash e2e/cloud/gcp-cvm.sh tdx stop` selects every VM labelled `project=attestamp, platform=tdx`, so it would also act on the existing `attestamp-tdx-*`; `PREFIX` does not narrow `stop`, `start` or `delete` | **[outside]** `gcloud compute instances stop trial1116-tdx-vendor --zone us-central1-a` (and the same for SNP) | `stop`/`delete` limited to `PREFIX`, or the one-VM `gcloud` line in step 13 |
| `e2e/cvm/in-guest-verify.sh` (an extra, for JSONL) | **[finding] F10** every result line printed `sed: -e expression #1, char 8: unterminated 's' command` twice, and **every JSONL line has `"platform":""` and `"reason":""`**. Line 28's `json_str` has `sed -e 's/\/\\/g'`, which is broken (`env.sh` has `'s/\\/\\\\/g'`). The 11 checks themselves passed on both guests | filled in the platform by hand | fix line 28; the test matrix needs `platform` to put a line in its column |

### Things I did that no document told me to, including the ones done without thinking

| step | what I did | why |
|---|---|---|
| 0 | Ran `bash e2e/0-Setup-local-install.sh` in the existing checkout; did not `git clone` the public URL | the brief: this repository |
| 0 | `sudo -n true` said a password is required, yet the install script's `sudo apt-get …` and `sudo tar …` went through. It upgraded `libc6`, installed `build-essential` and Go 1.26.1 into `/usr/local/go`, and appended `PATH` to `~/.bashrc`. Not investigated | — |
| 0 | Every `go` call prefixed with `PATH=/usr/local/go/bin:$PATH`; my shell does not read `~/.bashrc` (`tdx.md` step 1 says so for scripts, `e2e/README.md` does not) | `go` was not found otherwise |
| 0 | Did **not** install a Linux `gcloud`. `command -v gcloud` prints `/mnt/c/Program Files (x86)/Google/Cloud SDK/…/bin/gcloud`, the Windows install folder, and it runs under WSL (Google Cloud SDK 592.0.0, config in `~/.config/gcloud`). `tdx.md` step 1 says to install the Linux one and check for `/usr/bin/gcloud`; that check fails here, yet everything worked, only slowly (`gcp-cvm.sh status` over 2 minutes) | already signed in, as the brief said |
| 0 | `aws` was already installed (`aws-cli/2.40.3`, Linux) and signed in | — |
| Nitro | Gave the instance addresses on the command line every time, never created `e2e/env.local.sh` (`e2e/README.md` allows both) | the brief: write only this file |
| Nitro | `NET=local` with anvil 1.8.0 | no funded Sepolia keys; documented option |
| Nitro | Ran every script with `< /dev/null` | non-interactive; the EXIT trap only waits for a key on a terminal |
| Nitro | `sudo shutdown -h +360` on both instances right after creating them (`+120` when restarting the Auditor later) | cost safety; the Nitro docs have no safety stop (TDX/SNP have `+240`) |
| Nitro | Logged in with `ssh -i pem2.pem ec2-user@<ip>` to diagnose step 4 | — |
| all | **Unregistered check**: deployed an empty registry with `att-auditor registry-deploy` and ran `verify` against it | no script or step does it |
| Nitro | `att-user verify --format aws-nitro …` with the format written out | to compare the User command |
| Nitro | **One-byte check**: restarted only the Auditor instance, relaunched its `out/alpine-slim.eif`, audited it into a new registry with the flow's `e2e-work/auditor-build-config.json`, verified (exit 0), then copied the app folder to `src1b/mainline/alpine-slim` (same folder name), changed one byte with `sed`, `att-auditor build`, terminated the enclave, launched the new EIF, verified (exit 1) | to test Nitro the way the CVM docs test TDX and SEV-SNP |
| all | `replay-check.sh` against TDX and SEV-SNP with `TEE_ADDR`, `RESULTS_PLATFORM` (and `NET=local` so the `chain` field matches the other lines) | only the script's header says it works for another TEE |
| SNP | After the stock-boot replay run refused the documents (D18), installed the Option 1 boot again (`build`, `launch --reboot`, `--check`), re-ran `replay-check.sh`, then step 13 again | to have a fair T9 for SEV-SNP |
| TDX/SNP | Typed the guest steps as `gcloud compute ssh <vm> --command '…'`, several of a step's lines joined with `&&` | no interactive terminal |
| TDX/SNP | `e2e/cvm/in-guest-verify.sh` on each guest after step 8 (copied `att-auditor`, `att-user`, `anvil`, the script; several files in one `gcloud compute scp`, although the docs say one file per command) | to get script-written JSONL |
| TDX/SNP | A fresh anvil before each platform's step 11 in Option 1; in Option 2 I kept one anvil and deployed new registries (addresses not `0x5FbDB…`) | Option 1: so "on a fresh anvil it is `0x5FbDB…`" holds |
| TDX/SNP | "One byte changed" = `p` → `P` at the end of `hello from attestamp` | the docs do not say which byte |
| TDX/SNP | Option 2 step 4 a second time on the same guest: skipped `cp -p … /root/40-force-partuuid.cfg.orig` (the `.orig` was there and the live file had been restored to it); `apt-get` with `DEBIAN_FRONTEND=noninteractive -qq` | the original must not be overwritten by a changed copy; no terminal |
| TDX/SNP | `sudo shutdown -h +240` again after every reboot | documented, kept |
| all | Checked that the firewall rule `attestamp-ra` allows this machine's current `/32` (it did) | `tdx.md` step 10 mentions it |
| all | `go test ./...` once locally: exit 0, 45 packages `ok` | baseline |

### Other findings

- **F7** — `e2e/README.md` "Running without a public testnet" step 1 links Foundry without a command; the command is
  only in `tdx.md` step 1.
- **F9** — The Nitro AMI the script picks is the newest Amazon Linux 2023 (`ami-0f71b90bee7050a75` on 2026-11-15);
  docker and nitro-cli come from `dnf` unpinned. This run: `docker-25.0.18-1.amzn2023.0.3`,
  `aws-nitro-enclaves-cli-1.5.0`, the same as the 2026-11-12 baseline; git `2.50.1-1.amzn2023.0.2` (baseline `…0.1`).
- **F11** — The project and the account already had VMs and instances under the default names. Every create script
  collides with or silently reuses them, so a second person in the same project cannot follow the docs without
  `PREFIX`/`TAG`.
- **F13** — A stopped GCP VM can get a new external IP when started again (SEV-SNP did twice, TDX once); EC2 too. The
  docs say so for EC2 (`e2e/README.md`), not for the GCP guests.
- **F14** — `linux/` (where the README and step 2 put the guest binaries), `quote.bin`, `quote.bin.nonce`,
  `quote.bin.measurements.json`, `evidence.cbor*` and `measurements.json` (steps 10–11) land in the repository root and
  are **not** in `.gitignore`; `git add .` would take them.
- **F15** — A SEV-SNP guest on the stock boot answers with evidence that fails `ra-parse` (exit 7, D18). The docs say
  Option 1 needs our initrd, but not that the evidence is unusable until then; TDX evidence from the same state parses.

## 3. Instances I created

| name | cloud | type | made from | state now |
|---|---|---|---|---|
| EC2 tagged `Project=trial1116-nitro, Role=vendor` (no Name tag) | AWS eu-north-1 | m6i.xlarge, Nitro Enclaves on | the newest AL2023 x86_64 AMI of 2026-11-15 | **stopped** |
| EC2 tagged `Project=trial1116-nitro, Role=auditor` | AWS eu-north-1 | m6i.xlarge | the same | **stopped** (started once more for the extra checks, stopped again) |
| key pair `trial1116-nitro`, security group `trial1116-nitro` (22 and 5000 from `<my-ip>/32`) | AWS eu-north-1 | — | — | exist; no hourly cost. *(Both deleted 2026-11-18.)* |
| `trial1116-tdx-vendor` | GCP `<project-id>`, us-central1-a | c3-standard-4, TDX | `ubuntu-2404-noble-amd64-v20261105` | **stopped** (TERMINATED); put back to the stock boot |
| `trial1116-snp-vendor` | GCP `<project-id>`, us-central1-a | n2d-standard-2, SEV-SNP | `ubuntu-2404-noble-amd64-v20261105` | **stopped** (TERMINATED); put back to the stock boot |

Nothing deleted; the disks still cost a little. The existing `attestamp-e2e` instances and the
`attestamp-{tdx,snp}-{vendor,auditor}` VMs were only listed and `describe`d: not started, stopped or changed (the
project's SSH metadata did get a new key, F5). The firewall rule `attestamp-ra` was used as it was. On the two GCP
guests, the home folder still holds `base-initrd*.img`, `app`, `out`, the binaries and `anvil`; `/var/lib/attestamp` and
`/opt/app-verity` are gone.

## 4. Skipped

| what | why |
|---|---|
| Nitro flow 2 (`flow2-vendor-self-registers/1..5`, T6) | breadth first; the brief named flow 1's path |
| Nitro `debug-mode-check.sh`, `arch-*`, `toolchain-pin-check.sh`, `responder-check.sh`, `build-env-check.sh` | the same |
| `create-arm64` | optional, another billable instance |
| "The Auditor's own guest" on TDX and SEV-SNP | needs a second guest; so on the CVMs nobody rebuilt independently (D3) |
| T10 (two SEV-SNP VMs combined) | needs a second guest |
| Sepolia / mainnet | no funded keys; `NET=local` everywhere |
| anything needing a browser | none came up |

## 5. Values from this run

### Binaries (go1.26.1 linux/amd64, `CGO_ENABLED=0 -trimpath -buildvcs=false`)

| file | sha256 |
|---|---|
| `att-ra-responder` | `5573b5381ac72b1812263b886d04b667854bcb60c5d84573e1fcc34f8124d668` (= the documented cross-build value; the same on both Nitro instances) |
| `att-vendor` | `60a4ac31afe161819a8b58b548ae6aeacd1523fd0f299c198cff92be8680c870` |
| `att-auditor` | `33afa72c5129d4312f9c65cf8f8c5f77ce7b1406241f98db376bf220f4c3a3bb` |
| `att-user` | `37465a08d709b5888c4cc72bcfe351e33847cdcb9aad09242ff1dc97a208775c` |
| anvil | 1.8.0, commit `61ae26af`; archive `8c8560de380d58d1ee145934427887b107182367600a3c33aa71f16f2ce7ac57` |

### Nitro (nginx `mainline/alpine-slim` at `ef5a25a6…`)

| what | value |
|---|---|
| PCR0 | `2aacdf410ab98ea95138b9c99e48eb2436a0d2e6d9e7fc0f4b8d7084d92325174179c65b4292f5809f052b1d450c7520` |
| PCR1 | `4b4d5b3661b3efc12920900c80e126e4ce783c522de6c02a2a5bf7af3a2b9327b86776f188e4be1c1c404a129dbda493` (= 2026-11-12) |
| PCR2 | `e550477971775beb0f24a9afc2457e239404b5da83121b14c84478da7ed424bf730ce6e7d286dc6a3aec3f371b5dfc72` |
| note | PCR0/PCR2 differ from 2026-11-12 (`d5bbd4af…`/`6b64250f…`) and 2026-11-15 (`47dbca56…`), as `test-matrix.md` T1 predicts after `a402827` and `e57f02d`. The first values at `6a92a1f`. Only the Auditor built, so this is **not** a two-machine T1. *Later the same day:* `1967749d` changed the responder (`5573b538…` → `c61c988e…`), so these values are superseded too; the baseline was retaken at `23b83f9` (`docs/results/aws-nitro-2026-11-16.md`) |
| reproducibility scan | 9 warnings in the nginx Dockerfile (unpinned base tag, `apk add` without versions, …) |
| swapped image (flow 1 step 4) | PCR0 `9f1ebbec9da57cb0d95bc32bc2e7592ee8624ffdd85bde62178aea957295a6f638ef173875d35f444136e0a7ae1d81e2`, PCR2 `c7cd2011…cfcc` |
| one byte changed | PCR0 `c0263871dbded159e22795f8f3bd4b07773aa37ba3e190953cf921f1f6b75f9e5bb0ebe8a079bbb4e57c9b323a1abb5a`, PCR1 unchanged, PCR2 `8aeabedf9dea0903a4a95a08362afe70e37105905f5dbd784516b47e6f2a1e67d68fce5d06e2e041f60471621733a094` |
| record key | `0x89fecb6e0e8a463b828b7c2e0bb150956e7d0b4f8b62da3fa047a89f5debd865` (the same in both registries: approver, platform and measurement were the same) |

### TDX

| what | Option 1 | Option 2 |
|---|---|---|
| base initrd | stock, sha256 `4703ec07684196e8616d384ac585be54b6345ca8c6ea5544aab8445e0cd4b0a2` | made with the hooks, `8ee883a8ff9d5e33c8383bdcd909c857ec34ff97cabcd4d2bda98bfdde48e013` (differs every time) |
| root hash | — | `5dff1f1db0e18a2b31ac8b2e2101471601f613211cc98b9ea02bd2485c2d643f` (= 2026-11-15 on a guest and in WSL) |
| initrd sha384 | `bc0248877db758b303f3ae80adbc1d8aacc99d609cb9070f141c4c9e8b5aff985aa603da6c1148a3397bdff3ab98c43b` | `89d77351ee14d895d0a4e706bcf8513760cd17a38a0e364dcb14ad24b09ec6d5c40941a067f847fbf2f29f705e72501d` |
| MRTD | `f21e6b48743fe14c2d090078ca1197f09ae4c99438677f2c79901ef80e2aeefe2f9a7eb6e3eb3b5047c4f1511731e5d3` | the same |
| RTMR0 | `8847ec5d4441a3e41856f9de32c21676a788060c2746c565aee58b90a882fd427e9e45487dd992cd87eaf0b2fcdfee68` | the same |
| RTMR1 | `6e1a3f8856f3119c3036497985d35f79c1744ad141eed42a7fe75aea0de87ee9437d9c71e7a04bbc3af59d7d7842734a` | the same |
| RTMR2 | `0f36ec64f4436f40953c66afb557dd852fed81b4f1edb98fcf784352d159d6b671e2df4570da69ea50c4278985a23ba7` | `ca9a7abce85376564635f2cd2b726b1b2d028c44957608b1210f8d0318e68a935ce290bd5701efa2ecb1b1f08e0a6ace` |
| one byte changed | initrd `ef4dd5f8c27c5efbcb13623c97911f1f69a6348d1530f71fb9e022ec60f5b8cae1e74154e77c5702c8caa4c144d16dd7`, RTMR2 `f36c52029f91989e2989d567a8ddbc3bb5558653fb0f4676c2b1eb86d8c18b105c725b213dd67db7f035151fde6ac24c` | root hash `15e18e5edb718e73e9e8fc18935e8a1b9d9d8ee3148b79272a8f43f9691027e7`, initrd `ebd3ad55…9a91`, RTMR2 `de2d3b8b895df40758e8c98d437819aa3ea69ded7aa98092d48b71a80645f853d7f2940b022f2aa04d9c2ce2685b3fea` |
| record key | `0xe71fc43bf988ef8bb3d236324360798e6ad12124fda01933b5728c40429de2a9` | `0x926f767c7a9ee55488489ec3c471b844fdfec2ee756ee701ff8f4fdcc4883824` |

Option 1's base initrd, initrd and all four values **equal the 2026-11-15 trial** (`docs/results/trial-2026-11-16.md`):
a third guest of the same image, made a day later, measured the same.

### SEV-SNP

| what | Option 1 (`amd-sev-snp-initrd`) | Option 2 (`amd-sev-snp-verity`) |
|---|---|---|
| base initrd | stock `4703ec07…b0a2` (the same file as on TDX) | with the hooks, `8e45bd98a835043dc71d5c8cedb9c78d1b7f9e258e6964d66c3125b5935c2f1f` |
| MEASUREMENT | `f67900851e25a378f04b4d4b5f1ea4410c11deb74e97ddcc7e3363a33f6e1dbb7880d304005387bc98e9cbdceec4ea28` | the same |
| PCR4 | `5b65d2d390d40a5dffb29675a90939615b25aaf164a8b493743242e6f5e84389` | the same |
| KERNEL | `3cf8c2c34cd30720d3f37c8b5327ae7d10f24073e73a34edb1e4aaea4dd39adb` | the same |
| CMDLINE | `80dfd7d3a1f7ba936075eb6666f5e72c865fa2c11295ba0f35cfbdb276cc3de0` | `d12176cc31f02a72a8b41fe772a5b07acd2e498ab0e15ff510efc123fea6486d` (carries the root hash; = 2026-11-15) |
| INITRD | `11cd710ea6ab1172674109ee431e344ecbdb79ab30ac40467ba074fcef594737` | `8e45bd98…2f1f` (the base itself) |
| PCR12 | — | `33784785e3150c4ee3295efb76fe252a009451e19a05c9c14076e4b57b27c5fa` (= predicted by `build`; = 2026-11-15) |
| root hash, image | — | `5dff1f1d…643f` (the same as TDX), `app.img` sha256 `cbcbb19772efc7bd14a03a356b7b3c20bcc3dca14de36d01dca4fa8444ae7eae` |
| one byte changed | INITRD `db9c977900ccbfaf151bf338ce6cb9bbafd73854a68f8b9e517a488c694ee41c` | root hash `15e18e5e…27e7`, CMDLINE `ac98ccae39b7b0eaaa698b4be8614568f23336f8e0522276ce72217c66505ccb`, PCR12 `35850d87a7bd0e4d1986115464f231cfdb9cface23a69e270aaf37ca600f7007` |
| record key | `0xc1a8ae56ba014fecc497845c2e8b884aef0225daa355db5884f557ac70a52fd0` | `0x232fa396d75d8de7560ae4593b2788e5ccce485fd9afba38c6448a6efe7183ec` |
| evidence | 68,353 bytes | 68,605 bytes |

All of Option 1's five values **equal the 2026-11-15 trial**.

### Exit codes

| command | Nitro | TDX | SEV-SNP |
|---|---|---|---|
| `audit` | 0 (`PASS: audited and registered`) | 0 (both options) | 0 (both options) |
| `verify`, registered | 0 | 0 (both options) | 0 (both options) |
| `verify`, app changed | 1 (whole image), 1 (one byte) | 1 (both options) | 1 (both options) |
| `verify`, empty registry | 1 | 1 (both options) | 1 (both options) |
| `ra-parse`, fresh document | 0 | 0 (also on the stock boot) | 0 on our boot; **7 on the stock boot** |
| `ra-parse`, document 1 against request 2's nonce | 6 | 6 | 6 |
| `in-guest-verify.sh` | — | 0 (11 passed) | 0 (11 passed) |
| `replay-check.sh` | 0 | 0 | 1 on the stock boot (D18), 0 on our boot |
| `0-Setup-local-upload.sh` as documented | 1, silent (F3); 0 with the workaround | — | — |
| `aws-nitro.sh create` / `gcp-cvm.sh … create` | 1 (F1), then 126 (F2) | 126 (F2) | 126 (F2) |
| `flow1/4` | 255, then 1 (F4), then 0 | — | — |
| `go test ./...` (local) | 0 | | |

## Afterwards (2026-11-16)

What changed in the repository because of this trial. Nothing here was run again on an instance: the script fixes were
checked with a stand-in `aws` that records its arguments, a read-only `gcp-cvm.sh status`, and `go test ./...`.

| finding | change | commit |
|---|---|---|
| F3 | `env.sh` no longer ends a `set -e` script when an address variable is unset | `36d1775` |
| F1 | the key is written to a `.partial.pem` (gitignored) and moved on success; the error names `KEY_NAME`, `SG_NAME`, `TAG` | `535ff57` |
| F2 | `exec bash "$0" status` in all three cloud scripts | `535ff57` |
| F11, F12, D16 | `gcp-cvm.sh status`, `stop`, `start`, `delete` act only on `PREFIX`; `create` says a name is reused and how to choose another | `062b905` |
| F10 | `in-guest-verify.sh`'s `json_str` fixed, so `platform` and `reason` are written | `b7495a0e0e7` |
| F4 | flow 1 step 4 (and flow 2 step 3) start docker before `docker build` | `7e919cd` |
| F14 | `/linux/`, `/quote.bin*`, `/evidence.cbor*`, `/measurements.json` in `.gitignore` | `79394467763393f` |
| D12 | every command prints the values sorted by name, in one column | `aa0ecf2` |
| F5, F6, F13, F15, D18, step 9, `PREFIX`, the anvil archive, gcloud under WSL | `tdx.md` and `sev-snp.md` steps 1, 3, 7, 8, 9, 13 | `eae8a25`, `d275e95` |
| F7, F11, D19, Go `PATH`, a Nitro safety stop | `e2e/README.md`, `e2e/cloud/README.md` | `8d7471a` |
| — | found while fixing F2: `fd3353f` had dropped `--region` from every `aws` call in `aws-nitro.sh` and `aws-snp.sh` (the CR-stripping wrapper replaced the one that added it), so they used the region of the profile or the environment. This trial's Nitro instances are in `eu-north-1`, the script's default, so that is what the profile gave here; another profile would have made them elsewhere | `94ac753` |

Not changed, and why:

| finding | why |
|---|---|
| D3 (no independent rebuild on the CVMs) | *(Rewritten later on 2026-11-16, `375e4ee`; first: "needs a second guest; 'The Auditor's own guest' in each doc is still unrun". Later still the same day it ran and passed, on both platforms: `docs/results/sessions-2026-11-16/`.)* **written, not run.** Both procedures now say that step 11 is not an audit, and "The Auditor's half" (A1 to A9) moves Nitro's flow 1 to the CVMs, with what "the same inputs" means (`docs/build/tdx.md`, `sev-snp.md`). Running it needs two guests at once; `docs/status/cvm-port.md` Phase 3 item 6 and Phase 4b item 5 say what run confirms it |
| D1, D2, D4–D9, D11 | differences of the platforms themselves, not defects; the User's side is the same command |
| D13 | TDX and SEV-SNP evidence has no module ID and no time to show |
| D14 | the getting-started steps are typed by hand and write no JSONL; `in-guest-verify.sh` and `replay-check.sh` do |
| D17 | *(Rewritten later on 2026-11-16, `375e4ee`; first: "'enclave' in the CLI wording on VMs: cosmetic, left for a pass over all CLI text".)* partly: `audit` and `verify`, which the Auditor's half runs against guests, now say "guest" on TDX and SEV-SNP and "enclave" only on Nitro (`cli.TEENoun`); the TDX e2e test requires it. Other commands' wording (`ra-request`, `self-register`, `lookup`, usage text) is left for a pass over all CLI text |
| F9 | the Nitro AMI and the `dnf` packages are unpinned; this run's versions are recorded above and equal the baseline's except git |
| `net_retry` not retrying `Connection closed by …` | not reproduced |
| `sudo` working although `sudo -n true` asked for a password | not investigated; outside the repository |
