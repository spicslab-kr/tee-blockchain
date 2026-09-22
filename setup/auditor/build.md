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
네트워크가 없어서, 제3자가 RA 를 요청하려면 응답기가 이미지 안에 있어야 한다. 빌드는 소프트웨어 이미지 위에 `tb-ra-responder`
바이너리와 진입점 한 장을 얹어 부팅 때 `ra-serve` 가 vsock 에서 듣게 한다. 응답기 구현은 `tee/request/auditor/aws` 에 있고
빌드는 그것을 얹기만 한다. 소프트웨어 이미지는 고치지 않는다.

**어느 바이너리를 얹느냐가 재현빌드의 전제다.** `tb vendor build`/`tb auditor build` 실행파일 자기 자신을 얹던 예전 방식은
역할마다 다른 CLI(`tb-vendor` vs `tb-auditor`, `cmd/vendor` vs `cmd/auditor`로 따로 컴파일)를 이미지에 넣는 셈이라, 이미지
내용물 자체가 역할마다 달라서 PCR0·PCR2 가 절대 같을 수 없었다(실측: 실제 EC2 두 대에서 Vendor·Auditor 가 같은 Dockerfile 을
각자 빌드해서 재현). `cmd/ra-responder` 를 따로 빌드해 `tb-vendor`/`tb-auditor` 옆에 `tb-ra-responder` 라는 이름으로 두면,
모든 역할이 정확히 같은 파일을 얹는다 — `Build.Binary` 를 비우면 실행파일 옆의 `tb-ra-responder` 를 찾고, 없으면 에러로
멈춘다(예전처럼 조용히 자기 자신으로 대체하지 않는다). 다른 경로에 두려면 `tb auditor build --responder <경로>` 로 지정한다.

**Layer normalization.** `docker build` reassembles layer contents fresh every run — the file ordering and
timestamps inside each layer differ between runs even when the final filesystem content is byte-identical.
nitro-cli measures off that assembly, so Vendor and Auditor building the same source independently got
different PCR0/PCR2 even with the same responder binary embedded (verified: reproduced on two separate
physical instances). After the T1 responder is layered on, before the EIF is built, the image is exported
(dropping layer boundaries), repacked deterministically (`tar --sort=name --mtime=@0 --owner=0 --group=0`),
and re-imported under the same tag.

Layer normalization and the shared responder binary are two separate requirements — either one missing
breaks reproducibility, and normalization alone was mistakenly believed sufficient before the shared-binary
requirement was found (an earlier version of this note claimed "exact match across two instances" from
normalization alone; that claim was never actually re-tested cross-role and turned out to be wrong once it
was).

**This only fixes "the packaging differs every time." `tb auditor build` does not check this automatically** —
if the source itself produces different content on each build (a tool that mints a random signing key,
unpinned package versions), normalization still won't make Vendor's and Auditor's builds match. That's not a
bug in normalization; it's a Dockerfile that isn't written reproducibly, and right now nothing surfaces it
until the build finishes (or Auditor compares). To keep a Dockerfile reproducible:
- Pin the base image by digest, not tag (`FROM img@sha256:...`)
- Pin package versions on install (`apt-get install pkg=1.2.3`), commit a lockfile for dependencies
- Avoid tools that generate and leave random state (keys, UUIDs) in the image during build
- Remove files like build logs that embed the literal build timestamp as text
