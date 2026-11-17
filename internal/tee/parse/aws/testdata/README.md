# Recorded Nitro attestation document: `fixture-my-web`

A document from a real AWS Nitro enclave running nginx ("my-web"), and the nonce that was sent with it. Used by
`internal/tee/parse/aws` (`fixture_test.go`, `measure_test.go`) and by `cmd/parse_e2e_test.go` and
`cmd/flow_e2e_test.go`, so that parsing, verification and the trust decision are tested with no enclave running.

| file | content |
|---|---|
| `fixture-my-web.cbor` | the COSE_Sign1 attestation document as `nsm` returned it, 4,503 bytes, sha256 `f20fde0e7f406033cbd5481cb8609fd0e9b5c8e0737e74e2c8dd9a475bc329b6` |
| `fixture-my-web.nonce` | the 32-byte nonce sent with the request, as 64 hex characters and a newline (sha256 of the file `c3091476…9433`) |

## What is known about it

Read from the document itself on 2026-11-12 (`att-user ra-parse --doc fixture-my-web.cbor --format aws-nitro --nonce
fixture-my-web.nonce`): the signature chains to the AWS Nitro root, the nonce matches.

| | |
|---|---|
| made | **2026-09-21 16:04:35 UTC**, the document's own signed `timestamp` |
| added to this repository | 2026-11-10, with the initial import (`dcd8e90`) |
| PCR0 (before `a402827`) | `a0099c1cd2af380adf19601ad09aa49380fc8c3c78e875ac9fe958fc6a836a8d3c30ca5ff5d66c9dc7293cd30ce67fb1` |
| PCR1 | `4b4d5b3661b3efc12920900c80e126e4ce783c522de6c02a2a5bf7af3a2b9327b86776f188e4be1c1c404a129dbda493` |
| PCR2 (before `a402827`) | `62f677d0dea159ce42932fa7cc14b885d53d295136f74347a8572ba5f48a36e82485055108bd4f1fa2a3302ab64424cf` |
| note | a document is what it is: these values stay valid for this recording. "Before `a402827`" says only that a rebuild of the same image with today's `build` may give another PCR0 and PCR2, if the image has an account `adduser` made (`docs/build/aws-nitro.md`) |
| debug | false (PCR0 is not all zero) |
| nitro-cli | **not recorded**. PCR1 measures nitro-cli's enclave kernel and init only, and it equals the PCR1 that `aws-nitro-enclaves-cli` 1.5.0 gave on x86_64 in the 2026-11-12 baseline (`docs/results/aws-nitro-2026-11-12.md`). So the same kernel blobs were used, which is consistent with 1.5.0 on x86_64; it does not prove the package version |
| AMI, instance type, region | **not recorded** by whoever made it |
| application | nginx; its source and commit were not recorded. The PCR0/PCR2 differ from the 2026-11-12 test application's, which is expected: a different image and an older responder |

## Identifying information

The document's signed `module_id` carries the **EC2 instance ID** of the parent instance it was made on, and
`measure_test.go` asserts its prefix. It cannot be removed without re-recording. Whether to publish this file is a
decision for a person (`docs/status/build-info-gaps.md`, #15). The document carries no secret: it is signed by the
platform, and anyone may verify it.

## Recording another one

On a Nitro parent instance with an enclave running the responder (`att-vendor launch`), from any machine that reaches
it: `att-user ra-request --tee-addr <host>:5000 --format aws-nitro --out-file doc.cbor` writes the document and
`doc.cbor.nonce` next to it. Record the instance's AMI, `nitro-cli --version` and the date with it
(`docs/build/toolchain.md`, "Nitro parent instance", lists the commands).
