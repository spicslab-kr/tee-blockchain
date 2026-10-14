# User guide (`att-user`)

The User needs one binary, `att-user`, and network access. No key, no ETH, no Nitro, no Docker.
Build instructions: [Build the binaries](../README.md#build-the-binaries).

## verify

```
att-user verify --tee-addr <vendor-public-ip>:5000 --rpc <url> --registry <0x...>
```

What it does: asks the Vendor's enclave to attest, verifies the document, finds who registered
that measurement, and checks each registration. Exit 0 if at least one registration matches on
value, expiry and approver. Exit 1 with the reason if nobody registered the value or none check out.
A matching registration is shown with what the chain recorded for it, architecture included:

```
✓ matches  nginx 1.31.6 (amd64) by 0x..., expires ...
```

A registration passes only if all three hold:

- its value equals what the enclave just attested,
- it has not expired, judged on the chain's own clock,
- its approver is the registry's approver, or one of the addresses given with `--trusted-approvers`.

Flags: `--tee-addr`, `--rpc`, `--registry` (required, repeatable), `--trusted-approvers <0x...,0x...>` (comma
separated; default is each registry's approver), `--format` (default `aws-nitro`), `--timeout`
(default 30s), `--nonce` (pin the nonce, for tests), `--out-dir <dir>` (keep the intermediate files;
otherwise a temporary directory is used and removed). A value you leave out is asked for; an empty
`--rpc` defaults to Ethereum mainnet.

### Several registries

Repeat `--registry` (a comma separated list also works) to check several registries, for example
the Vendor's and the Auditor's:

```
att-user verify --tee-addr <vendor-public-ip>:5000 --rpc <url> \
    --registry <vendor registry 0x...> --registry <auditor registry 0x...>
```

The registries are shown side by side and every one of them must vouch for what the enclave runs.
If the Vendor's registry says yes and the Auditor's says no, the result is `not trusted` with exit 1.

### Naming other trusted approvers

```
att-user verify --tee-addr <host>:5000 --rpc <url> --registry <0x...> --trusted-approvers <0xApprover1>,<0xApprover2>
```

`--trusted-approvers` replaces the default (the registry's approver), and applies to every `--registry`.

## The steps it wraps

Attestation document from the Vendor's enclave:

```
att-user ra-request --tee-addr <vendor-public-ip>:5000 --out-file doc.cbor --format aws-nitro
```

Verify it and extract PCR0, PCR1, PCR2:

```
att-user ra-parse --doc doc.cbor --out-file parsed.json
```

Find the registration by measurement alone. No key from anyone is needed:

```
att-user lookup --rpc <url> --registry <0x...> --measurements parsed.json
```

The trust decision, for each record key `lookup` found:

```
att-user compare --attested parsed.json --rpc <url> --registry <0x...> --record-key <a record key lookup found> \
    [--trusted-approvers <0x...,0x...>]
```

`ra-request` writes the nonce it used to `<out-file>.nonce`, and `ra-parse` checks the document against
it. Exit codes: [Exit codes](../README.md#exit-codes).

## Key types, fields and functions

A map of the code a new reader should know. Links are relative to this file. This is not every identifier.

### Command dispatch

`main` in [main.go](main.go) switches on the first argument and calls one function per command. A missing or unknown command prints the usage text and exits 2. A returned error goes through `exitCode`.

| Name | Where | What it is or does |
|---|---|---|
| `main`, `usage` | [main.go](main.go) | Dispatch (`ra-parse`, `ra-request`, `lookup`, `compare`, `verify`) and the help text. |
| `verifyCmd` | [verify.go](verify.go) | `verify`: the whole trust decision in one command. |
| `raRequest` | [main.go](main.go) | `ra-request`: sends a fresh nonce, saves the document and `<out-file>.nonce`. |
| `raParse` | [parse.go](parse.go) | `ra-parse`: verifies the document and writes the measurements as JSON. |
| `lookup` | [chain.go](chain.go) | `lookup`: reads one record key, or finds the record keys by measurements and reads them. Returns the record keys it found. |
| `compareCmd`, `checkMode` | [compare.go](compare.go) | `compare`: either two measurement sets (`--expected`), or the attested measurements against a registration (`--record-key`). |
| `resolveTrustedApprovers` | [compare.go](compare.go) | `--trusted-approvers` if given, else the registry's own approver. |
| `readMeasurements` | [chain.go](chain.go) | Reads measurements from a `parsed.json` file or `NAME=hex,...`. |
| `quiet` | [quiet.go](quiet.go) | Set by `verify` so the single-step functions print nothing and `verify` prints its own sentences. |

### The verify flow, in order

| Step | Function | Where |
|---|---|---|
| Make a nonce and request the document | `TCPRequester.RequestFresh` (random nonce from `CryptoRand`, then `Request`), called through `raRequest` | [tcp.go](../../internal/tee/request/user/tcp.go), [nonce.go](../../internal/tee/request/user/nonce.go) |
| Check the format | `RADoc.ExpectFormat` | [request.go](../../internal/tee/request/user/request.go) |
| Verify the signature chain to the platform root and the nonce | `AWSParser.VerifySig` and `AWSParser.CheckNonce`, called by `raParse` | [aws.go](../../internal/tee/parse/user/aws.go), [verify.go](../../internal/tee/parse/aws/verify.go), [nonce.go](../../internal/tee/parse/aws/nonce.go) |
| Extract PCR0, PCR1, PCR2 | `AWSParser.Parse` (wraps `aws.Parser.Parse`) | [measure.go](../../internal/tee/parse/aws/measure.go) |
| Find record keys by measurement | `Reader.FindRecordKeys`, via `lookup` | [reader.go](../../internal/chain/lookup/user/ether/reader.go) |
| Read each registration | `Reader.Lookup`, returning a `Registration` | [reader.go](../../internal/chain/lookup/user/ether/reader.go), [lookup.go](../../internal/chain/lookup/user/lookup.go) |
| Decide | `Impl.Check`, with `Clock(reg)` as the current time | [compare.go](../../internal/core/compare/user/compare.go) |

`verify` repeats the last three steps for every `--registry` and trusts only if each registry has at least one passing registration. The chain types live in [lookup.go](../../internal/chain/lookup/user/lookup.go): `Registration` (`RecordKey`, `Values`, `Meta`, `Expires`, `ChainTime`, `Approver`, `Signature`) and `Meta` (`Service`, `Version`, `Platform`, `Arch`).

### The trust decision

`Impl.Check(attested, reg, now, trustedApprovers)` in [compare.go](../../internal/core/compare/user/compare.go) hashes the attested measurements and hands the rest to the shared `Check` in [check.go](../../internal/core/compare/check.go).

| Input | Meaning |
|---|---|
| measurement hash | Hash of the attested PCRs (see below), compared with the hash stored in the registration. |
| registration | `Approver` (who signed it) and `Expires` (unix seconds, chain clock). |
| now | `Clock(reg)`: the time of the block the registration was read from (`ChainTime`), because the contract judges expiry by its own clock. It falls back to this machine's clock only if `ChainTime` is missing. |
| trusted approvers | `--trusted-approvers`, else the registry's approver. Addresses compare case-insensitively. |

The result is a `Verdict` (`Pass` plus a `Reason` list). It fails, listing every reason that applies, when:

- `registration expired`: `Expires` is not after now.
- `approver ... is not in the trusted list`.
- `measurement hash differs from the chain registration`.
- `could not compute the measurement hash`: no measurements, an empty name or value, or a duplicate name.

### How the measurement hash is built

`measurementHash` in [compare.go](../../internal/core/compare/user/compare.go) sorts the measurements by name (`PCR0`, `PCR1`, `PCR2`), writes one line `name=<hex value>` plus a newline for each, and takes the keccak256 of the result. The copy in [reader.go](../../internal/chain/lookup/user/ether/reader.go) must stay identical, because `FindRecordKeys` searches the registry with it. `Parse` leaves out an all-zero register (debug mode) and rejects the document if every PCR is zero.

### Document formats

`--format` names the attestation document format. `aws-nitro` is the only one that can be verified (`ra-parse`, `verify`). The `ra-request` help also lists `gcp-cs-jwt` and `azure-maa-jwt`, but those parsers ([gcp.go](../../internal/tee/parse/user/gcp.go), [azure.go](../../internal/tee/parse/user/azure.go)) are not implemented yet. An empty `--format` on `ra-request` accepts any format; `verify` defaults to `aws-nitro`.

### Why the User never signs or registers

`att-user` has no `register`, no key and no ETH. The chain side is read-only: `Reader` in [lookup.go](../../internal/chain/lookup/user/lookup.go) has only `FindRecordKeys` and `Lookup`. A User can check what an approver registered but cannot create or change a registration. That is also why the hash code is repeated here instead of imported from a register package.

### Exit codes

`exitCode` in [main.go](main.go) maps errors: 4 wrong format, 5 signature or certificate chain invalid (or no document extracted), 6 nonce mismatch or bad response, 7 malformed document, 3 network error, 1 anything else (including an untrusted `verify`). 0 is success and 2 a missing or unknown command. See [Exit codes](../README.md#exit-codes).
