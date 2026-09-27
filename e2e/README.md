# e2e — end-to-end 실행 스크립트

역할별 폴더다. 파일 이름 끝이 **어디서 도는지**다: `*.nitro.sh` 는 그 역할의 Nitro 인스턴스 안에서(bash), `*.windows.ps1` 은 그 역할의
자기 Windows 컴퓨터(키가 있는 곳)에서(PowerShell). 값은 전부 `setup/env.windows.ps1` 한 곳에 있고, `.nitro.sh` 는 소스 레포·커밋·이미지 이름만 안다.

```
e2e/
  setup/                              누가 하든 한 번
    env.windows.ps1                   인스턴스 IP, pem, RPC, 키 파일, approver 주소, service/version
    upload.windows.ps1                바이너리 빌드 → 두 인스턴스에 올리기 (+ 각 역할의 .nitro.sh)
    stop.windows.ps1                  엔클레이브 · TCP 다리 내리기
  vendor/                             Vendor
    pack-image.nitro.sh               내 docker build → tb-vendor pack --image → launch     (이미지를 파일로 넘기는 흐름)
    build-source.nitro.sh             tb-vendor build --app-dir → launch                    (Auditor 가 소스를 다시 빌드하는 흐름)
    handoff.windows.ps1               out/nginx.eif · out/measurements.json → Auditor 의 from-vendor/
    self-register.windows.ps1         Auditor 없이 직접 등록 (흐름 2): 내 등록부 배포 → self-register → verify
  auditor/                            Auditor
    check-received-image.nitro.sh     tb-auditor launch --eif 받은것 --measurements 받은것  (재서 대조 → 같을 때만 띄움)
    rebuild-source.nitro.sh           tb-auditor build --app-dir (같은 커밋) → launch
    audit.windows.ps1                 내가 얻은 값 가져오기 → registry-deploy → tb-auditor audit (RA → 대조 → 등록 → 자기확인)
  user/                               User
    verify.windows.ps1                tb-user verify (엔클레이브 ↔ 체인). 키 없음, 아무 컴퓨터
```

## 순서

두 흐름의 차이는 Vendor 가 Auditor 에게 **이미지 파일**을 주느냐, **소스(레포·커밋)** 를 알려 주느냐다.

| | 이미지를 파일로 | 소스를 알려서 |
|---|---|---|
| 준비 | `.\e2e\setup\upload.windows.ps1` | 같음 |
| Vendor 인스턴스 | `./pack-image.nitro.sh` | `./build-source.nitro.sh` |
| Vendor 컴퓨터 | `.\e2e\vendor\handoff.windows.ps1` | — (레포·커밋만 알려 준다) |
| Auditor 인스턴스 | `./check-received-image.nitro.sh` | `./rebuild-source.nitro.sh` → PCR0·2 가 Vendor 와 같은지 확인 |
| Auditor 컴퓨터 | `.\e2e\auditor\audit.windows.ps1` | 같음 |
| User | `.\e2e\user\verify.windows.ps1` | 같음 |
| 정리 | `.\e2e\setup\stop.windows.ps1` | 같음 |

Auditor 없이 Vendor 혼자(흐름 2): Vendor 인스턴스 `./pack-image.nitro.sh` 또는 `./build-source.nitro.sh` → Vendor 컴퓨터 `.\e2e\vendor\self-register.windows.ps1`.

## 전제
- 인스턴스: Nitro Enclaves 켠 EC2 두 대, docker·git·nitro-cli 설치, `/etc/nitro_enclaves/allocator.yaml` 에 `cpu_count: 2 / memory_mib: 512`,
  보안그룹 Inbound TCP 5000. (`launch` 가 allocator 서비스는 켜 주고, 포트는 권한 없으면 안내만 한다.)
- 컴퓨터: Go, ssh/scp, `pem2.pem`, `deployer.key`(가스 · 흐름 2 의 Vendor 키), `auditor.key`(approver). Sepolia ETH.
- `.windows.ps1` 은 UTF-8 **BOM** 으로 저장돼 있어야 한다 — PowerShell 5.1 이 BOM 없는 UTF-8 을 CP949 로 읽어 한글 주석 다음 줄을 먹는다 (실측).

## 테스트마다 컨트랙트를 새로 까는 이유
같은 측정값을 같은 컨트랙트에 또 올리면 `AlreadyRegistered` 다. 운영에선 등록부 하나를 계속 쓰고 새 버전만 `audit` 으로 올린다.
`audit.windows.ps1 -Contract 0x…` 로 기존 등록부를 쓸 수 있다.
