# Registry contract

`evm/Registry.sol` is the whole on-chain side: one small Solidity contract (`^0.8.28`, MIT) built on
OpenZeppelin's `ECDSA` and `EIP712`. It records who vouches for which enclave image measurement,
until when. Deploy it with `registry-deploy` (see the [Auditor guide](../cmd/auditor/README.md)
or [Vendor guide](../cmd/vendor/README.md)).

## What it stores

For each registration, one `RegistrationInfo` under a record key:

| field | meaning |
|---|---|
| `approver` | the address that signed the registration (always the registry's approver) |
| `expires` | unix seconds when the registration ends |
| `registeredAt` | block time at registration |

The full registration, including the descriptive fields below, is not kept in storage. It is
announced in the `Registered` event together with the signature, so anyone can read it back from the
chain's logs. The storage mapping is `registrations`.

## The signed registration (EIP-712)

The approver signs a typed `Registration`, under the EIP-712 domain `Registry`, version `1`:

| field | meaning |
|---|---|
| `platform` | the measurement format, for example `aws-nitro` |
| `arch` | the CPU architecture the image was built for, `amd64` or `arm64`. Display only; it is signed but is not part of the record key |
| `service` | service name, for example `nginx` |
| `version` | service version, for example `1.31.6` |
| `measurement` | one 32-byte value: the measurements (PCR0, PCR1, PCR2), ordered by name and hashed with keccak256 |
| `expires` | unix seconds, required and in the future |

The type string is
`Registration(string platform,string arch,string service,string version,bytes32 measurement,uint64 expires)`.

`register` rejects a registration with an empty `platform`, `arch`, `service` or `version`, a zero
`measurement`, a zero `expires`, or an `expires` that is not after the current block time.
`digest(registration)` returns the hash the approver signs, so the Go side can check it computes the
same value.

## One approver per registry

The approver address is fixed in the constructor and cannot change (`APPROVER`, immutable). The
registry recovers the signing address from the signature and accepts the registration only if it is
the approver. Anyone may send the transaction and pay the gas; the sender is recorded as the
`registrant` in the `Registered` event. A Vendor and an Auditor who both register each deploy their
own registry, and a User compares them by passing several `--registry` values to `att-user verify`.

## Never overwrites, never revokes

A record key that already has a registration is rejected (`AlreadyRegistered`). There is no update
and no revoke function. Expiry is the only way a registration ends, which keeps the contract small
and the history unchangeable. Rotate by letting a registration expire and registering a new one.

## The record key

```
recordKey = keccak256(approver, platform, measurement)
```

`recordKeyOf(approver, platform, measurement)` computes it for any approver. `recordKeyFor(platform,
measurement)` does the same with this registry's approver and needs no registration to exist, which
is how `lookup` finds a registration from the measurement alone, with no record key from anyone.
The approver is always the registry's own, so in effect there is one registration per
`(platform, measurement)`. Because the measurements of an x86_64 build and an arm64 build differ,
the two architectures get different record keys and each has its own registration.

## Lookup and the chain's clock

`lookup(recordKey)` returns `(approver, expires, registeredAt, chainTime)`, where `chainTime` is the
current block timestamp. Whether a registration has expired is judged against `chainTime` and not
against the clock of the machine asking, so a skewed local clock cannot make an expired
registration look valid or a live one look expired.

## Events and errors

`Registered(bytes32 indexed recordKey, address indexed approver, address registrant, ...)` is
emitted once per registration.

Errors: `NotAnApprover(approver)`, `AlreadyRegistered(recordKey, expires)`, `MissingField(field)`
and `ExpiryAlreadyPassed(expires, nowTime)`. The commands map the first three to exit codes 4, 5 and
6 (see [Exit codes](../cmd/README.md#exit-codes)).
