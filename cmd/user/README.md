# User guide (`att-user`)

You need one binary, `att-user`, and network access. You need no key, no ETH, no Nitro and no Docker.
Build instructions: [Build the binaries](../README.md#build-the-binaries).

## verify

```
att-user verify --tee-addr <vendor-public-ip>:5000 --rpc <url> --registry <0x...>
```

`verify` (`verifyCmd` in [verify.go](verify.go)) asks the Vendor's enclave for an attestation
([`TCPRequester.RequestFresh`](../../internal/tee/request/user/tcp.go)), checks the document
([`RADoc`](../../internal/tee/request/user/request.go) with `ExpectFormat`), finds who registered the
measurement it contains ([`Reader.FindRecordKeys`](../../internal/chain/lookup/user/ether/reader.go)), and checks
each registration it finds ([`Registration`](../../internal/chain/lookup/user/lookup.go), read with `Reader.Lookup`).

A registration passes when all three hold. [`Check`](../../internal/core/compare/check.go) decides:

- its measurement hash equals the hash of what the enclave just attested (`Registration.Values`, compared with the hash from `measurementHash` in [compare.go](../../internal/core/compare/user/compare.go)),
- it has not expired (`Registration.Expires`), judged on the chain's own clock (`Clock`, which returns `Registration.ChainTime`),
- its approver (`Registration.Approver`) is the registry's approver, or one of the addresses you give with `--trusted-approvers`.

For one registry, `verify` exits 0 when at least one of its registrations passes (`Verdict.Pass`). With several
`--registry` flags (collected in `registries`), every one of those registries needs at least one passing registration. Otherwise it
exits 1 and prints the reason (`Verdict.Reason`).

Flags:

- `--tee-addr` is the enclave's `host:port`.
- `--rpc` is the Ethereum RPC URL. If you leave it empty, `verify` asks for it and offers Ethereum mainnet.
- `--registry` is the registry contract address. It is required and you can repeat it.
- `--trusted-approvers <0x...,0x...>` is a comma separated list. The default is each registry's own approver. The value is held in `trustedApprovers`.
- `--format` is the expected document format (`format`, passed to `ExpectFormat`). The default is `aws-nitro`.
- `--timeout` is the request timeout. The default is 30s.
- `--nonce` pins the nonce, for tests.
- `--out-dir <dir>` keeps the intermediate files. Without it, `verify` uses a temporary directory and removes it at the end. The variable is `outDir`.

If you leave out `--tee-addr` or `--registry`, `verify` prompts for it.

### What a trusted run prints

```
Asking the enclave at <host>:5000 for an attestation... ✓ aws-nitro document, <n> bytes
Checking the attestation (signature chain to the platform root, nonce)... ✓ genuine
    PCR0  <hex>
    PCR1  <hex>
    PCR2  <hex>
Looking up who registered what this enclave runs, in the registry at 0x...... ✓ 1 registration(s)
    checking each against the attestation (value, expiry, approver):
    0x<record key>  ✓ matches  nginx 1.31.6 (amd64) by 0x<approver>, expires 2027-01-01T00:00:00Z
trusted: this enclave runs exactly what 0x<approver> registered
```

The dotted lines that start with `·` explain each step and are left out here. The registration line
shows what the chain recorded for the registration (`Registration.Meta`, architecture in `Meta.Arch`). Exit code: 0.

If nobody registered the measurement, the lookup step ends in `✗` and the run stops with:

```
att-user: nobody has registered these measurements on the chain, so there is no basis for trust
```

The exit code is 1.

### Several registries

Repeat `--registry` to check several registries, for example the Vendor's and the Auditor's. A comma
separated list also works.

```
att-user verify --tee-addr <vendor-public-ip>:5000 --rpc <url> \
    --registry <vendor registry 0x...> --registry <auditor registry 0x...>
```

`verify` shows the registries side by side. If the Vendor's registry vouches and the Auditor's does not,
the run prints `not trusted: only 1 of 2 registries vouch for what this enclave runs; the others never approved it`
and exits 1.

### Naming other trusted approvers

```
att-user verify --tee-addr <host>:5000 --rpc <url> --registry <0x...> --trusted-approvers <0xApprover1>,<0xApprover2>
```

`--trusted-approvers` replaces the default (the registry's approver) and applies to every `--registry`.
Addresses compare case-insensitively.

## The steps it wraps

Each step is a command of its own. You can run them one at a time.

Request an attestation document from the Vendor's enclave:

```
att-user ra-request --tee-addr <vendor-public-ip>:5000 --out-file doc.cbor --format aws-nitro
```

`ra-request` writes the nonce it used to `<out-file>.nonce`.

Verify the document and extract PCR0, PCR1 and PCR2 (`AWSParser` returns them as `Measurements`, a list of `Measurement` entries with `Name` and `Value`). `ra-parse` checks the document against the nonce file:

```
att-user ra-parse --doc doc.cbor --out-file parsed.json
```

Find the registration by measurement alone. This needs no key from anyone:

```
att-user lookup --rpc <url> --registry <0x...> --measurements parsed.json
```

Take the trust decision for each record key `lookup` found:

```
att-user compare --attested parsed.json --rpc <url> --registry <0x...> --record-key <a record key lookup found> \
    [--trusted-approvers <0x...,0x...>]
```

`compare` can also match two sets of measurements as they are:

```
att-user compare --attested parsed.json --expected other.json
```

## Troubleshooting

Every error is printed as `att-user: <text>` and sets the exit code described in
[Exit codes](../README.md#exit-codes). The function `exitCode` in [main.go](main.go) maps the error to the code. This role returns 1 (any other error, including an untrusted
result), 3 (network error), 4 (wrong format), 5 (invalid signature or certificate chain), 6 (nonce
mismatch or bad response) and 7 (malformed document).

- `nobody: this registry's approver never registered the value this enclave runs`: The registry holds no record for the measurement the enclave attested. Check that `--registry` and `--rpc` point at the right registry and network. Compare the printed PCR0, PCR1 and PCR2 with the values the Vendor published, and ask the Vendor or Auditor to register them.

- `registration expired`: A registration exists and its value matches, but its expiry is not after the chain's current block time. Ask the approver to register the measurement again with a later expiry.

- `approver 0x... is not in the trusted approvers`: The registration was signed by an address that is neither the registry's approver nor in your `--trusted-approvers` list. If you know and trust that address, pass it with `--trusted-approvers`. Otherwise treat the registration as untrusted.

- `document nonce is not the one sent` (exit 6): The document does not carry the nonce you sent, so it may be a replay. Run `verify` again. If you passed `--nonce`, remove it, because `--nonce` replaces the nonce `verify` just sent.

- `attestation document has a different format than expected: got "gcp-cs-jwt", want "aws-nitro"` (exit 4): The enclave answered in another format. Only `aws-nitro` can be verified. `ra-parse` reports `document format does not match this parser: no parser for "gcp-cs-jwt"`. `gcp-cs-jwt` and `azure-maa-jwt` can be requested with `ra-request` but have no verifier, so a host that answers in those formats cannot be checked with `att-user`.

- `dial tcp <host>:5000: ...` such as `i/o timeout` (exit 3): `att-user` could not reach the enclave. Check `--tee-addr`, the Vendor's firewall and that the responder is running. Raise `--timeout` on a slow link. The same exit code applies when the `--rpc` endpoint is unreachable.

## Reading the code

Execution starts in `main` in [main.go](main.go), which switches on the first argument and calls one
function per command. A missing or unknown command prints the usage text and exits 2. A returned error
goes through `exitCode`. `att-user` has no signing key and no `register` command, and its chain reader
has only `FindRecordKeys` and `Lookup`, so you can check what an approver registered and cannot create
or change a registration.

In verify order:

- [`verifyCmd`](verify.go) runs the whole sequence and prints one line per step.
- [`TCPRequester.RequestFresh`](../../internal/tee/request/user/tcp.go) creates a random nonce and fetches the document.
- [`RADoc.ExpectFormat`](../../internal/tee/request/user/request.go) rejects a document in another format.
- [`AWSParser`](../../internal/tee/parse/user/aws.go) verifies the signature chain and the nonce, and extracts PCR0, PCR1 and PCR2.
- [`Reader.FindRecordKeys`](../../internal/chain/lookup/user/ether/reader.go) searches the registry by measurement hash.
- [`Reader.Lookup`](../../internal/chain/lookup/user/ether/reader.go) reads one `Registration` (fields `RecordKey`, `Expires`, `ChainTime`, `Approver`, and `Meta.Arch`).
- [`Impl.Check`](../../internal/core/compare/user/compare.go) hashes the attested values with `measurementHash` and passes `Clock` as the current time.
- [`Check`](../../internal/core/compare/check.go) takes the hash, a `RegInfo` (`MeasurementHash`, `Expires`, `Approver`), the time and `trustedApprovers`, and returns a `Verdict` with `Pass` and `Reason`.

`Check` passes a registration only when all three conditions hold. It lists every failure that applies.

1. The expiry is after the chain's current time. Otherwise the reason is `registration expired`.
2. The approver is in the trusted list. Otherwise the reason is `approver <address> is not in the trusted approvers`.
3. The registration's measurement hash equals the hash of the attested values. Otherwise the reason is `measurement hash differs from the chain registration`.

If the attested values cannot be hashed (none, an empty name or value, or a duplicate name), `Impl.Check`
fails with `could not compute the measurement hash: <detail>`.
