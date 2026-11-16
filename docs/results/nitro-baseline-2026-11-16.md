# Nitro baseline, taken again (session 1 of 2026-11-16): what to measure and what to change

Written before the run, while SSH to the instances was blocked by a changed public address (the security group allows
only the old ones; the operator updates it). Everything below needs no SSH to prepare.

## Why the recorded values are stale

Three commits since the 2026-11-12 baseline change what a Nitro build gives for the same source:

| commit | what it changes | effect on the test application's values |
|---|---|---|
| `fc105b0` | `build` empties the build day `adduser` writes into `/etc/shadow` | PCR0 and PCR2 move; PCR1 does not |
| `ade66e6` | every build is `CGO_ENABLED=0` | none on the recorded values: they were already built with the cgo-free cross-built responder `5573b538…` |
| `029bbdc` | adds `TEENoun` to `internal/cli`, which `att-ra-responder` imports | **the responder's bytes change, `5573b538…` → `c61c988e…`** (built just before and at `029bbdc`, 2026-11-16), so PCR0 and PCR2 move again. The responder never calls the new function; the package changing was enough |

So no recorded PCR0/PCR2 matches the current code, including the clean trial's (`2aacdf41…`, taken at `e29d028`, before
`029bbdc`). **Finding:** any change to a package the responder links (`internal/cli`, `internal/tee/request/...`) moves
Nitro's PCR0/PCR2, even one the responder does not call. Recorded, not changed; whether the responder should depend on
`internal/cli` at all is a separate decision.

The test application's PCR0/PCR2 also depend on the day: the Dockerfile starts `FROM alpine:3.24` (a tag) and installs
`apk` packages without versions (`docs/results/aws-nitro-2026-11-12.md`). A re-measurement gives today's values; what
the baseline tests is that two machines agree on the same day.

**Recording rule, from this finding:** every measured value is written next to the commit it was measured at (and,
for Nitro, the responder's sha256). Without it, the next person who builds at another commit sees another value and
takes it for a fault. All documents changed by this session follow it.

## What to measure again

At the current HEAD (responder `c61c988e…`), on the same two `m6i.xlarge` (`attestamp-e2e`, eu-north-1, started again)
and one new `c7g.xlarge` (arm64):

| script | row | what it gives |
|---|---|---|
| `arch-measurement-compare.sh` | T1, T3 | PCR0/1/2 from the Vendor and the Auditor (two checkout paths): **the new reference values**, and whether they agree; and arm64's values, which must differ |
| `toolchain-pin-check.sh` | T2 | the refusal under go1.26.2, now with Nitro's reason (`56814d4`); the two responders' hashes |
| `responder-check.sh` | T3 | the four responder cases, including the cgo refusal (`ade66e6`) |
| `build-env-check.sh` | T7 | the x86_64 side as before, and **the arm64 mirror** (never run) |
| `arch-record-check.sh` | T7 | the `uname -m` mapping on both architectures (the arm64 side never run) |

Record the instances' AMI, kernel, nitro-cli, docker and git versions (`docs/build/toolchain.md`, "Nitro parent
instance"). The x86_64 pair's AMI is the 2026-11-12 one (`ami-048b77d5ca32f6d32`), since they are the same instances;
the arm64 instance is `ami-0ce2f8dd9b0fcfb35`, the newest Amazon Linux 2023 arm64 on 2026-11-16.

## Where the stale values are written, and what to change

| file | what is there | change after the run |
|---|---|---|
| `docs/results/aws-nitro-2026-11-12.md` | PCR0/PCR2 "before `fc105b0`" (`d5bbd4af…`, `6b64250f…`); the note "must be taken again at `ade66e6` or later" | keep as the record of that day; point to the new results file, and say that `029bbdc` moved them again |
| a new `docs/results/aws-nitro-2026-11-16.md` and `.jsonl` | — | the new reference values, the five scripts' verdicts, the arm64 values |
| `docs/build/aws-nitro.md`, lines about "before `fc105b0`" and "taken again at `ade66e6` or later" | the warning that the values are stale | say they were taken again, at which commit, and add `029bbdc` to the list of what moved them |
| `docs/status/test-matrix.md` | the "Nitro baseline 2026-11-12" legend, the T1 cell ("not yet measured"), footnote ¹³'s `e29d028` values, and the arm64 parts of T1 and T7 ("not run") | the T1 cell gets the new two-machine values; arm64 T1 and T7 get their results; ¹³ notes that `029bbdc` moved its values |
| `docs/status/build-info-gaps.md` row 2 | "the baseline must be retaken at `ade66e6` or later" | done, with the commit and the results file |
| `docs/results/aws-nitro-2026-11-15.md`, `aws-nitro-2026-11-15-local-chain.md` | values from before `fc105b0` (`d5bbd4af…`, `47dbca56…`), already marked | leave as records of those days |
| `docs/results/trial-2026-11-16-clean.md` | PCR0 `2aacdf41…`, PCR2 `e5504779…` at `e29d028` | leave as the record of that run; add one line that `029bbdc` moved them |
| `internal/tee/parse/aws/testdata/README.md`, `internal/tee/request/testdata/README.md` | the fixtures' PCR0/PCR2, marked "before `fc105b0`" | **leave**: those are the values signed into the recorded documents, not reference values |
| `docs/build/aws-nitro.md` (cgo table), `docs/status/build-info-gaps.md` 2.3, `internal/setup/build/responder_test.go` (and the project's rules of that day; the rule without the values is now in `CONTRIBUTING.md`, "Build") | `5573b538…` as the cgo-free responder | **leave**: they record the cgo comparison at `a5c4306`/`ade66e6`, which is still true for that code; add in `aws-nitro.md` that later commits give other bytes |

## Before running (once SSH works)

1. `bash e2e/cloud/aws-nitro.sh status` prints the three addresses and, since 2026-11-16, whether this machine's
   public address is allowed in. Nothing runs until it says "allowed".
2. Addresses on the command line, never in `e2e/env.local.sh` (`VENDOR_HOST=… AUDITOR_HOST=… ARM64_HOST=…`).
3. `bash e2e/0-Setup-local-upload.sh < /dev/null`: builds and uploads the x86_64 binaries and installs git on all three
   hosts (the arm64 host's git is new: `arch-measurement-compare.sh` clones there).
4. Then, each with `< /dev/null` and its output kept: `arch-measurement-compare.sh`, `toolchain-pin-check.sh`,
   `responder-check.sh`, `build-env-check.sh`, `arch-record-check.sh`. If a step fails with "Software caused connection
   abort", run that step again on its own (the 2026-11-12 note).
5. `bash e2e/cloud/aws-nitro.sh stop`: all three stopped, none terminated.

## arm64: T1 and T7

**T1 (arm64 side).** `arch-measurement-compare.sh` with `ARM64_HOST` set cross-builds `att-vendor` and the responder
for arm64 here, uploads them to the arm64 host, clones the test application at the pinned commit, runs `att-vendor
build` there and fetches `measurements.json`. Pass: the x86_64 pair agrees (T1 proper), and the arm64 values **differ**
from the x86_64 ones (`arm64-differs different`); identical values would be an anomaly (the script says so). Record
all three PCRs from arm64. Expected: PCR1 differs too, since nitro-cli's enclave kernel is per architecture.

**T7 (arm64 mirror).** `build-env-check.sh` with `ARM64_HOST` set: on the arm64 host, `receive` with a
`build-config.json` that declares **amd64** must be refused, naming both architectures; with `--ignore-env` it only
warns; one that declares arm64 passes. `arch-record-check.sh` with `ARM64_HOST` set: `uname -m` on the arm64 host must
map to `arm64`.

What arm64 needs that x86_64 did not: `git` (above); `att-vendor build` installs docker and nitro-cli by itself on
Amazon Linux 2023 (`dnf install aws-nitro-enclaves-cli docker`), as on x86_64. Not known until it runs: whether
`alpine:3.24` and its `apk` packages build the same way on arm64 (they should: the image is multi-architecture).
