#!/usr/bin/env bash
# SEV-SNP is done on GCP; this script records the AWS path (SEV-SNP is an instance
# option, the vTPM is an AMI attribute) and is kept for reference, not used.
# (Decided 2026-11-11: AWS is used for Nitro Enclaves only. See docs/status/cvm-port.md.)
#
# AWS SEV-SNP with a vTPM. This takes three steps, because SEV-SNP is an instance option but the
# vTPM (NitroTPM) is an AMI attribute, so the stock AMI has to be re-registered with TPM support.
#
#   bash e2e/cloud/aws-snp.sh base     # 1. launch a plain SEV-SNP instance from the stock AMI
#   bash e2e/cloud/aws-snp.sh mkami    # 2. stop it, snapshot it, re-register with TPM support
#   bash e2e/cloud/aws-snp.sh vm       # 3. launch the real instance from that AMI, SEV-SNP on
#   bash e2e/cloud/aws-snp.sh status | stop | start | delete
#
# Verified against the AWS docs (2026-11):
#   - SEV-SNP is turned on at launch with --cpu-options AmdSevSnp=enabled and
#     "You can't enable AMD SEV-SNP after launch"
#   - on shared tenancy it is available only in US East (Ohio, us-east-2) and
#     Europe (Ireland, eu-west-1). Any commercial Region works on a Dedicated Host
#   - supported families for shared tenancy: c6a.*, m6a.*, r6a.*
#
# Note for the parser: AWS signs SEV-SNP reports with a VLEK, so the AMD chain comes from
#   https://kdsintf.amd.com/vlek/v1/Milan/cert_chain  — not the VCEK path.
set -uo pipefail

REGION=${REGION:-us-east-2}         # or eu-west-1; nothing else on shared tenancy
TYPE=${TYPE:-m6a.large}
KEY_NAME=${KEY_NAME:-attestamp-e2e}
SG_NAME=${SG_NAME:-attestamp-snp}
TAG=${TAG:-attestamp-snp}
AMI_NAME=${AMI_NAME:-attestamp-tpm}
PEM_OUT=${PEM_OUT:-snp.pem}
RA_PORT=${RA_PORT:-5000}

here="$(cd "$(dirname "$0")" && pwd)"; cd "$here/../.."
die() { echo "error: $*" >&2; exit 1; }
command -v aws >/dev/null 2>&1 || die "aws CLI not found."
# On Windows (Git Bash) the aws CLI ends every output line with a carriage return, so a value captured with $(...)
#   keeps a trailing CR and the next command rejects it (the same as gcloud's in gcp-cvm.sh `stop`, 2026-11-15). Strip it here, once,
#   for every call in this script; the exit status is aws's own. --region is here too: this is the only aws() wrapper,
#   so every call goes to $REGION and not to the profile's default region.
aws() { command aws --region "$REGION" "$@" | tr -d '\r'; return "${PIPESTATUS[0]}"; }
aws sts get-caller-identity >/dev/null 2>&1 || die "aws CLI is not authenticated. Run 'aws configure'."
case "$REGION" in us-east-2|eu-west-1) ;; *) die "SEV-SNP shared tenancy is only in us-east-2 and eu-west-1 (got $REGION)";; esac

ubuntu_ami() {
  aws ec2 describe-images --owners 099720109477 \
    --filters "Name=name,Values=ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*" \
              "Name=state,Values=available" \
    --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text
}
id_of() { aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" "Name=tag:Role,Values=$1" \
            "Name=instance-state-name,Values=pending,running,stopping,stopped" \
            --query 'Reservations[].Instances[0].InstanceId' --output text; }
ensure_sg() {
  SG=$(aws ec2 describe-security-groups --filters "Name=group-name,Values=$SG_NAME" --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null)
  if [ "$SG" = "None" ] || [ -z "$SG" ]; then
    SG=$(aws ec2 create-security-group --group-name "$SG_NAME" --description "attestamp SEV-SNP: ssh and RA" --query GroupId --output text) || die "create-security-group"
    MYIP=$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]'); [ -n "$MYIP" ] || die "cannot determine your public IP"
    aws ec2 authorize-security-group-ingress --group-id "$SG" --protocol tcp --port 22         --cidr "$MYIP/32" >/dev/null
    aws ec2 authorize-security-group-ingress --group-id "$SG" --protocol tcp --port "$RA_PORT" --cidr "$MYIP/32" >/dev/null
  fi
  echo "$SG"
}
launch() { # launch <role> <ami>
  SG=$(ensure_sg)
  # MSYS_NO_PATHCONV=1: Git Bash would turn DeviceName=/dev/sda1 into a Windows path (see aws-nitro.sh)
  MSYS_NO_PATHCONV=1 aws ec2 run-instances --image-id "$2" --count 1 --instance-type "$TYPE" \
    --key-name "$KEY_NAME" --security-group-ids "$SG" \
    --cpu-options AmdSevSnp=enabled \
    --block-device-mappings 'DeviceName=/dev/sda1,Ebs={VolumeSize=32,VolumeType=gp3}' \
    --tag-specifications "ResourceType=instance,Tags=[{Key=Project,Value=$TAG},{Key=Role,Value=$1}]" \
    --query 'Instances[0].InstanceId' --output text
}

case "${1:-}" in
base)
  # Through a temporary .pem (covered by .gitignore): a redirect straight into $PEM_OUT leaves an empty file when aws
  # fails, and the next run would skip creating the key.
  if [ ! -f "$PEM_OUT" ]; then
    tmp_pem="${PEM_OUT%.pem}.partial.pem"
    aws ec2 create-key-pair --key-name "$KEY_NAME" --query KeyMaterial --output text > "$tmp_pem" \
      || { rm -f "$tmp_pem"; die "could not create key pair $KEY_NAME (if it exists in $REGION without its .pem here, set KEY_NAME, SG_NAME and TAG)"; }
    mv "$tmp_pem" "$PEM_OUT" && chmod 400 "$PEM_OUT" && echo "wrote $PEM_OUT"
  fi
  AMI=$(ubuntu_ami); [ "$AMI" != "None" ] || die "no Ubuntu 24.04 AMI found in $REGION"
  echo "base AMI $AMI   type $TYPE   region $REGION"
  id=$(launch base "$AMI") || die "run-instances"
  echo "base instance: $id"
  aws ec2 wait instance-running --instance-ids "$id"
  echo "SEV-SNP as the API reports it:"
  aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[].Instances[].CpuOptions' --output json
  echo "Next: bash e2e/cloud/aws-snp.sh mkami"
  ;;

mkami)
  id=$(id_of base); [ "$id" != "None" ] && [ -n "$id" ] || die "no base instance; run 'base' first"
  echo "stopping $id…"; aws ec2 stop-instances --instance-ids "$id" >/dev/null; aws ec2 wait instance-stopped --instance-ids "$id"
  ROOT=$(aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[].Instances[].RootDeviceName' --output text)
  # $ROOT is /dev/sda1: MSYS_NO_PATHCONV=1 keeps Git Bash from rewriting it here and in register-image below
  VOL=$(MSYS_NO_PATHCONV=1 aws ec2 describe-instances --instance-ids "$id" \
        --query "Reservations[].Instances[].BlockDeviceMappings[?DeviceName=='$ROOT'].Ebs.VolumeId" --output text)
  echo "root $ROOT  volume $VOL"
  SNAP=$(aws ec2 create-snapshot --volume-id "$VOL" --description "attestamp SEV-SNP base" --query SnapshotId --output text) || die "create-snapshot"
  echo "snapshot $SNAP, waiting…"; aws ec2 wait snapshot-completed --snapshot-ids "$SNAP"
  # The vTPM is an AMI attribute, so it can only be set by registering a new image.
  NEW=$(MSYS_NO_PATHCONV=1 aws ec2 register-image --name "$AMI_NAME-$(date +%Y%m%d%H%M)" \
        --architecture x86_64 --root-device-name "$ROOT" --virtualization-type hvm --ena-support \
        --boot-mode uefi --tpm-support v2.0 \
        --block-device-mappings "DeviceName=$ROOT,Ebs={SnapshotId=$SNAP,VolumeSize=32,VolumeType=gp3,DeleteOnTermination=true}" \
        --query ImageId --output text) || die "register-image"
  echo "TPM-supported AMI: $NEW"
  aws ec2 create-tags --resources "$NEW" --tags "Key=Project,Value=$TAG" >/dev/null
  aws ec2 describe-images --image-ids "$NEW" --query 'Images[].[ImageId,Name,TpmSupport,BootMode]' --output text
  echo "Next: bash e2e/cloud/aws-snp.sh vm"
  ;;

vm)
  AMI=$(aws ec2 describe-images --owners self --filters "Name=tag:Project,Values=$TAG" \
        --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text)
  [ "$AMI" != "None" ] && [ -n "$AMI" ] || die "no TPM-supported AMI; run 'mkami' first"
  id=$(launch vm "$AMI") || die "run-instances"
  echo "instance $id from $AMI"; aws ec2 wait instance-running --instance-ids "$id"
  exec bash "$0" status
  ;;

status)
  printf '%-7s %-21s %-11s %-16s %s\n' ROLE INSTANCE STATE ADDRESS SEV_SNP
  aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" \
    "Name=instance-state-name,Values=pending,running,stopping,stopped" \
    --query 'Reservations[].Instances[].[Tags[?Key==`Role`]|[0].Value,InstanceId,State.Name,PublicIpAddress,CpuOptions.AmdSevSnp]' \
    --output text | sort
  echo; echo "AMIs with TPM support owned by this account:"
  aws ec2 describe-images --owners self --query 'Images[].[ImageId,Name,TpmSupport,BootMode]' --output text
  echo; echo "On the instance, check the vTPM with:  ls -l /dev/tpm0"
  ;;

stop|start)
  ids=$(aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" --query 'Reservations[].Instances[].InstanceId' --output text)
  [ -n "$ids" ] || die "no instances tagged Project=$TAG"
  # shellcheck disable=SC2086
  aws ec2 "$1-instances" --instance-ids $ids >/dev/null && echo "$1: $ids"
  ;;

delete)
  ids=$(aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" \
        "Name=instance-state-name,Values=pending,running,stopping,stopped" \
        --query 'Reservations[].Instances[].InstanceId' --output text)
  # shellcheck disable=SC2086
  [ -n "$ids" ] && { aws ec2 terminate-instances --instance-ids $ids >/dev/null; aws ec2 wait instance-terminated --instance-ids $ids; echo "terminated $ids"; }
  echo "AMIs, snapshots and the security group were left alone. List them with 'status' and remove by hand when done — they keep costing storage."
  ;;

*) sed -n '2,20p' "$0"; exit 2 ;;
esac
