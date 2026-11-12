#!/bin/bash
# 닫힌 VM이 부팅된 뒤 밖에서 코드를 바꿀 길이 있는지 찾아본다. 같은 VPC의 다른 VM에서 실행.
T=${1:?사용법: probe_closed.sh <닫힌 VM 주소>}
echo "== 1. TCP 포트 전체 (1-65535) 중 열린 것"
seq 1 65535 | xargs -P 400 -I{} sh -c "nc -z -w 2 $T {} 2>/dev/null && echo '  열림: {}'"
echo "== 2. SSH"
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=no "$T" true 2>&1 | sed 's/^/  /'
echo "== 3. Ollama 직접 (11434, 모델 받기·바꾸기 API가 있는 포트)"
curl -s -m 5 "http://$T:11434/api/tags" >/dev/null && echo "  열림" || echo "  연결 안 됨 (curl 종료 코드 $?)"
echo "== 4. 응답기(8443)로 Ollama 관리 API를 불러 보기"
for p in /api/pull /api/create /api/delete /api/copy /api/push /api/tags /api/generate /api/chat /api/blobs/sha256-0 \
         //api/pull /ask/../api/pull /ask/x /debug/pprof/ /admin /shell; do
  c=$(curl -s -o /dev/null -w '%{http_code}' -m 5 -X POST -d '{"name":"qwen2.5:0.5b"}' "http://$T:8443$p")
  echo "  POST $p -> $c"
done
c=$(curl -s -o /dev/null -w '%{http_code}' -m 5 "http://$T:8443/ask"); echo "  GET /ask -> $c"
