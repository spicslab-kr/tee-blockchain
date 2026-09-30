// SPDX-License-Identifier: MIT
pragma solidity ^0.8.28;

import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {EIP712} from "@openzeppelin/contracts/utils/cryptography/EIP712.sol";

/// @title Registry — 체인에 올라가는 기준값.
/// @notice 누가 어떤 TEE 이미지의 측정값을 언제까지 보증하는지 적는다.
///         등록은 배포 때 정한 서명자(APPROVER) 한 명의 서명이 있어야만 받는다. 컨트랙트 하나 = 서명자 하나.
///         Auditor 와 Vendor 가 둘 다 등록하는 흐름은 각자 자기 컨트랙트를 배포해 따로 조회한다.
///         트랜잭션을 보내는 계정은 누구여도 된다. 서명자가 곧 등록자다.
contract Registry is EIP712 {
    address public immutable APPROVER;

    /// 서명되는 값. 측정값은 이름순으로 묶어 keccak256 한 32바이트(measurement)로 들어간다.
    struct Entry {
        string platform; // 측정값 형식. 예: aws-nitro
        string arch; // 이미지를 빌드한 CPU 아키텍처. 예: amd64 / arm64 — 표시용, 키에는 안 들어간다
        string service;
        string version;
        bytes32 measurement;
        uint64 expires; // unix 초. 필수, 미래
    }

    struct Record {
        address signer;
        uint64 expires;
        uint64 registeredAt;
    }

    /// key = keccak256(signer, platform, measurement). signer 는 이 컨트랙트에선 항상 APPROVER 라, 사실상
    /// (platform, measurement) 하나에 기록 하나다. keyOf 에 signer 를 남겨 둔 건 다른 컨트랙트의 키와 모양을 맞추기 위해서.
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

    /// @notice 서명된 값을 등록한다. 서명자를 복원해 APPROVER 인지 본다.
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

    // 이벤트 인자가 많아 register 안에서 바로 emit 하면 스택이 넘친다. 따로 빼고, 인자는 memory 로 받는다 —
    // calldata 문자열은 (offset, length) 두 슬롯, memory 문자열은 포인터 한 슬롯이라 arch 까지 실은 이벤트가
    // 그래야 스택에 들어간다. signer 는 위에서 APPROVER 임을 확인했으니 다시 넘기지 않는다.
    function _announce(bytes32 key, Entry memory e, bytes memory signature) private {
        emit Registered(
            key, APPROVER, msg.sender, e.platform, e.arch, e.service, e.version, e.measurement, e.expires, signature
        );
    }

    /// @notice 서명자가 서명하는 EIP-712 해시. Go 쪽 구현이 같은 값을 내는지 맞춰 보는 데 쓴다.
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

    /// @notice 이 컨트랙트에서 측정값 하나가 갖는 키 — keyOf(APPROVER, platform, measurement).
    function keyFor(string calldata platform, bytes32 measurement) external view returns (bytes32) {
        return keyOf(APPROVER, platform, measurement);
    }

    /// @notice 키 아래 기록과 체인의 현재 시각. 만료 판정은 이 시각으로 한다.
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
