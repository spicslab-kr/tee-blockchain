# chain — 체인에 기준값을 올리기 전에 준비할 것 (auditor)

## Ethereum

### 한 번만

1. **내 서명 키의 주소.** 컨트랙트 하나는 서명자 **하나**의 서명이 붙은 등록만 받는다. Auditor 는 자기 주소로 자기 컨트랙트를
   올린다 (Vendor 가 등록하는 흐름은 Vendor 가 따로 자기 컨트랙트를 올린다 — 조회하는 쪽은 컨트랙트 주소를 각각 안다).
   키는 나만 갖고, 조회하는 쪽에는 컨트랙트 주소만 알려준다. 키 파일은 32 바이트 hex 한 줄. 이더리움 지갑 도구 아무것으로나 만든다.
2. **RPC 주소**와 **가스를 낼 계정**. 테스트넷(Sepolia)이면 수도꼭지에서 받는다.
3. **컨트랙트 배포.** 트랜잭션은 누가 보내도 된다. 서명자 주소는 배포 뒤 바꿀 수 없다. 바꾸려면 새로 배포하고 주소를 다시 알린다.

       tb auditor registry-deploy --rpc <url> --key-file deployer.key --approver 0x…

   나온 컨트랙트 주소를 Vendor · User 와 공유한다. User 는 이 주소만 알면 조회한다.

### 릴리스마다

빌드가 끝나 측정값(`out/measurements.json`)이 나오면:

    tb auditor register --rpc <url> --contract 0x… --key-file auditor.key \
      --measurements out/measurements.json --service my-app --version 1.2.0 --expires 2160h

- 옵션을 빼고 `tb auditor register` 만 치면 값을 하나씩 물어본다. 엔터는 기본값.
- 내 키로 서명하고 같은 키로 트랜잭션을 보낸다. 컨트랙트가 서명자를 복원해 배포 때 정한 서명자(APPROVER)인지 확인한다.
- 결과의 `key` 가 조회 좌표다. 같은 측정값을 Vendor 도 등록하면 Vendor 의 컨트랙트에 다른 key 로 생긴다.
- `--expires` 가 지나면 등록은 효력을 잃는다. 취소는 없다. 새 릴리스는 새 측정값이라 새 key 다.
- 종료코드: 3 RPC 연결 실패 · 4 서명 키가 승인자가 아님 · 5 이미 등록됨 · 6 만료 시각이 과거.

### 노드 없이 시험하기

    go test ./internal/chain/register/...

go-ethereum 의 시뮬레이션 체인에 Auditor · Vendor 컨트랙트를 하나씩 배포하고 각자 등록 · 조회하는 것까지 돈다. 실제 노드나 지갑이 필요 없다.

### 컨트랙트를 고쳤을 때

`contracts/evm/Registry.sol` 이 원본이다. forge(solc 0.8.28)로 컴파일하고 abigen 으로 Go 바인딩(`registry.go`)을 다시 만든다.
명령은 각 역할의 `ether/generate.go` 에 있다 (`go generate ./internal/chain/register/...`) — forge 는 `~/.foundry/bin`,
abigen 은 `go run github.com/ethereum/go-ethereum/cmd/abigen` 이라 따로 설치할 것이 없다. OpenZeppelin 은 서브모듈이라
처음 클론 뒤 `git submodule update --init` 이 필요하다. 두 역할의 `registry.go` 는 같은 파일이어야 한다.

## 실제로 올려 본 것 — Sepolia (2026-09-22)

| | |
|---|---|
| 컨트랙트 | `0x1C7361b6Fc9D4e34D9C44201B811C19764E7C179` (https://sepolia.etherscan.io/address/0x1C7361b6Fc9D4e34D9C44201B811C19764E7C179) |
| Auditor | `0xF9AA9e4b4317872a5Be2A643c66E45F2a6B345a2` |
| Vendor | `0x771a04FA3A5d1da41cAb340421F5A2989Ad52424` |
| 등록한 값 | Nitro 에서 실제로 빌드한 nginx 엔클레이브의 PCR0·1·2 (`aws-nitro`, my-web 1.27) |
| Vendor 등록 | key `0x30240a6d…`, tx `0x926c4ad5…`, block 11752020 |
| Auditor 등록 | key `0xacb600af…`, tx `0x85caf2c6…`, block 11752021 |
| Auditor, ollama 엔클레이브 측정값 | key `0x4d2dbf7a…`, tx `0x538d158b…`, block 11752122 |
| 두 번째 배포 (abigen 바인딩으로) | `0x6256dccba6864fD627116571498f889d478cf4d0`, tx `0x55849a96…` |

같은 측정값을 두 승인자가 각자 등록해 키 둘이 생겼고, 중복 등록은 거부됐다. (이 기록은 승인자 둘을 한 컨트랙트에 받던 이전
컨트랙트다 — 지금 컨트랙트는 서명자 하나라 `keysOf` 가 없고, 역할마다 컨트랙트를 따로 올린다.)
테스트용 지갑이라 키는 `~/.tb/` 에만 있다.

## 다른 체인

`chain/register/<역할>/` 아래 폴더를 하나 더 만든다. 컨트랙트가 EVM 이면 `ether` 를 RPC 주소만 바꿔 그대로 쓴다.
