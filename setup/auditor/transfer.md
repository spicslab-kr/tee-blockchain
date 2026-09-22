# transfer — 파일 전달 (auditor)

기본은 CLI. 채널은 Git 저장소다.

## 받기 — 빌드 설정값 + 소스 (시퀀스 1)

```
tb auditor receive --repo <Vendor 소스 저장소 URL> [--ref <commit>] --into <dir>
```

clone 하고 루트의 `build-config.json` 을 검증한다. `source.commit` 이 checkout 된 이력에 없으면 거절.
`<dir>` 에 Vendor 코드가 그대로 들어온다 — 이 디렉터리를 TEE 에 올려 빌드한다.

## 보내기 — 측정값 기록 + 이미지 (시퀀스 9)

```
tb auditor send --repo <올릴 저장소 URL> --record measurement-record.json --image svc.eif
```

기록 · 사이드카 해시 · 이미지 파일을 저장소 루트에 커밋 · push 한다. `image.sha256` 은 파일에서 계산해 채운다.
Vendor 가 이 저장소를 읽을 수 있어야 한다.

## 직접 하려면

- 받기: Vendor 저장소를 clone 하고 루트의 `build-config.json` 을 읽는다.
- 보내기: `measurement-record.json` 과 이미지 파일을 같은 디렉터리(저장소 루트)에 두고 커밋 · push. `image.path` 는 파일 이름만, `image.sha256` 은 그 파일의 SHA-256.

## 오류

| 종료 코드 | 뜻 | 할 일 |
|---|---|---|
| 3 | git 없음 | git 설치, PATH 확인 |
| 4 | 저장소 접근 불가 | URL · 권한 확인. 비공개면 Vendor 에게 권한 요청 |
| 5 | 커밋 없음 | `--ref` 확인 |
| 6 | push 거절 | 쓰기 권한 확인, 원격이 앞서 있으면 pull |
