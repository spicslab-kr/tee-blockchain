#!/bin/bash
# CloudShell에서 실행: AWS 관리자 권한으로 닫힌 VM 안에 들어가거나 명령을 넣을 수 있는지 시험한다.
# 필요한 환경 변수: INSTANCE(인스턴스 ID), REGION, TARGET(공인 IP). SG와 MYIP를 주면 끝에 임시 전체 포트 규칙을 닫는다.
# 마지막에 재부팅을 요청한다. 닫힌 VM은 종료 신호를 무시하므로 AWS가 약 4분 뒤 강제로 재부팅한다.
I=${INSTANCE:?INSTANCE 를 지정}; R=${REGION:?REGION 을 지정}; IP=${TARGET:?TARGET 을 지정}
echo "시작 $(date -u +%T) UTC"
rm -f /tmp/k /tmp/k.pub; ssh-keygen -t ed25519 -N '' -f /tmp/k -q
echo "## 1. EC2 Instance Connect (콘솔의 연결 버튼과 같은 방식: 키를 넣고 SSH)"
aws ec2-instance-connect send-ssh-public-key --region $R --instance-id $I --instance-os-user ec2-user --ssh-public-key file:///tmp/k.pub --query Success
ssh -i /tmp/k -o ConnectTimeout=8 -o StrictHostKeyChecking=no -o BatchMode=yes ec2-user@$IP true 2>&1 | tail -1
echo "## 2. SSM (Session Manager, Run Command)"
aws ssm describe-instance-information --region $R --filters Key=InstanceIds,Values=$I --query 'InstanceInformationList[].PingStatus'
aws ssm send-command --region $R --instance-ids $I --document-name AWS-RunShellScript --parameters commands=id 2>&1 | tail -1
echo "## 3. 시리얼 콘솔"
aws ec2 get-serial-console-access-status --region $R
aws ec2-instance-connect send-serial-console-ssh-public-key --region $R --instance-id $I --serial-port 0 --ssh-public-key file:///tmp/k.pub 2>&1 | tail -1
( sleep 6; printf '\r\nroot\r\nls /\r\necho HELLO_FROM_SERIAL\r\n'; sleep 6 ) | timeout 25 ssh -tt -i /tmp/k -o StrictHostKeyChecking=no $I.port0@serial-console.ec2-instance-connect.$R.aws 2>&1 | tr -d '\r' | tail -12
echo "## 4. 콘솔 출력 (읽기 전용) 마지막 줄"
aws ec2 get-console-output --region $R --instance-id $I --latest --output text | tail -8
rm -f /tmp/k /tmp/k.pub
echo "## 5. 끝 $(date -u +%T) UTC: 전체 포트 임시 규칙 닫고 재부팅 (닫힌 VM은 종료 신호를 무시해서 약 4분 뒤 강제 재부팅됨)"
[ -n "$SG" ] && [ -n "$MYIP" ] && aws ec2 revoke-security-group-ingress --region $R --group-id "$SG" --protocol tcp --port 0-65535 --cidr "$MYIP/32" --query Return
aws ec2 reboot-instances --region $R --instance-ids $I && echo "재부팅 요청함"
