# e2e — end-to-end 실행 스크립트

역할별 폴더. 파일 이름 **앞의 숫자가 실행 순서**, **끝이 어디서 도는지**: `*.nitro.sh` 는 그 역할의 Nitro 인스턴스 안에서(bash, ssh 로 들어가 `~` 에서),
`*.windows.ps1` 은 그 역할의 자기 Windows PC(PowerShell, 키가 있는 곳, `TB-live` 에서). 값은 전부 `setup/env.windows.ps1` 한 곳에 있고,
`.nitro.sh` 는 소스 레포·커밋·이미지 이름만 안다. 원칙: 엔클레이브를 만들고 띄우는 일만 인스턴스에서, 전달·등록·검증은 PC 에서. 개인키는 인스턴스에 안 올린다.

```
e2e/
  setup/                                    누가 하든 한 번
    env.windows.ps1                         인스턴스 IP, pem, RPC, 키 파일, approver 주소, service/version
    0-upload.windows.ps1                    바이너리 빌드 → 두 인스턴스에 올리기 (+ 각 역할의 .nitro.sh). 떠 있던 엔클레이브는 내린다
    99-stop.windows.ps1                     엔클레이브 · TCP 다리 내리기
  vendor/                                   Vendor
    1-pack-image.nitro.sh                   내 docker build → tb-vendor pack --image → launch     (이미지를 파일로 넘기는 흐름)
    1-build-source.nitro.sh                 tb-vendor build --app-dir → launch                    (소스를 알려 주는 흐름)
    2-handoff.windows.ps1                   out/nginx.eif · out/measurements.json → Auditor 의 from-vendor/
    7-check-record.windows.ps1              Auditor 가 돌려준 기록 ↔ 내 엔클레이브 attestation ↔ 체인 등록
    8-self-register.windows.ps1             Auditor 없이 직접 등록 (흐름 2): 내 등록부 배포 → self-register → verify
    9a-demo-tampered-json.windows.ps1       조작 데모 1: measurements.json 을 바꿔 보내면 Auditor 3 이 거부한다
    9b-demo-swap-enclave.nitro.sh           조작 데모 2: 다른 이미지로 바꿔 띄우면 User 5 · Auditor 4 -CheckVendor 가 잡는다
  auditor/                                  Auditor
    3-check-received-image.nitro.sh         tb-auditor launch --eif 받은것 --measurements 받은것  (재서 대조 → 같을 때만 띄움)
    3-rebuild-source.nitro.sh               tb-auditor build --app-dir (같은 커밋) → launch
    4-audit.windows.ps1                     내가 얻은 값 가져오기 → registry-deploy → tb-auditor audit (RA → 대조 → [-CheckVendor] → 등록 → 자기확인)
    6-handback.windows.ps1                  감사 결과(측정값 기록 + 등록 좌표)를 Vendor 에게
  user/                                     User
    5-verify.windows.ps1                    tb-user verify (엔클레이브 ↔ 체인). 키 없음. 등록부 여러 개는 쉼표로
```

## 순서

| # | 어디서 | 이미지를 파일로 | 소스를 알려서 |
|---|---|---|---|
| 0 | PC | `.\e2e\setup\0-upload.windows.ps1` | 같음 |
| 1 | Vendor 인스턴스 | `./1-pack-image.nitro.sh` | `./1-build-source.nitro.sh` |
| 2 | Vendor PC | `.\e2e\vendor\2-handoff.windows.ps1` | — (레포·커밋만 알려 준다) |
| 3 | Auditor 인스턴스 | `./3-check-received-image.nitro.sh` | `./3-rebuild-source.nitro.sh` → PCR0·2 가 Vendor 와 같은지 확인 |
| 4 | Auditor PC | `.\e2e\auditor\4-audit.windows.ps1 [-CheckVendor]` | 같음 |
| 5 | User PC | `.\e2e\user\5-verify.windows.ps1` | 같음 |
| 6 | Auditor PC | `.\e2e\auditor\6-handback.windows.ps1` | 같음 |
| 7 | Vendor PC | `.\e2e\vendor\7-check-record.windows.ps1` | 같음 |
| 8 | Vendor PC | (Auditor 없는 흐름 2) `.\e2e\vendor\8-self-register.windows.ps1` — 1 뒤에 바로 | 같음 |
| 9 | Vendor | 조작 데모: `9a-demo-tampered-json.windows.ps1` (PC) · `./9b-demo-swap-enclave.nitro.sh` (인스턴스) → 그 뒤 `5-verify`, `4-audit -CheckVendor` | 같음 |
| 99 | PC | `.\e2e\setup\99-stop.windows.ps1` | 같음 |

설계 흐름 2 ②(Vendor 가 A 여야 하는데 B 를 띄움): `./9b-demo-swap-enclave.nitro.sh` → `8-self-register.windows.ps1 -RemoteOut out-tampered` →
`4-audit.windows.ps1 -CheckVendor` → `5-verify.windows.ps1 <Vendor등록부>,<Auditor등록부>` → `not trusted — only 1 of 2 registries vouch`.

## 전제
- 인스턴스: Nitro Enclaves 켠 EC2 두 대, docker·git·nitro-cli 설치, `/etc/nitro_enclaves/allocator.yaml` 에 `cpu_count: 2 / memory_mib: 512`,
  보안그룹 Inbound TCP 5000. (`launch` 가 allocator 서비스는 켜 주고, 포트는 권한 없으면 안내만 한다.)
- PC: Go, ssh/scp, `pem2.pem`, `deployer.key`(가스 · 흐름 2 의 Vendor 키), `auditor.key`(approver). Sepolia ETH.
- `.windows.ps1` 은 UTF-8 **BOM** 으로 저장돼 있어야 한다 — PowerShell 5.1 이 BOM 없는 UTF-8 을 CP949 로 읽어 한글 주석 다음 줄을 먹는다 (실측).

## 테스트마다 컨트랙트를 새로 까는 이유
같은 측정값을 같은 컨트랙트에 또 올리면 `AlreadyRegistered` 다. 운영에선 등록부 하나를 계속 쓰고 새 버전만 `audit` 으로 올린다.
`4-audit.windows.ps1 -Contract 0x…` 로 기존 등록부를 쓸 수 있다.
