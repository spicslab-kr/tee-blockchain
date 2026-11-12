#!/bin/bash
# 닫힌 AWS VM의 서명된 상태 보고서를 20초마다 기록한다. VM이 2분 넘게 응답하지 않으면 끝낸다.
cd "$(dirname "$0")"; down=0; : > monitor.log
T=${TARGET:?TARGET 에 닫힌 VM의 공인 IP를 지정}; ASK=./snp-ask-aws; [ -x ./snp-ask-aws.exe ] && ASK=./snp-ask-aws.exe
for i in $(seq 1 240); do
  t=$(date -u +%T)
  out=$($ASK -url http://$T:8443 -expected expected-aws.json -status 2>&1)
  if echo "$out" | grep -q '판정: 아래 상태'; then
    down=0
    procs=$(echo "$out" | grep -E '^  pid' | sed -E 's/ +/ /g' | tr '\n' ';')
    ports=$(echo "$out" | grep -E '^  (tcp|udp|raw)' | tr -d ' ' | tr '\n' ' ')
    echo "$t UTC 승인 | $procs | $ports" >> monitor.log
  else
    down=$((down+1)); echo "$t UTC 응답 없음 또는 거부: $(echo "$out" | tail -1 | cut -c1-80)" >> monitor.log
    [ $down -ge 6 ] && break
  fi
  sleep 20
done
echo "끝 $(date -u +%T) UTC" >> monitor.log
