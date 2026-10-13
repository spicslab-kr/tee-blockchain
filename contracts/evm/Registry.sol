// SPDX-License-Identifier: MIT
pragma solidity ^0.8.28;

import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {EIP712} from "@openzeppelin/contracts/utils/cryptography/EIP712.sol";

/// @title Registry: reference values published on chain.
/// @notice Records who vouches for the measurement of which TEE image, and until when.
///         A registration is accepted only with a signature from the single approver (APPROVER) fixed at
///         deployment: one contract, one approver. If an auditor and a vendor both register, each deploys
///         its own contract and is looked up separately.
///         Any account may submit the transaction; the signature, not the sender, authorizes it.
contract Registry is EIP712 {
    address public immutable APPROVER;

    /// The signed value. The measurements are sorted by name and hashed with keccak256 into the 32-byte `measurement`.
    struct Entry {
        string platform; // measurement format, e.g. aws-nitro
        string arch; // CPU architecture the image was built for, e.g. amd64 / arm64; informational, not part of the key
        string service;
        string version;
        bytes32 measurement;
        uint64 expires; // unix seconds; required, must be in the future
    }

    struct Record {
        address signer;
        uint64 expires;
        uint64 registeredAt;
    }

    /// key = keccak256(signer, platform, measurement). The signer is part of the key hash, but here it is always
    /// APPROVER, so there is effectively one record per (platform, measurement).
    mapping(bytes32 => Record) public records;

    event Registered(
        bytes32 indexed key,
        address indexed signer,
        address registrant,
        string platform,
        string arch,
        string service,
        string version,
        bytes32 measurement,
        uint64 expires,
        bytes signature
    );

    error NotAnApprover(address signer);
    error AlreadyRegistered(bytes32 key, uint64 expires);
    error MissingField(string field);
    error ExpiryAlreadyPassed(uint64 expires, uint64 nowTime);

    bytes32 internal constant ENTRY_TYPEHASH = keccak256(
        "Entry(string platform,string arch,string service,string version,bytes32 measurement,uint64 expires)"
    );

    constructor(address approver) EIP712("Registry", "1") {
        if (approver == address(0)) revert MissingField("approver");
        APPROVER = approver;
    }

    /// @notice Registers a signed entry. Recovers the signer and requires it to be APPROVER.
    function register(Entry calldata e, bytes calldata signature) external returns (bytes32 key) {
        if (bytes(e.platform).length == 0) revert MissingField("platform");
        if (bytes(e.arch).length == 0) revert MissingField("arch");
        if (bytes(e.service).length == 0) revert MissingField("service");
        if (bytes(e.version).length == 0) revert MissingField("version");
        if (e.measurement == bytes32(0)) revert MissingField("measurement");
        // forge-lint: disable-next-line(unsafe-typecast)
        uint64 nowTime = uint64(block.timestamp);
        if (e.expires == 0) revert MissingField("expires");
        // forge-lint: disable-next-line(block-timestamp)
        if (e.expires <= nowTime) revert ExpiryAlreadyPassed(e.expires, nowTime);

        address signer = ECDSA.recover(digest(e), signature);
        if (signer != APPROVER) revert NotAnApprover(signer);

        key = keyOf(signer, e.platform, e.measurement);
        if (records[key].expires != 0) revert AlreadyRegistered(key, records[key].expires);
        records[key] = Record({signer: signer, expires: e.expires, registeredAt: nowTime});
        _announce(key, e, signature);
    }

    // Emitting directly in register would overflow the stack because the event has many arguments, so it lives in
    // a separate function that takes its arguments in memory: a calldata string takes two stack slots (offset,
    // length), a memory string one pointer slot, which is what lets the event with `arch` fit. The signer was
    // already checked to be APPROVER in register, so it is not passed again.
    function _announce(bytes32 key, Entry memory e, bytes memory signature) private {
        emit Registered(
            key, APPROVER, msg.sender, e.platform, e.arch, e.service, e.version, e.measurement, e.expires, signature
        );
    }

    /// @notice The EIP-712 hash the approver signs. Used to check that the Go implementation produces the same value.
    function digest(Entry calldata e) public view returns (bytes32) {
        return _hashTypedDataV4(
            keccak256(
                abi.encode(
                    ENTRY_TYPEHASH,
                    keccak256(bytes(e.platform)),
                    keccak256(bytes(e.arch)),
                    keccak256(bytes(e.service)),
                    keccak256(bytes(e.version)),
                    e.measurement,
                    e.expires
                )
            )
        );
    }

    function keyOf(address signer, string calldata platform, bytes32 measurement)
        public
        pure
        returns (bytes32)
    {
        return keccak256(abi.encode(signer, keccak256(bytes(platform)), measurement));
    }

    /// @notice The key of a measurement in this contract: keyOf(APPROVER, platform, measurement).
    function keyFor(string calldata platform, bytes32 measurement) external view returns (bytes32) {
        return keyOf(APPROVER, platform, measurement);
    }

    /// @notice The record under a key plus the chain's current time. Expiry is judged against that time.
    function lookup(bytes32 key)
        external
        view
        returns (address signer, uint64 expires, uint64 registeredAt, uint64 chainTime)
    {
        Record storage r = records[key];
        // forge-lint: disable-next-line(unsafe-typecast)
        return (r.signer, r.expires, r.registeredAt, uint64(block.timestamp));
    }
}
