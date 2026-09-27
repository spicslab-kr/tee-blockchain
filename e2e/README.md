# e2e — end-to-end 실행 스크립트

역할(Vendor · Auditor · User)과 **어디서 도는지**(Nitro 인스턴스 / 노트북)로 나눴다. 값은 전부 `env.ps1` 에 있고,
인스턴스 쪽 스크립트는 소스 레포·커밋·이름만 안다.

```
e2e/
  env.ps1                       노트북 값: 인스턴스 IP, pem, RPC, 키 파일, approver 주소, service/version
  laptop/                       노트북(PowerShell)에서
    00-upload.ps1               바이너리 빌드 → 두 인스턴스에 올리기 (+ 아래 .sh 들)
    10-handoff-case-a.ps1       Case A: Vendor 의 out/nginx.eif · measurements.json → Auditor 의 from-vendor/
    20-audit.ps1                Auditor 의 값 가져오기 → registry-deploy → tb-auditor audit (검사 → 등록 → 자기확인)
    30-verify.ps1               tb-user verify (User: Vendor TEE ↔ 체인)
    40-flow2-self-register.ps1  흐름 2: Vendor 등록부 배포 → tb-vendor self-register → verify
    99-stop.ps1                 엔클레이브 · 다리 내리기
  vendor/                       Vendor 인스턴스(bash, ~ 에서)
    case-a.sh                   docker build (자기 방식) → tb-vendor pack --image → launch
    case-b.sh                   tb-vendor build --app-dir → launch
  auditor/                      Auditor 인스턴스(bash, ~ 에서)
    case-a.sh                   tb-auditor launch --eif 받은것 --measurements 받은것   (재서 대조 → 같을 때만 띄움)
    case-b.sh                   tb-auditor build --app-dir (같은 커밋) → launch
```

## 순서

| | Case A (Vendor 가 EIF 파일을 넘김) | Case B (Vendor 가 소스·커밋을 알림) |
|---|---|---|
| 노트북 | `.\e2e\laptop\00-upload.ps1` | 같음 |
| Vendor | `./case-a.sh` | `./case-b.sh` |
| 노트북 | `.\e2e\laptop\10-handoff-case-a.ps1` | — |
| Auditor | `./case-a.sh` | `./case-b.sh` → PCR0·2 가 Vendor 와 같은지 눈으로 확인 |
| 노트북 | `.\e2e\laptop\20-audit.ps1` | 같음 |
| 노트북 | `.\e2e\laptop\30-verify.ps1` | 같음 |
| 노트북 | `.\e2e\laptop\99-stop.ps1` | 같음 |

흐름 2(Auditor 없음): Vendor `./case-a.sh` 또는 `./case-b.sh` → 노트북 `.\e2e\laptop\40-flow2-self-register.ps1`.

## 전제
- 인스턴스: Nitro Enclaves 켠 EC2 두 대, docker·git·nitro-cli 설치, `/etc/nitro_enclaves/allocator.yaml` 에 `cpu_count: 2 / memory_mib: 512`,
  보안그룹 Inbound TCP 5000. (`launch` 가 allocator 서비스는 켜 주고, 포트는 권한 없으면 안내만 한다.)
- 노트북: Go, ssh/scp, `pem2.pem`, `deployer.key`(가스), `auditor.key`(approver). Sepolia ETH.
- `.ps1` 은 UTF-8 **BOM** 으로 저장돼 있어야 한다 — PowerShell 5.1 이 BOM 없는 UTF-8 을 CP949 로 읽어 한글 주석 다음 줄을 먹는다 (실측).

## 테스트마다 컨트랙트를 새로 까는 이유
같은 측정값을 같은 컨트랙트에 또 올리면 `AlreadyRegistered` 다. 운영에선 등록부 하나를 계속 쓰고 새 버전만 `audit` 으로 올린다.
`20-audit.ps1 -Contract 0x…` 로 기존 등록부를 쓸 수 있다.
