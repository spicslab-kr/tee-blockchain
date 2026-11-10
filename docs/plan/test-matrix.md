# The common test matrix

The claim this project wants to support is: **the same flow, the same checks and the same verdicts
on three TEEs.** That claim is only worth anything if the same tests are actually run on all three
and the results are recorded side by side.

Nitro is the baseline. It must be run on real instances and its results recorded in the same shape
as the other two, not quoted from the README.

## The tests

| id | test | expected verdict | Nitro script today | SEV-SNP | TDX |
|---|---|---|---|---|---|
| T1 | same source built on two machines | identical measurement | `arch-measurement-compare.sh` (two x86_64 hosts) | — | — |
| T2 | only the Go version differs | different measurement | `toolchain-pin-check.sh` ¹ | — | — |
| T3 | only the build path differs, with `-trimpath` | identical measurement | inside `arch-measurement-compare.sh` | — | — |
| T4 | normal flow: Auditor rebuilds, registers, User verifies | trusted, exit 0 | `flow1-auditor-registers/1,2,3` | — | — |
| T5 | application swapped after registration | not trusted, exit 1 | `flow1-auditor-registers/4,5` | — | — |
| T6 | Vendor self-registers a different image, Auditor disagrees | not trusted when both registries are required | `flow2-vendor-self-registers/1..5` | — | — |
| T7 | build environment (OS / arch / Go) does not match | `receive` refuses | `build-env-check.sh` | — | — |
| T8 | TEE in debug mode | measurements refused | **missing** ² | — | — |
| T9 | replay of an earlier attestation | refused (nonce) | **no explicit script** ³ | — | — |
| T10 | two attestations from different machines, combined | refused | **not applicable** ⁴ | **fails today** ⁵ | — |

¹ Nitro behaves differently here by design. Attestamp pins the toolchain and `build` **refuses** to
run under another patch version, rather than producing a different measurement. The underlying fact
is the same (the script's own comment records that go1.26.2 and go1.26.8 give different binaries
even with `-trimpath`), but the product behaviour differs, so record both: refused, and — in a
separate run with the pin bypassed — the measurement that differs.

² `tee/parse/aws/measure.go` detects a debug enclave by an all-zero PCR0 and `Info.Debug` carries
it, but no e2e script launches a debug-mode enclave to prove the refusal happens. Add one.

³ The nonce check is inside `verify`, and `tee/parse/aws/nonce.go` has a unit test, but no e2e
script replays a saved document against a live verifier. The SEV-SNP runs did test this. Add it to
Nitro so the row can be filled for all three.

⁴ Nitro returns one document from one enclave; there is nothing to combine.

⁵ Reproduced on GCP and AWS: a vTPM attestation from an approved VM plus an SNP report from a
different VM passes verification. This row is the honest reason SEV-SNP is weaker than the other
two, and it has to appear in the table rather than in a footnote.

## Making the results machine-readable

The scripts today print `✅` / `❌` and set an exit code. That is enough for a person watching and
not enough to assemble a three-column table automatically.

Each test should additionally append one line to `e2e-work/results/<platform>.jsonl`:

```json
{"platform":"aws-nitro","test":"T5","result":"rejected","expected":"rejected","reason":"nobody has registered these measurements","at":"2026-11-10T12:00:00Z"}
```

Fields: `platform` is the format string (`aws-nitro`, `intel-tdx`, `amd-sev-snp-initrd`,
`amd-sev-snp-verity`), `result` and `expected` are one of `accepted` / `rejected` / `identical` /
`different` / `refused`, and `reason` is the message the tool printed.

With that in place the comparison table is generated, not typed, and a run that quietly stops
passing shows up as a missing line rather than as nothing at all.

**Do this for Nitro first.** Adding the JSONL line to the existing scripts is a small change, and
it fixes the output contract before two more platforms are written against it.

## What running the baseline needs

From `e2e/README.md`:

- two Nitro-enabled EC2 instances (Vendor, Auditor), plus an optional arm64 (Graviton) for T1/T3
- `git`, `ssh`, and exactly the pinned Go version on the local machine
- an SSH key in the repository root, and two funded key files: `auditor.key` (approver in flow 1)
  and `deployer.key` (pays gas, approver in flow 2)
- test ETH on Sepolia, which is the default. `NET=mainnet` spends real ETH and additionally
  requires `CONFIRM_MAINNET=1`
- `e2e/env.local.sh`, copied from `env.local.example.sh`, holding the instance addresses. It is
  gitignored; never commit it

Later, SEV-SNP needs GCP and AWS instances and TDX needs GCP instances. They do not all have to
exist at the same time: each platform's rows can be filled in its own session, as long as every
run writes to the same JSONL contract.
