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
