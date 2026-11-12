#!/usr/bin/env bash
# Create, inspect and remove the AWS Nitro Enclaves instances the e2e suite needs.
#
#   bash e2e/cloud/aws-nitro.sh create         # key pair, security group, two x86_64 instances (vendor, auditor)
#   bash e2e/cloud/aws-nitro.sh create-arm64   # one more, arm64 (Graviton), for the architecture checks (ARM64_HOST)
#   bash e2e/cloud/aws-nitro.sh status         # addresses, to paste into e2e/env.local.sh
#   bash e2e/cloud/aws-nitro.sh stop           # stop all of them (keeps the disks, stops the hourly charge)
#   bash e2e/cloud/aws-nitro.sh start          # start them again
#   bash e2e/cloud/aws-nitro.sh delete         # terminate all of them and delete the security group
#
# Verified against the AWS docs (2026-11):
#   - Nitro Enclaves is turned on at launch with --enclave-options 'Enabled=true'
#   - the smallest size of most families is NOT supported (no m5.large, no m6i.large);
#     m6i.xlarge is what this project's e2e runs used
#   - Graviton (arm64) instances support Nitro Enclaves too; c7g.xlarge (4 vCPU) is the arm64 counterpart
#     (2026-11-12, aws ec2 describe-instance-types in eu-north-1: NitroEnclavesSupport "supported" for both types)
#   - the parent instance must run Linux or Windows 2016+
set -uo pipefail

REGION=${REGION:-eu-north-1}
TYPE=${TYPE:-m6i.xlarge}              # 4 vCPU, x86_64. .large sizes are not supported by Nitro Enclaves
TYPE_ARM64=${TYPE_ARM64:-c7g.xlarge}  # 4 vCPU, arm64 (Graviton)
KEY_NAME=${KEY_NAME:-attestamp-e2e}
SG_NAME=${SG_NAME:-attestamp-e2e}
TAG=${TAG:-attestamp-e2e}
PEM_OUT=${PEM_OUT:-pem2.pem}      # e2e/env.sh expects this name in the repository root
RA_PORT=${RA_PORT:-5000}

here="$(cd "$(dirname "$0")" && pwd)"; cd "$here/../.."
aws() { command aws --region "$REGION" "$@"; }
die() { echo "error: $*" >&2; exit 1; }

command -v /usr/bin/env >/dev/null
command -v aws >/dev/null 2>&1 || die "aws CLI not found. Install it and run 'aws configure' first."
aws sts get-caller-identity >/dev/null 2>&1 || die "aws CLI is not authenticated. Run 'aws configure'."

# latest_al2023 <x86_64|arm64> - the newest Amazon Linux 2023 AMI for that architecture.
latest_al2023() {
  aws ec2 describe-images --owners amazon \
    --filters "Name=name,Values=al2023-ami-2023.*-kernel-*-$1" "Name=state,Values=available" \
    --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text
}

ids_by_role() {
  aws ec2 describe-instances \
    --filters "Name=tag:Project,Values=$TAG" "Name=tag:Role,Values=$1" \
              "Name=instance-state-name,Values=pending,running,stopping,stopped" \
    --query 'Reservations[].Instances[].InstanceId' --output text
}

address_of() {
  aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" "Name=tag:Role,Values=$1" \
    "Name=instance-state-name,Values=running" --query 'Reservations[].Instances[].PublicIpAddress' --output text
}

# ensure_key_and_group - the key pair (written to $PEM_OUT) and the security group, created once and then reused.
# Sets SG.
ensure_key_and_group() {
  if [ ! -f "$PEM_OUT" ]; then
    aws ec2 create-key-pair --key-name "$KEY_NAME" --query KeyMaterial --output text > "$PEM_OUT" \
      || die "could not create key pair $KEY_NAME (does it already exist? delete it or set KEY_NAME)"
    chmod 400 "$PEM_OUT"
    echo "wrote $PEM_OUT  (gitignored - never commit it)"
  else
    echo "$PEM_OUT already exists, reusing it"
  fi

  SG=$(aws ec2 describe-security-groups --filters "Name=group-name,Values=$SG_NAME" \
         --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null)
  if [ "$SG" = "None" ] || [ -z "$SG" ]; then
    SG=$(aws ec2 create-security-group --group-name "$SG_NAME" \
           --description "attestamp e2e: ssh and the RA port" --query GroupId --output text) || die "create-security-group"
    MYIP=$(curl -fsS https://checkip.amazonaws.com 2>/dev/null | tr -d '[:space:]')
    [ -n "$MYIP" ] || die "could not determine your public IP; open the ports by hand"
    aws ec2 authorize-security-group-ingress --group-id "$SG" --protocol tcp --port 22        --cidr "$MYIP/32" >/dev/null
    aws ec2 authorize-security-group-ingress --group-id "$SG" --protocol tcp --port "$RA_PORT" --cidr "$MYIP/32" >/dev/null
    echo "security group $SG: ports 22 and $RA_PORT open to $MYIP/32 only"
  else
    echo "security group $SG already exists, reusing it"
  fi
}

# launch_role <role> <instance type> <ami> - one instance with Nitro Enclaves on, unless one with that role exists.
launch_role() {
  if [ -n "$(ids_by_role "$1")" ]; then echo "$1 instance already exists, skipping"; return 0; fi
  # MSYS_NO_PATHCONV=1: Git Bash (MSYS) rewrites an argument holding /dev/xvda into a Windows path before aws.exe
  # sees it ("Invalid device name C:/Program Files/Git/dev/xvda"), and quoting does not stop it. This turns the
  # conversion off for this one command; every other argument here does not start with /. Ignored on Linux and macOS.
  id=$(MSYS_NO_PATHCONV=1 aws ec2 run-instances \
    --image-id "$3" --count 1 --instance-type "$2" \
    --key-name "$KEY_NAME" --security-group-ids "$SG" \
    --enclave-options 'Enabled=true' \
    --block-device-mappings 'DeviceName=/dev/xvda,Ebs={VolumeSize=64,VolumeType=gp3}' \
    --tag-specifications "ResourceType=instance,Tags=[{Key=Project,Value=$TAG},{Key=Role,Value=$1}]" \
    --query 'Instances[0].InstanceId' --output text) || die "run-instances ($1)"
  echo "$1: $id ($2)"
}

case "${1:-}" in
create)
  echo "region $REGION   type $TYPE"
  echo "This creates two billable EC2 instances. Ctrl-C now to stop."; sleep 4
  ensure_key_and_group
  AMI=$(latest_al2023 x86_64); [ "$AMI" != "None" ] || die "no Amazon Linux 2023 x86_64 AMI found in $REGION"
  echo "AMI $AMI (latest Amazon Linux 2023, x86_64)"
  for role in vendor auditor; do launch_role "$role" "$TYPE" "$AMI"; done
  echo; echo "waiting for them to be running..."
  aws ec2 wait instance-running --filters "Name=tag:Project,Values=$TAG" 2>/dev/null
  exec "$0" status
  ;;

create-arm64)
  echo "region $REGION   type $TYPE_ARM64 (arm64)"
  echo "This creates one billable EC2 instance. Ctrl-C now to stop."; sleep 4
  ensure_key_and_group
  AMI=$(latest_al2023 arm64); [ "$AMI" != "None" ] || die "no Amazon Linux 2023 arm64 AMI found in $REGION"
  echo "AMI $AMI (latest Amazon Linux 2023, arm64)"
  launch_role arm64 "$TYPE_ARM64" "$AMI"
  echo; echo "waiting for it to be running..."
  aws ec2 wait instance-running --filters "Name=tag:Project,Values=$TAG" "Name=tag:Role,Values=arm64" 2>/dev/null
  exec "$0" status
  ;;

status)
  printf '%-9s %-21s %-11s %s\n' ROLE INSTANCE STATE ADDRESS
  aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" \
    "Name=instance-state-name,Values=pending,running,stopping,stopped" \
    --query 'Reservations[].Instances[].[Tags[?Key==`Role`]|[0].Value,InstanceId,State.Name,PublicIpAddress]' \
    --output text | sort
  echo
  V=$(address_of vendor); A=$(address_of auditor); R=$(address_of arm64)
  if [ -n "$V" ] || [ -n "$A" ] || [ -n "$R" ]; then
    echo "Paste into e2e/env.local.sh (gitignored):"
    echo "  VENDOR_HOST=$V"
    echo "  AUDITOR_HOST=$A"
    echo "  ARM64_HOST=$R"
  fi
  ;;

stop|start)
  ids=$(aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" \
        "Name=instance-state-name,Values=pending,running,stopping,stopped" \
        --query 'Reservations[].Instances[].InstanceId' --output text)
  [ -n "$ids" ] || die "no instances tagged Project=$TAG"
  aws ec2 "$1-instances" --instance-ids $ids --query "${1^}pingInstances" --output text 2>/dev/null \
    || aws ec2 "$1-instances" --instance-ids $ids >/dev/null
  echo "$1: $ids"
  ;;

delete)
  ids=$(aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" \
        "Name=instance-state-name,Values=pending,running,stopping,stopped" \
        --query 'Reservations[].Instances[].InstanceId' --output text)
  [ -n "$ids" ] && { echo "terminating $ids"; aws ec2 terminate-instances --instance-ids $ids >/dev/null
                     aws ec2 wait instance-terminated --instance-ids $ids; }
  SG=$(aws ec2 describe-security-groups --filters "Name=group-name,Values=$SG_NAME" \
        --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null)
  [ "$SG" != "None" ] && [ -n "$SG" ] && aws ec2 delete-security-group --group-id "$SG" && echo "deleted $SG"
  echo "key pair $KEY_NAME and $PEM_OUT were left alone; delete them by hand if you are done."
  ;;

*) sed -n '2,16p' "$0"; exit 2 ;;
esac
