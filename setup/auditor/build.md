# build — 빌드가 끝난 뒤 측정값 기록에 적을 것 (auditor)

| 플랫폼 | 측정값 기록에 넣을 값 |
|---|---|
| AWS Nitro | 이미지 파일(EIF) · PCR0·1·2 · 쓴 빌드 도구와 버전 |
| GCP Confidential Space | 컨테이너 이미지 digest(태그 아님) · 쓴 빌드 도구 |
| Azure Confidential VM | digest(컨테이너) 또는 이미지 ID(VM) · 쓴 빌드 도구 |

## 자동화 (Case 2) — `tb auditor build` · AWS Nitro

`internal/setup/build/auditor/aws`. 소프트웨어의 Dockerfile 을 받아 위 표의 값(EIF · PCR0·1·2)을 `out/` 에 낸다.
도구(docker · nitro-cli)는 라이브러리가 아니라 exec 로 부른다.

설계서 B 빌드에 없던 한 단계가 여기 들어간다 — **T1 응답기 얹기**. Nitro 엔클레이브는 안에서만 `/dev/nsm` 을 열 수 있고
네트워크가 없어서, 제3자가 RA 를 요청하려면 응답기가 이미지 안에 있어야 한다. 빌드는 소프트웨어 이미지 위에 tb 바이너리와
진입점 한 장을 얹어 부팅 때 `ra-serve` 가 vsock 에서 듣게 한다. 응답기 구현은 `tee/request/auditor/aws` 에 그대로 있고
빌드는 그것을 얹기만 한다. 소프트웨어 이미지는 고치지 않는다.
