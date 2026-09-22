# transfer — 파일 전달 (vendor)

기본은 CLI. 채널은 Git 저장소다.

## 보내기 — 빌드 설정값 (시퀀스 1)

```
tb vendor send --repo <소스 저장소 URL> --config build-config.json
```

build-config.json 을 저장소 루트에 커밋 · push 한다. `source.commit` 은 그 직전 HEAD 로 채워진다.
Auditor 가 이 저장소를 읽을 수 있어야 한다 — 비공개면 Auditor 에게 읽기 권한(collaborator · deploy key)을 준다. 권한은 CLI 가 만들지 않는다.

## 받기 — 측정값 기록 + 이미지 (시퀀스 9)

```
tb vendor receive --repo <Auditor 저장소 URL> [--ref <commit>] --into <dir>
```

clone 하고 `measurement-record.json` · 사이드카 해시 · 이미지 파일 해시를 검증한다. 받은 원본은 그대로 두고 M2 기준값으로 쓴다.

## 직접 하려면

- 보내기: `build-config.json` 을 소스 저장소 **루트**에 두고 커밋 · push. `source.commit` 에는 소스 커밋 해시를 적는다.
- 받기: Auditor 저장소를 clone 하고 루트의 `measurement-record.json` 과 옆의 이미지 파일을 쓴다. `image.sha256` 과 파일 해시가 같아야 한다.

## 오류

| 종료 코드 | 뜻 | 할 일 |
|---|---|---|
| 3 | git 없음 | git 설치, PATH 확인 |
| 4 | 저장소 접근 불가 | URL · 권한 확인. 비공개면 권한 요청 |
| 5 | 커밋 없음 | `--ref` 확인 |
| 6 | push 거절 | 쓰기 권한 확인, 원격이 앞서 있으면 pull |
