# att-user

You need the `att-user` binary and network access. You need no key, no ETH, no Nitro and no Docker.
**What you build:** `att-user` only. See [Build](../../README.md#build).

## verify

```
att-user verify --tee-addr <enclave-ip>:5000 --rpc <url> --registry <0x...>
```

`verify` (`verifyCmd` in [verify.go](verify.go)) asks the enclave for an attestation
(`TCPRequester.RequestFresh`), checks the document (`AWSParser`), finds who registered the measurement it
contains (`Reader.FindRecordKeys`) and checks each registration it finds (`Reader.Lookup`).

[`Check`](../../internal/core/compare/check.go) passes a registration when all three hold:

- Its value equals what the enclave attested.
- It has not expired on the chain's clock (`Registration.Expires`).
- Its approver (`Registration.Approver`) is the registry's approver or one you list with `--trusted-approvers`.

With one registry, `verify` exits 0 when one registration passes. With several `--registry` flags, for example
the Vendor's and the Auditor's, every registry needs one passing registration. Otherwise it exits 1 and
prints the reason. `--trusted-approvers` takes a comma separated list, replaces the registry's approver
as the default and applies to every registry. Addresses compare without regard to case.

Flags:

- `--tee-addr` is the enclave's `host:port`.
- `--rpc` is the Ethereum RPC URL.
- `--registry` is the registry contract address. You can repeat it.
- `--trusted-approvers <0x...,0x...>` names the approvers you trust.
- `--format` is the expected document format. Only `aws-nitro` can be verified. This is the default.
- `--timeout` is the request timeout. The default is 30s.
- `--nonce` pins the nonce, for tests.
- `--out-dir` keeps the document and the measurements. Without it, `verify` uses a temporary directory.

If you leave out `--tee-addr` or `--registry`, `verify` asks for them.

## What it prints

```
Asking the enclave at <host>:5000 for an attestation... ✓ aws-nitro document, <n> bytes
Checking the attestation (signature chain to the platform root, nonce)... ✓ genuine
    0x<record key>  ✓ matches  nginx 1.31.6 (amd64) by 0x<approver>, expires 2027-01-01T00:00:00Z
trusted: this enclave runs exactly what 0x<approver> registered
```

A run that finds no registration stops with exit code 1:

```
Looking up who registered what this enclave runs, in the registry at 0x...... ✗ nobody: this registry's approver never registered the value this enclave runs
att-user: nobody has registered these measurements on the chain, so there is no basis for trust
```

## Single steps

Each step of `verify` is a command of its own.

| Command | What it does |
|---|---|
| `ra-request --tee-addr <host:port> --out-file doc.cbor` | Fetches an attestation document and saves the nonce next to it. |
| `ra-parse --doc doc.cbor --out-file parsed.json` | Verifies the document and extracts PCR0, PCR1 and PCR2. |
| `lookup --rpc <url> --registry <0x...> --measurements parsed.json` | Finds the record keys registered for those measurements. |
| `compare --attested parsed.json --expected other.json` | Compares two sets of measurements. |
| `compare --attested parsed.json --registry <0x...> --rpc <url> --record-key <0x...>` | Checks one registration. `--trusted-approvers` is optional. |

`att-user` has no version command.

## Troubleshooting

- `nobody: this registry's approver never registered the value this enclave runs`: check `--registry` and `--rpc`, then ask the Vendor or Auditor to register the printed PCR values.
- `registration expired`: ask the approver to register the measurement again with a later expiry.
- `approver 0x... is not in the trusted approvers`: if you trust that address, add it with `--trusted-approvers`.
- `document nonce is not the one sent`: run `verify` again and drop `--nonce` if you set it.
- `attestation document has a different format than expected`: the enclave answers in a format other than `aws-nitro`, which `att-user` cannot verify.
- `dial tcp <host>:5000: i/o timeout`: check `--tee-addr` and the Vendor's firewall, or raise `--timeout`.

## Reading the code

- [main.go](main.go) switches on the first argument and calls one function per command. `exitCode` maps an error to an exit code.
- [verifyCmd](verify.go) runs the whole sequence.
- [AWSParser](../../internal/tee/parse/user/aws.go) verifies the signature chain and the nonce.
- [Reader](../../internal/chain/lookup/user/ether/reader.go) has `FindRecordKeys` and `Lookup`.
- [Check](../../internal/core/compare/check.go) fails a registration with `registration expired`, `approver <address> is not in the trusted approvers` or `measurement hash differs from the chain registration`.

This binary has no signing key and no register command, so it can read the registry and cannot change it.
