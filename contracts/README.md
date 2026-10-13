# Registry contract

`evm/Registry.sol` is the whole on-chain side: one small Solidity contract (`^0.8.28`, MIT) built on
OpenZeppelin's `ECDSA` and `EIP712`. It records who vouches for which enclave image measurement,
until when. Deploy it with `registry-deploy` (see the [Auditor guide](../cmd/auditor/README.md)
or [Vendor guide](../cmd/vendor/README.md)).

## What it stores

For each registration, one record under a key:

| field | meaning |
|---|---|
| `signer` | the address that signed the entry (always the contract's approver) |
| `expires` | unix seconds when the registration ends |
| `registeredAt` | block time at registration |

The full entry, including the descriptive fields below, is not kept in storage. It is announced in
the `Registered` event together with the signature, so anyone can read it back from the chain's
logs.

## The signed entry (EIP-712)

The approver signs a typed `Entry`, under the EIP-712 domain `Registry`, version `1`:

| field | meaning |
|---|---|
| `platform` | the measurement format, for example `aws-nitro` |
| `arch` | the CPU architecture the image was built for, `amd64` or `arm64`. Display only; it is signed but is not part of the key |
| `service` | service name, for example `nginx` |
| `version` | service version, for example `1.31.6` |
| `measurement` | one 32-byte value: the measurements (PCR0, PCR1, PCR2), ordered by name and hashed with keccak256 |
| `expires` | unix seconds, required and in the future |

`register` rejects an entry with an empty `platform`, `arch`, `service` or `version`, a zero
`measurement`, a zero `expires`, or an `expires` that is not after the current block time.
`digest(entry)` returns the hash the approver signs, so the Go side can check it computes the same
value.

## One approver per contract

The approver address is fixed in the constructor and cannot change (`APPROVER`, immutable). The
contract recovers the signer from the signature and accepts the registration only if it is the
approver. Anyone may send the transaction and pay the gas; the signer is the registrant. A Vendor
and an Auditor who both register each deploy their own contract, and a User compares them by
passing several `--contract` values to `att-user verify`.

## Never overwrites, never revokes

A key that already has a record is rejected (`AlreadyRegistered`). There is no update and no
revoke function. Expiry is the only way a registration ends, which keeps the contract small and
the history unchangeable. Rotate by letting a registration expire and registering a new one.

## The key

```
key = keccak256(signer, platform, measurement)
```

`signer` is always the approver, so in effect there is one record per `(platform, measurement)`.
`keyFor(platform, measurement)` computes it for this contract and needs no registration to exist,
which is how `lookup` finds a registration from the measurement alone, with no key from anyone.
Because the measurements of an x86_64 build and an arm64 build differ, the two architectures get
different keys and each has its own registration.

## Lookup and the chain's clock

`lookup(key)` returns `(signer, expires, registeredAt, chainTime)`, where `chainTime` is the
current block timestamp. Whether a registration has expired is judged against `chainTime` and not
against the clock of the machine asking, so a skewed local clock cannot make an expired
registration look valid or a live one look expired.

## Errors

`NotAnApprover(signer)`, `AlreadyRegistered(key, expires)`, `MissingField(field)` and
`ExpiryAlreadyPassed(expires, nowTime)`. The commands map the first three to exit codes 4, 5 and 6
(see [Exit codes](../cmd/README.md#exit-codes)).
