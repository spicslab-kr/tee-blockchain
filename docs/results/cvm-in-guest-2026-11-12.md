# The whole chain inside one confidential VM, 2026-11-12

The first run of request, verification, registration and lookup on **fresh** TDX and SEV-SNP evidence, before any
build or launch existed for those platforms: `e2e/cvm/in-guest-verify.sh` (added in `71bad06`) on one GCP guest per
platform, everything on that guest. Recorded on the day in the CVM port's working notes (`d723882` for SEV-SNP,
`ffde8dd` for TDX), which are no longer tracked; this page is that record, unchanged in substance. The per-check
`results.jsonl` the script wrote on the guests was not kept.

## How it ran

Without a firewall rule, everything ran on the guest and talked over `127.0.0.1`: `att-ra-responder` (as root, which
ConfigFS TSM and `/dev/tpmrm0` need), anvil 1.8.0 (from the Foundry v1.8.0 release tarball, its sha256 checked against
the release's checksum file), `att-auditor registry-deploy` and `audit`, `att-user verify`. The binaries were built
locally with `-trimpath -buildvcs=false` and their sha256 checked on the guest after copying.

What the run does **not** prove:

- **No rebuild.** The Auditor's expected values were the first live document's own values, because no build or launch
  existed for these platforms yet. The run proves request, verification, registration and lookup on fresh evidence.
  The Auditor's own rebuild ran later (2026-11-16, [`sessions-2026-11-16/`](sessions-2026-11-16/)).
- **The verifier ran inside the attested guest.** A verifier on another machine ran on 2026-11-15
  ([`trial-2026-11-15.md`](trial-2026-11-15.md)).

## Results

| guest | boot | command | result |
|---|---|---|---|
| SEV-SNP (`n2d`), kernel `7.0.0-1017-gcp` | stock (no application in the initrd; INITRD `4703ec07…`, the distribution initrd) | `sh in-guest-verify.sh snp-initrd amd-sev-snp-initrd` | **11 passed, 0 failed** |
| the same | the same | `sh in-guest-verify.sh snp-verity amd-sev-snp-verity` | **refused, as it must be**: the boot had no `attestamp.roothash=` on the command line ("the kernel command line has no attestamp.roothash=, so no disk image was checked"); the verity format needs a verity boot |
| TDX (`c3-standard-4`), kernel `7.0.0-1017-gcp` | stock, RTMR2 `9960b923…` | `sh in-guest-verify.sh tdx intel-tdx` | **11 passed, 0 failed** |
| the same | the application mounted from a dm-verity image at boot (v2, `docs/build/app-fixture-recording.md`, "TDX Option 2 mounted at boot") | `sh in-guest-verify.sh tdx intel-tdx` | **11 passed, 0 failed**; the live quote's RTMR2 equalled the recording's |

The SEV-SNP run's checks, one by one:

| check | result |
|---|---|
| two requests answered, 67,810 bytes each | pass |
| each verifies (VCEK chain, vTPM, SHA3-512 binding, nonce) | pass |
| the nonce check ran ("nonce      ok") | pass |
| the two documents differ | pass |
| document 1 against request 2's nonce | refused, exit 6 (T9, live) |
| registry deployed on anvil | pass |
| `att-auditor audit` registered | pass |
| `att-user verify` | exit 0, `✓ matches  in-guest 1.0.0 (amd64) by 0x7099…79C8` |

On TDX the same checks passed: 8,000-byte quotes, the two differ, a replay against the next nonce refused with exit 6,
audit registered, verify exit 0 with `✓ matches`.

The TDX guest could not be started on the first attempt that night (`ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS`, no
`c3-standard-4` in `us-central1-a`); the second attempt, later the same night, started it. Both guests were stopped
afterwards.
