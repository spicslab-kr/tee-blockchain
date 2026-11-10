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

## Key types, fields and functions

Everything below is in [`evm/Registry.sol`](evm/Registry.sol). Names were checked against it and
against the Go side.

### Constructor and `APPROVER`

`constructor(address approver)` sets the EIP-712 domain (`"Registry"`, `"1"`) and stores the approver
in the immutable `APPROVER`. A zero address reverts with `MissingField("approver")`. Nothing can
change it afterwards.

### `Registration` (the signed value)

| field | meaning | in the signed digest |
|---|---|---|
| `platform` | measurement format, for example `aws-nitro` | yes, and in the record key |
| `arch` | CPU architecture of the build, `amd64` or `arm64` | yes, not in the record key |
| `service` | service name | yes, not in the record key |
| `version` | service version | yes, not in the record key |
| `measurement` | keccak256 of the name-sorted measurements, 32 bytes | yes, and in the record key |
| `expires` | unix seconds, must be after the block time | yes, not in the record key |

Strings are hashed with keccak256 before they enter the digest, as EIP-712 requires.

### `RegistrationInfo` and `registrations`

`RegistrationInfo` is what storage keeps: `approver` (the recovered signer, always `APPROVER`),
`expires` (copied from the registration) and `registeredAt` (the block time of the transaction).
`registrations` is the public mapping from record key to `RegistrationInfo`. An entry with
`expires == 0` means nothing is registered.

### Functions

| function | inputs | returns | notes |
|---|---|---|---|
| `register(Registration e, bytes signature)` | the registration and a 65-byte signature | the record key | checks the fields and expiry, recovers the signer, requires it to be `APPROVER`, rejects a key already used, stores the info, emits `Registered`. Any account may send it |
| `digest(Registration e)` | the registration | the EIP-712 hash to sign | view; the Go side compares its own hash with it |
| `recordKeyOf(approver, platform, measurement)` | any approver | the record key | pure |
| `recordKeyFor(platform, measurement)` | no approver | the record key | same as `recordKeyOf(APPROVER, ...)`; works before anything is registered |
| `lookup(recordKey)` | a record key | `approver, expires, registeredAt, chainTime` | view; zeros (except `chainTime`) for an unknown key |

`lookup` returns `chainTime` (the current block timestamp) so the caller judges expiry on the chain's
clock, not its own. Checks in `register` run in this order: empty `platform`, `arch`, `service`,
`version`, zero `measurement`, zero `expires`, past `expires`, signer is not `APPROVER`, key already
registered.

### The `Registered` event

`Registered(bytes32 indexed recordKey, address indexed approver, address registrant, string platform,
string arch, string service, string version, bytes32 measurement, uint64 expires, bytes signature)`.
`registrant` is `msg.sender`, who paid for the transaction; `approver` is the signer. The descriptive
fields exist only here, not in storage.

### Custom errors

| error | thrown when |
|---|---|
| `MissingField(string field)` | the named field is empty or zero (`approver` in the constructor; `platform`, `arch`, `service`, `version`, `measurement`, `expires` in `register`) |
| `ExpiryAlreadyPassed(uint64 expires, uint64 nowTime)` | `expires` is not after the block time |
| `NotAnApprover(address approver)` | the recovered signer is not `APPROVER` |
| `AlreadyRegistered(bytes32 recordKey, uint64 expires)` | the record key already has a registration |

A malformed signature fails earlier, inside OpenZeppelin's `ECDSA.recover`, with its own errors.

### EIP-712 domain and type string

| part | value |
|---|---|
| name | `Registry` |
| version | `1` |
| chain id | the chain the contract is deployed on |
| verifying contract | the Registry's own address |

Type string: `Registration(string platform,string arch,string service,string version,bytes32 measurement,uint64 expires)`.
Because the domain carries the chain id and the contract address, a signature made for one deployment
is useless on another. The generated binding also exposes `Eip712Domain` (EIP-5267).

### How the Go side mirrors it

| Go | role |
|---|---|
| [`sign.go`](../internal/chain/register/auditor/ether/sign.go) | `Digest` builds the same typed data (same domain and type string) and `Sign` signs it, adding 27 to `v` as `ECDSA.recover` expects |
| [`registry.go`](../internal/chain/register/auditor/ether/registry.go) | abigen binding generated from the contract by `go generate` (see `generate.go` next to it); `RegistryRegistration` is the struct above |
| [`reader.go`](../internal/chain/lookup/user/ether/reader.go) | calls `RecordKeyFor` and `Lookup` and carries `ChainTime` into the result; the Auditor and Vendor have matching readers under `internal/chain/lookup/` |

### Worked example: deriving a record key

Take the approver address, the platform string `aws-nitro` and the 32-byte measurement of the image.
First hash the platform text with keccak256 to get 32 bytes. Then lay three 32-byte words side by
side: the approver address padded on the left with zeros, the platform hash, and the measurement.
The keccak256 of those 96 bytes is the record key. Change any one of the three and the key is
completely different; change the service, version, arch or expiry and it is unchanged, which is why
the same measurement can be registered only once per approver and platform.
