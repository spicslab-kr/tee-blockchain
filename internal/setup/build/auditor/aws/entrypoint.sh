#!/bin/sh
# 엔클레이브가 부팅되면 원래 이미지의 명령 대신 이 파일이 먼저 돈다.
# loopback 을 올리고 RA 응답기를 뒤에서 띄운 뒤, /att-app-cmd 에 한 줄씩 적힌 원래 명령을 그대로 실행한다.
# (원래 명령을 Docker CMD 로 넘기면 nitro-cli 가 스크립트 진입점과 같이 처리하지 못해 파일로 넘긴다.)
# Docker 는 실행 시 HOME 을 넣어 주지만 엔클레이브 init 은 넣지 않는다. 같은 기본값을 준다.
export HOME="${HOME:-/root}"
export PATH="${PATH:-/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin}"
/att-ra lo-up || echo "att: loopback up failed" >&2
/att-ra ra-serve --listen "vsock:${ATT_RA_PORT:-5000}" --extractor nsm &
set --
while IFS= read -r arg; do set -- "$@" "$arg"; done < /att-app-cmd
exec "$@"
