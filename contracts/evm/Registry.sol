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
    struct Registration {
        string platform; // measurement format, e.g. aws-nitro
        string arch; // CPU architecture the image was built for, e.g. amd64 / arm64; informational, not in the record key
        string service;
        string version;
        bytes32 measurement;
        uint64 expires; // unix seconds; required, must be in the future
    }

    struct RegistrationInfo {
        address approver;
        uint64 expires;
        uint64 registeredAt;
    }

    /// recordKey = keccak256(approver, platform, measurement). The approver is part of the hash, but here it is
    /// always APPROVER, so there is effectively one registration per (platform, measurement).
    mapping(bytes32 => RegistrationInfo) public registrations;

    event Registered(
        bytes32 indexed recordKey,
        address indexed approver,
        address registrant,
        string platform,
        string arch,
        string service,
        string version,
        bytes32 measurement,
        uint64 expires,
        bytes signature
    );

    error NotAnApprover(address approver);
    error AlreadyRegistered(bytes32 recordKey, uint64 expires);
    error MissingField(string field);
    error ExpiryAlreadyPassed(uint64 expires, uint64 nowTime);

    bytes32 internal constant REGISTRATION_TYPEHASH = keccak256(
        "Registration(string platform,string arch,string service,string version,bytes32 measurement,uint64 expires)"
    );

    constructor(address approver) EIP712("Registry", "1") {
        if (approver == address(0)) revert MissingField("approver");
        APPROVER = approver;
    }

    /// @notice Registers a signed registration. Recovers the approver and requires it to be APPROVER.
    function register(Registration calldata e, bytes calldata signature) external returns (bytes32 recordKey) {
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

        address approver = ECDSA.recover(digest(e), signature);
        if (approver != APPROVER) revert NotAnApprover(approver);

        recordKey = recordKeyOf(approver, e.platform, e.measurement);
        if (registrations[recordKey].expires != 0) {
            revert AlreadyRegistered(recordKey, registrations[recordKey].expires);
        }
        registrations[recordKey] = RegistrationInfo({approver: approver, expires: e.expires, registeredAt: nowTime});
        _announce(recordKey, e, signature);
    }

    // Emitting directly in register would overflow the stack because the event has many arguments, so it lives in
    // a separate function that takes its arguments in memory: a calldata string takes two stack slots (offset,
    // length), a memory string one pointer slot, which is what lets the event with `arch` fit. The approver was
    // already checked to be APPROVER in register, so it is not passed again.
    function _announce(bytes32 recordKey, Registration memory e, bytes memory signature) private {
        emit Registered(
            recordKey, APPROVER, msg.sender, e.platform, e.arch, e.service, e.version, e.measurement, e.expires, signature
        );
    }

    /// @notice The EIP-712 hash the approver signs. Used to check that the Go implementation produces the same value.
    function digest(Registration calldata e) public view returns (bytes32) {
        return _hashTypedDataV4(
            keccak256(
                abi.encode(
                    REGISTRATION_TYPEHASH,
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

    function recordKeyOf(address approver, string calldata platform, bytes32 measurement)
        public
        pure
        returns (bytes32)
    {
        return keccak256(abi.encode(approver, keccak256(bytes(platform)), measurement));
    }

    /// @notice The record key of a measurement in this contract: recordKeyOf(APPROVER, platform, measurement).
    function recordKeyFor(string calldata platform, bytes32 measurement) external view returns (bytes32) {
        return recordKeyOf(APPROVER, platform, measurement);
    }

    /// @notice The registration under a record key plus the chain's current time. Expiry is judged against that time.
    function lookup(bytes32 recordKey)
        external
        view
        returns (address approver, uint64 expires, uint64 registeredAt, uint64 chainTime)
    {
        RegistrationInfo storage r = registrations[recordKey];
        // forge-lint: disable-next-line(unsafe-typecast)
        return (r.approver, r.expires, r.registeredAt, uint64(block.timestamp));
    }
}
