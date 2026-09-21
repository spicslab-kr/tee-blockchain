# build — 빌드가 끝난 뒤 측정값 기록에 적을 것 (auditor)

| 플랫폼 | 측정값 기록에 넣을 값 |
|---|---|
| AWS Nitro | 이미지 파일(EIF) · PCR0·1·2 · 쓴 빌드 도구와 버전 |
| GCP Confidential Space | 컨테이너 이미지 digest(태그 아님) · 쓴 빌드 도구 |
| Azure Confidential VM | digest(컨테이너) 또는 이미지 ID(VM) · 쓴 빌드 도구 |

