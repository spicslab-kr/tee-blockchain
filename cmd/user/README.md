# User guide (`att-user`)

The User needs one binary, `att-user`, and network access. No key, no ETH, no Nitro, no Docker.
Build instructions: [Build the binaries](../README.md#build-the-binaries).

## verify

```
att-user verify --tee <vendor-public-ip>:5000 --rpc <url> --contract <0x...>
```

What it does: asks the Vendor's enclave to attest, verifies the document, finds who registered
that measurement, and checks each registration. Exit 0 if at least one registration matches on
value, expiry and signer. Exit 1 with the reason if nobody registered the value or none check out.
A matching registration is shown with what the chain recorded for it, architecture included:

```
✓ matches  nginx 1.31.6 (amd64) by 0x..., expires ...
```

A registration passes only if all three hold:

- its value equals what the enclave just attested,
- it has not expired, judged on the chain's own clock,
- its signer is the contract's approver, or one of the addresses given with `--trusted`.

Flags: `--tee`, `--rpc`, `--contract` (required, repeatable), `--trusted <0x...,0x...>` (comma
separated; default is each contract's approver), `--format` (default `aws-nitro`), `--timeout`
(default 30s), `--nonce` (pin the nonce, for tests), `--out <dir>` (keep the intermediate files;
otherwise a temporary directory is used and removed). A value you leave out is asked for; an empty
`--rpc` defaults to Ethereum mainnet.

### Several registries

Repeat `--contract` (a comma separated list also works) to check several registries, for example
the Vendor's and the Auditor's:

```
att-user verify --tee <vendor-public-ip>:5000 --rpc <url> \
    --contract <vendor registry 0x...> --contract <auditor registry 0x...>
```

The registries are shown side by side and every one of them must vouch for what the enclave runs.
If the Vendor's registry says yes and the Auditor's says no, the result is `not trusted` with exit 1.

### Naming other trusted signers

```
att-user verify --tee <host>:5000 --rpc <url> --contract <0x...> --trusted <0xSigner1>,<0xSigner2>
```

`--trusted` replaces the default (the contract's approver), and applies to every `--contract`.

## The steps it wraps

Attestation document from the Vendor's enclave:

```
att-user ra-request --tee <vendor-public-ip>:5000 --out doc.cbor --format aws-nitro
```

Verify it and extract PCR0, PCR1, PCR2:

```
att-user ra-parse --doc doc.cbor --out parsed.json
```

Find the registration by measurement alone. No key from anyone is needed:

```
att-user lookup --rpc <url> --contract <0x...> --measurements parsed.json
```

The trust decision, for each key `lookup` found:

```
att-user compare --a parsed.json --rpc <url> --contract <0x...> --key <a key lookup found> \
    [--trusted <0x...,0x...>]
```

`ra-request` writes the nonce it used to `<out>.nonce`, and `ra-parse` checks the document against
it. Exit codes: [Exit codes](../README.md#exit-codes).
