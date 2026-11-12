#!/bin/bash
# GCP User VM에서: 닫힌 VM을 검증하고 밖에서 바꿀 길을 찾아본다.  사용법: gcp_suite.sh before|after
T=${TARGET:?TARGET 에 닫힌 VM의 내부 IP를 지정}; E=${EXPECTED:-expected-closed.json}; U=http://$T:8443
filt() { grep -vE '^\s*$'; }
if [ "$1" = before ]; then
  echo "########## 1. VM 안 상태 보고서 (증명 포함)"; ./snp-ask -url $U -expected $E -status 2>&1 | filt
  echo "########## 2. 정상 질문";  ./snp-ask -url $U -expected $E -save gcp-answer.json 2>&1 | grep -E '^질문|^답|실패|판정'
  echo "########## 3. 중간 변조"; ./snp-ask -url $U -expected $E -tamper 2>&1 | grep -E '바꿔서|실패|판정'
  echo "########## 4. 재사용";   ./snp-ask -url $U -expected $E -replay gcp-answer.json 2>&1 | grep -E '실패|판정'
  echo "########## 5. 밖에서 찾아보기"; bash probe_closed.sh $T 2>&1
else
  echo "########## 다시 상태 보고서"; ./snp-ask -url $U -expected $E -status 2>&1 | filt
  echo "########## 다시 질문"; ./snp-ask -url $U -expected $E 2>&1 | grep -E '^답|실패|판정'
fi
