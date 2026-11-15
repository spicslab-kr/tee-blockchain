# Recorded Nitro attestation document: `nitro-attestation.cbor`

A document from a real AWS Nitro enclave, served back by the fixture extractor (`ra-serve --extractor fixture=<doc>`)
so the request/response path can be tested with no enclave: `cmd/request_e2e_test.go` checks that `att-user
ra-request` receives exactly these bytes. The four `hash_golden_test.go` files use its PCRs as the `aws-nitro` case of
the measurement hash.

| file | content |
|---|---|
| `nitro-attestation.cbor` | the COSE_Sign1 attestation document, 4,543 bytes, sha256 `6bbcd24161878956656f4e785cf773f7aa570894a5f732c64303d0b5e56fb54c` |

There is **no nonce file**: the nonce sent with it was not kept. The fixture extractor serves the document whatever
nonce is asked for, so it is not bound to the request (it prints a warning saying so), and nothing that needs the
nonce check uses this file; `internal/tee/parse/aws/testdata/fixture-my-web.*` is the one with its nonce.

## What is known about it

Read from the document itself on 2026-11-12 (`att-user ra-parse --doc nitro-attestation.cbor --format aws-nitro`): the
signature chains to the AWS Nitro root.

| | |
|---|---|
| made | **2026-09-14 05:21:38 UTC**, the document's own signed `timestamp` |
| added to this repository | 2026-11-10, with the initial import (`4364e19`) |
| PCR0 (before `fc105b0`) | `af229a81a0d7f55ff47d6b06bda729f6680bd42220ad31c4f5ecd952084dade9cab1f08b4e7f5962352f3c9d452514fc` |
| PCR1 | `3b4a7e1b5f13c5a1000b3ed32ef8995ee13e9876329f9bc72650b918329ef9cf4e2e4d1e1e37375dab0ba56ba0974d03` |
| PCR2 (before `fc105b0`) | `bd1cddbb73cd08a147ac41e19fcedca0b602d24b0432562a726ea4cdabb783e1ddaee8c0ff4000752460a68575f2580f` |
| note | the recording stays valid as it is (the golden hash tests use it unchanged). "Before `fc105b0`": a rebuild of the same image with today's `build` may give another PCR0 and PCR2 (`docs/build/aws-nitro.md`) |
| debug | false |
| nitro-cli | **not recorded**. Its PCR1 differs from both `fixture-my-web` and the 2026-11-12 baseline (nitro-cli 1.5.0, x86_64), so it was made with other enclave kernel blobs: another nitro-cli version or another architecture. Which one cannot be told from the document |
| AMI, instance type, region, application | **not recorded** |

## Identifying information

The signed `module_id` carries an **EC2 instance ID**, a different one from `fixture-my-web`'s. It cannot be removed
without re-recording; publication is a decision for a person (`docs/plan/build-info-gaps.md`, #15;
`docs/plan/audit-2026-11-12.md`).
