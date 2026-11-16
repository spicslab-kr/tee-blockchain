# AMD SEV-SNP, session 3 of 2026-11-16: the Auditor's own rebuild (D3), T2, T6, and the relay (T10)

Run on 2026-11-16 at commit `57b4350`, with an AI coding agent under operator supervision. *(This sentence was reworded on 2026-11-16 in `9702f42`: it first named who ran the session; it now states the conditions. The facts are unchanged.)* 21 result lines, written by hand in the results contract
(`"script":"manual: …"`): 16 in `amd-sev-snp-initrd.jsonl`, 5 in `amd-sev-snp-verity.jsonl`. **20 have `result` equal
to `expected`; the one that does not is T10's relay, accepted where the row expected refused, which is the finding.**
Local anvil 1.8.0 (`NET=local`), verifier on the operator's Windows machine.

## Setup

| what | value |
|---|---|
| guests | `d3-snp-vendor` and `d3-snp-auditor`, made in **one** call: `PREFIX=d3-snp IMAGE=ubuntu-2404-noble-amd64-v20261105 bash e2e/cloud/gcp-cvm.sh snp create vendor auditor` |
| image | `ubuntu-2404-noble-amd64-v20261105`, the same version as session 2's TDX guests. It advertises `SEV_SNP_CAPABLE` as well as `TDX_CAPABLE`, and three earlier `n2d-standard-2` SEV-SNP guests from it had passed `launch --check` (checked before creating). Kernel `7.0.0-1017-gcp`, `n2d-standard-2`, `us-central1-a` |
| guest binaries | `att-vendor` `3a30019c…`, `att-auditor` `b14f445d…`, `att-ra-responder` `c61c988e…`, `record-snp` `5b0e8f5d…` (go1.26.1, `CGO_ENABLED=0 -trimpath -buildvcs=false`) |
| state now | both **stopped** (TERMINATED); the boot entry and `/var/lib/attestamp` undone (step 13) before the stop |

## D3: the Auditor's half, followed as written

`docs/build/sev-snp.md`, "The Auditor's half", A1 to A9, both formats. **Passed, with the negative control.**

| | `amd-sev-snp-initrd` (Option 1) | `amd-sev-snp-verity` (Option 2) |
|---|---|---|
| base initrd | each guest's own `/boot` file, `4703ec07…` on both | the Vendor's `base-initrd-v.img`, `2c81cd1a…`, copied to the Auditor |
| Vendor's build | INITRD `11cd710e…` | INITRD `2c81cd1a…` (the base itself), root hash `5dff1f1d…`, PCR12 `33784785…` |
| Auditor's build (A4) | **the same** | **the same** three |
| both guests' values | MEASUREMENT `f6790085…`, PCR4 `5b65d2d3…`, KERNEL `3cf8c2c3…`, CMDLINE `80dfd7d3…`, INITRD `11cd710e…`, **equal** | the same MEASUREMENT, PCR4, KERNEL; CMDLINE `d12176cc…`, INITRD `2c81cd1a…`, PCR12 `33784785…`, **equal** |
| A8 `audit --vendor-tee` | `identical: the guest runs exactly what I built`, `same: the Vendor's guest runs exactly what I built`, `PASS` | the same |
| A9 `verify` of the **Vendor's** guest against the Auditor's registry | **exit 0**, `trusted` | **exit 0**, `trusted` |
| negative: one byte changed on the Vendor's side only | Vendor INITRD `db9c9779…`; `audit --vendor-tee`: `the Vendor's guest runs something else` (`INITRD: values differ`), exit 0; `verify`: **exit 1** | root hash `15e18e5e…`, PCR12 `35850d87…`, CMDLINE `ac98ccae…`; `audit`: `runs something else` (PCR12 and CMDLINE differ); `verify`: **exit 1** |

Option 1's five values equal those of every earlier SEV-SNP guest of this image (2026-11-15 and the clean trial).

**A second unplanned negative control.** The local `vendor-app` from session 2 was still there (removing it was
refused by the permission check, and that was not worked around), so fetching the Vendor's `app` into it nested the
folder: the Auditor's `app` held `hello.txt` **and** `app/hello.txt`. Its build gave INITRD `ab36d68c…` instead of the
Vendor's `11cd710e…`, and A4's comparison stopped the procedure before anything was launched or registered (T5,
`rejected`, as expected). The copy was redone through a fresh local folder, `vendor-app-snp`. Together with session 2's
empty `app`, that is two input mistakes, both caught by the same comparison before registration.

**What was done differently from the page.** As in session 2: every guest command through `gcloud compute ssh
--command`, `launch --reboot` started with `nohup … &`, host keys answered `y` (the Auditor guest got the address the
TDX Auditor had, so `pscp` asked to replace a cached key). One `audit` was first run with a mistyped registry address
and failed at registration (`no contract code at given address`) after both comparisons had passed; it was run again
with the right address. That was an operator mistake, not a finding.

## T2: only the Go version differs

The same two Go applications as on TDX (`cmd/ra-responder`, all three settings), each alone in `app/`, built with
`att-auditor build --platform amd-sev-snp-initrd` and booted on the Auditor's guest.

| | go1.26.1 | go1.26.2 |
|---|---|---|
| binary | `c61c988e…` | `24724239…` |
| INITRD on the guest | `48e09d59…` | **`bca95760…`** |
| MEASUREMENT, PCR4, KERNEL, CMDLINE | equal | equal |

An `att-vendor` built with go1.26.2 refuses `build` on the guest (exit 1), now with SEV-SNP's own reason: `on
amd-sev-snp-initrd att-ra-responder is not measured; the measurement covers the initrd … that att itself writes`
(the message was fixed between the sessions).

## T6: the Vendor self-registers

As on TDX: with the Vendor's guest running the one-byte-changed application, `self-register` into the Vendor's own
registry (exit 0); `verify` against it alone exit 0; `audit --vendor-tee` `runs something else`; `verify` with both
registries required **exit 1**, `only 1 of 2 registries vouch`.

## T10: the relay, reproduced

**The relay is accepted.** VM A was the Auditor's guest, running the approved application (INITRD `11cd710e…`,
registered). VM B was the Vendor's guest, running the one-byte-changed application (INITRD `db9c9779…`, not
registered). One nonce text for all, `attestamp-snp-t10` (nonce `0f896406…8766`).

| document | what it is | `ra-parse` | values |
|---|---|---|---|
| `a.cbor` | A's own pair | **exit 0** | INITRD `11cd710e…` (approved) |
| `b.cbor` | B's own pair | **exit 0** | INITRD `db9c9779…`; `lookup` in the Auditor's registry: nothing registered |
| `naive.cbor` | B's own report with A's vTPM attestation | **exit 5**, `REPORT_DATA is not SHA3-512 of the vTPM attestation` | — |
| `relay.cbor` | B's report **requested over** SHA3-512 of A's vTPM attestation (`record-snp -relay-att`) | **exit 0**, `signature ok`, `nonce ok` | **A's** values (INITRD `11cd710e…`); `lookup` finds the Auditor's registration (record key `0xc1a8ae56…`) |

A's and B's reports carry different `CHIP_ID`s: the two VMs ran on different physical hosts. A's AK certificate names
A's instance (subject and GCE instance-information extension); B's report has nothing to compare it with. What this
means, and what could be done about it: `docs/build/ra-comparison.md`, "Do both halves come from the same VM?".

The recordings are **not committed** (the operator's decision, 2026-11-16): every `att.pb` carries the project ID and
an instance's name and ID inside Google's signed AK certificate. They are kept locally; where, their hashes and how to
make them again: `docs/build/app-fixture-recording.md`, "Reproducing T10". T10 therefore stays a live result, like
Nitro's T8.
