# launch — TEE 를 띄우고 RA 요청을 받을 수 있게 하기 (vendor)

## AWS Nitro

Vendor 가 준비할 것은 **Nitro Enclaves 를 켠 EC2 인스턴스**와 **소프트웨어의 Dockerfile** 이다. Dockerfile 은 고치지 않는다.

1. Nitro Enclaves 를 켜서 EC2 인스턴스를 만든다 (m5·c5·m6i 등 4 vCPU 이상, Amazon Linux 2023). 여기 들어간다.
2. `tb-vendor` 리눅스 바이너리와 소프트웨어 Dockerfile 디렉터리를 인스턴스에 올린다.
3. 인스턴스에서 두 명령. 설계서의 B 빌드 → A 실행 순서다.

       tb vendor build  --app-dir ./my-app
       tb vendor launch --eif out/my-app.eif

   - **build** (`internal/setup/build/Vendor/aws`): nitro-cli · docker 설치 → 소프트웨어 이미지 빌드 → 그 위에 T1 응답기(`ra-serve`)를
     얹어 EIF 빌드. `out/` 에 EIF 파일 · 측정값(PCR0·1·2)이 남는다. 자세한 것은 `build.md`.
   - **launch** (`internal/setup/launch/Vendor/aws`): 엔클레이브 실행 → 부모에 TCP→vsock 다리(`ra-proxy`) → 자기 자신에게 RA 요청해
     확인(`out/selfcheck.cbor`) → 보안 그룹에 TCP 5000 인바운드를 열어 본다. 인스턴스에 EC2 쓰기 권한(IAM 역할)이 있으면 그대로 열리고,
     없으면 AWS 계정에서 칠 `aws ec2 authorize-security-group-ingress ...` 한 줄을 찍어 준다. 메모리는 EIF 크기로 정해지며 `--memory` 로 바꾼다.
   - 남이 빌드한 EIF 를 받아 띄우기만 할 때(흐름 1 의 Vendor)는 `launch` 만 친다. 빌드만 하고 띄우지 않을 때(Auditor)는 `build` 만.

이제 누구나 `tb user ra-request --tee <공인 IP>:5000 --format aws-nitro` 로 RA 문서를 받는다.

- 엔클레이브 안에서는 소프트웨어가 `127.0.0.1` 에 붙을 수 있게 loopback 을 올려 주고, Docker 가 넣어 주던 `HOME` 도 넣어 준다.
- 소프트웨어는 원래 ENTRYPOINT · CMD 그대로 실행된다. 엔클레이브에는 네트워크 · 디스크가 없으니 그 안에서 되는 소프트웨어여야 한다.
- 인스턴스 생성과 AWS 자격증명은 Vendor 의 일이다. tb 는 인스턴스 안에서 docker · nitro-cli 를 부르고, 보안 그룹은 권한이 있을 때만 건드린다.
- 큰 파일이나 이미지를 Auditor 에게 넘길 때 S3 같은 저장소를 두면 편하다. 필수는 아니다.

## GCP Confidential Space · Azure Confidential VM

VM 자체에 네트워크가 있어 다리가 없다. VM 안에서 `tb-vendor ra-serve --listen tcp:0.0.0.0:5000 --extractor <플랫폼>` 만 띄우면 된다.
아직 실기기에서 확인하지 않았다.

## 다 띄운 뒤 설정값에 적을 것

| 플랫폼 | 값 |
|---|---|
| AWS Nitro | 인스턴스 ID · 리전 · AMI ID · RA 주소(공인 IP:포트) |
| GCP Confidential Space | 인스턴스 이름 · 프로젝트 ID · 컨테이너 이미지 · RA 주소 |
| Azure Confidential VM | VM 이름 · 리전 · MAA 엔드포인트 · RA 주소 |
