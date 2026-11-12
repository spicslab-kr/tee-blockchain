#!/bin/bash
# PC(User)에서: 닫힌 AWS VM을 검증하고 밖에서 바꿀 길을 찾아본다.  사용법: aws_suite.sh before|after
cd "$(dirname "$0")"
T=${TARGET:?TARGET 에 닫힌 VM의 공인 IP를 지정}; E=${EXPECTED:-expected-aws.json}; U=http://$T:8443
ASK=./snp-ask-aws; [ -x ./snp-ask-aws.exe ] && ASK=./snp-ask-aws.exe
filt() { grep -vE '^\s*$'; }
if [ "$1" = before ]; then
  echo "########## 1. VM 안 상태 보고서 (증명 포함)"; $ASK -url $U -expected $E -status 2>&1 | filt
  echo "########## 2. 정상 질문"; $ASK -url $U -expected $E -save aws-answer.json 2>&1 | grep -E '^질문|^답|실패|판정'
  echo "########## 3. 중간 변조"; $ASK -url $U -expected $E -tamper 2>&1 | grep -E '바꿔서|실패|판정'
  echo "########## 4. 재사용";   $ASK -url $U -expected $E -replay aws-answer.json 2>&1 | grep -E '실패|판정'
  echo "########## 5. TCP 포트 전체"; "${NMAP:-nmap}" -Pn -p- -T4 --open $T 2>&1 | grep -E '^[0-9]+/tcp|Not shown|scanned'
  echo "########## 6. SSH"; ssh -o ConnectTimeout=8 -o BatchMode=yes -o StrictHostKeyChecking=no ec2-user@$T true 2>&1 | tail -1
  echo "########## 7. 8443으로 Ollama 관리 API"
  for p in /api/pull /api/create /api/delete /api/copy /api/push /api/tags /api/generate /api/chat //api/pull /ask/../api/pull /debug/pprof/ /shell; do
    echo "  POST $p -> $(curl -s -o /dev/null -w '%{http_code}' -m 5 -X POST -d '{"name":"qwen2.5:0.5b"}' $U$p)"
  done
else
  echo "########## 다시 상태 보고서"; $ASK -url $U -expected $E -status 2>&1 | filt
  echo "########## 다시 질문"; $ASK -url $U -expected $E 2>&1 | grep -E '^답|실패|판정'
fi
