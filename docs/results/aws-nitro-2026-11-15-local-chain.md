# Nitro, 2026-11-15: both flows on a local chain (T4, T5, T6)

Both flows, run end to end against the two real Nitro instances, with the registry on a **local anvil chain, not
Sepolia**. Every verdict is in [`aws-nitro-2026-11-15-local-chain.jsonl`](aws-nitro-2026-11-15-local-chain.jsonl): the
11 lines of run `20261115T1040Z-local`, copied unchanged from `e2e-work/results/aws-nitro.jsonl`. Each carries
`"chain":"local"`. All 11 have `result` equal to `expected`.

| | |
|---|---|
| code | scripts at commit `cc4ab2f` (`NET=local` in `503e269`, addresses from the command line in `cc4ab2f`); the `att` binaries and `att-ra-responder` on the instances were those of the 2026-11-15 T8/T9 run (code `1663a5d`, responder sha256 `5573b538…`); no Go code changed in between |
| chain | anvil 1.8.0 (commit `61ae26af`, the version the Go tests use) on `127.0.0.1:8545`, started once and kept running for both flows. Signing keys: anvil's development accounts 0 (deployer; the Vendor in flow 2) and 1 (the Auditor in flow 1). See `e2e/README.md`, "Running without a public testnet" |
| instances | the two m6i.xlarge of the baseline, started again; new public IPs, passed on the command line |
| security group | not changed: 22 and 5000 from the operator's address only. No instance reached the chain: every chain call ran on the operator's machine |

## Results

| row | check | result |
|---|---|---|
| T4 | Auditor builds, launches, deploys a registry and audits: attested equals built, registered | accepted |
| T4 | Vendor launches the handed-over EIF; it measures to the received values and matches the registration | accepted |
| T4 | User `verify` against the Auditor's registry | **trusted, exit 0**, `✓ matches` |
| T5 | Vendor swaps in another image; the same `verify` | **not trusted, exit 1**: "✗ nobody … never registered the value this enclave runs" |
| T6 | Vendor builds with `--verify`, deploys its own registry, self-registers; `verify` against it | accepted, exit 0 (twice: in step 1 and step 2) |
| T6 | Vendor swaps the image and self-registers it in a second registry of its own; `verify` with only that registry | accepted, exit 0: the swap passes where the Vendor is its own approver |
| T6 | Auditor audits with `--vendor-tee` | "✗ the Vendor's enclave runs something else" |
| T6 | User `verify` with both registries required | **not trusted, exit 1**: "only 1 of 2 registries vouch" |

PCR0 of the test application: `d5bbd4af…` in flow 1 (the Auditor's docker cache from 2026-11-12), `47dbca56…` in flow 2
(the Vendor built it again today). Why they differ is the finding below.

## Finding: the build day is in the image

Flow 2 step 1 failed on its first attempt, in `build --verify`:

```
PCR0 differs: build 1 d5bbd4af… / build 2 47dbca56…
✗ not reproducible. 1 path(s) differ between the two builds (mode type sha256):
  /etc/shadow  build 1: 640 f 0abf4a3fd220…  build 2: 640 f c9173f61b470…
```

Build 1 came from the Vendor's docker cache, made on 2026-11-12; build 2 (`--no-cache`) was made on 2026-11-15. The
cached image's `/etc/shadow` has `nginx:!:20769:0:99999:7:::`; day 20769 is 2026-11-12, and the day of the run was
20772. The Dockerfile's `adduser` writes the day it ran into the "last password change" field. So **two builds of the
same source on different days give different PCR0 and PCR2**, and an Auditor rebuilding a day later than the Vendor
would not match. The 2026-11-12 baseline (T1) compared two builds made on the same day and could not show this.

`build` does not normalise this field yet (`docs/build/aws-nitro.md`, "What `build` cannot change"). Fixing it changes
the measurement of every image that has users, so it is left for a decision rather than made here.

To go on, the Vendor's cached `alpine-slim:app` was rebuilt with `--no-cache` on the same day, and `--verify` then
passed (`✓ reproducible on this host`).

## Other things that happened

- The second attempt at flow 2 step 1 built and verified, then `nitro-cli run-enclave` failed with E29/E39: the
  tampered enclave from flow 1 step 4 still held the instance's enclave resources. The flows assume a fresh Vendor;
  running flow 2 straight after flow 1 needs the Vendor's enclave stopped first (`99-Setup-local-stop.sh` would also
  stop the Auditor's, which flow 2 step 4 needs, so only the Vendor's was terminated). The third attempt passed.
- `launch` again printed the security-group command instead of changing the group (no IAM role on the instances).
- Both instances were stopped at the end (`aws-nitro.sh stop`; `status` shows `stopped`).
