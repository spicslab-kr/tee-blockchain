#!/usr/bin/env bash
# Create, inspect and remove the AWS Nitro Enclaves instances the e2e suite needs.
#
#   bash e2e/cloud/aws-nitro.sh create --yes         # key pair, security group, two x86_64 instances (vendor, auditor)
#   bash e2e/cloud/aws-nitro.sh create-arm64 --yes   # one more, arm64 (Graviton), for the architecture checks (ARM64_HOST)
#   (without --yes they print what they would make and stop; --dry-run prints it without calling aws)
#   bash e2e/cloud/aws-nitro.sh status         # addresses, to paste into e2e/env.local.sh, and whether this
#                                              # machine's public address is allowed in (only prints the command)
#   bash e2e/cloud/aws-nitro.sh stop           # stop all of them (keeps the disks, stops the hourly charge)
#   bash e2e/cloud/aws-nitro.sh start          # start them again, then the same address check
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
die() { echo "error: $*" >&2; exit 1; }

# Arguments, checked before any aws call. create and create-arm64 make instances only with --yes; without it they print
# what they would make and stop, and --dry-run prints the same without calling aws at all. Every other action takes no
# argument: an option given to it is refused, not ignored ("delete --dry-run" would otherwise really delete). The same
# rules hold in gcp-cvm.sh and aws-snp.sh; on 2026-11-16 a 4-second pause, the guard before, let a run with no
# terminal create two VMs (docs/status/build-info-gaps.md, #20).
ACTION=${1:-}; [ $# -gt 0 ] && shift
YES=0; DRY=0
for arg in "$@"; do
  case "$arg" in
    --yes) YES=1 ;;
    --dry-run) DRY=1 ;;
    *) echo "unknown argument '$arg': the only options are --yes and --dry-run, for create and create-arm64" >&2; exit 2 ;;
  esac
done
case "$ACTION" in
  create|create-arm64) ;;
  status|stop|start|delete)
    if [ "$YES" = 1 ] || [ "$DRY" = 1 ]; then echo "$ACTION takes no options; --yes and --dry-run are for create and create-arm64" >&2; exit 2; fi ;;
  *) sed -n '2,19p' "$0"; exit 2 ;;
esac
if [ "$ACTION" = create ] || [ "$ACTION" = create-arm64 ]; then
  if [ "$ACTION" = create ]; then n=2; what="Role=vendor and Role=auditor, $TYPE (x86_64)"
  else n=1; what="Role=arm64, $TYPE_ARM64 (arm64)"; fi
  echo "$ACTION: $n billable EC2 instance(s) in $REGION, tagged Project=$TAG: $what, each skipped if one with that tag"
  echo "        exists; and, if not there yet, the key pair $KEY_NAME (written to $PEM_OUT) and the security group"
  echo "        $SG_NAME (ports 22 and $RA_PORT from this machine's address /32 only)"
  if [ "$DRY" = 1 ]; then echo "dry run: nothing was created and aws was not called."; exit 0; fi
  if [ "$YES" != 1 ]; then echo "Nothing created. To create, run the same command with --yes." >&2; exit 3; fi
fi

command -v /usr/bin/env >/dev/null
command -v aws >/dev/null 2>&1 || die "aws CLI not found. Install it and run 'aws configure' first."
# On Windows (Git Bash) the aws CLI ends every output line with a carriage return, so a value captured with $(...)
#   keeps a trailing CR and the next command rejects it (the same as gcloud's in gcp-cvm.sh `stop`, 2026-11-15). Strip it here, once,
#   for every call in this script; the exit status is aws's own. --region is here too: this is the only aws() wrapper,
#   so every call goes to $REGION and not to the profile's default region.
aws() { command aws --region "$REGION" "$@" | tr -d '\r'; return "${PIPESTATUS[0]}"; }
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
    # Written to a temporary file first: a redirect straight into $PEM_OUT leaves an empty file behind when aws fails,
    # and the next run would then say "already exists, reusing it" and never create the key. The temporary name ends
    # in .pem so that .gitignore covers it even if the run is interrupted.
    tmp_pem="${PEM_OUT%.pem}.partial.pem"
    aws ec2 create-key-pair --key-name "$KEY_NAME" --query KeyMaterial --output text > "$tmp_pem" \
      || { rm -f "$tmp_pem"; die "could not create key pair $KEY_NAME. If it already exists in $REGION (made on
  another machine, so its private key is not here), choose new names for this machine:
    KEY_NAME=<name> SG_NAME=<name> TAG=<name> bash e2e/cloud/aws-nitro.sh create --yes
  and pass the same TAG to status, stop, start and delete."; }
    mv "$tmp_pem" "$PEM_OUT"
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

# check_my_address - is this machine's current public address allowed in by the security group on port 22 and the RA
# port? If not, print the commands that would add it; never change the group here, and never suggest 0.0.0.0/0.
# (2026-11-16: the address changed with the network, and every SSH then timed out with no other message.) Only an
# exact <address>/32 rule counts as allowed; a wider range that happens to cover it is reported as missing.
check_my_address() {
  local sg myip port have missing=0
  sg=$(aws ec2 describe-security-groups --filters "Name=group-name,Values=$SG_NAME" \
         --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null)
  if [ -z "$sg" ] || [ "$sg" = "None" ]; then echo "security group $SG_NAME not found"; return 0; fi
  myip=$(curl -fsS https://checkip.amazonaws.com 2>/dev/null | tr -d '[:space:]')
  if [ -z "$myip" ]; then echo "could not determine this machine's public address; check $sg by hand"; return 0; fi
  for port in 22 "$RA_PORT"; do
    have=$(aws ec2 describe-security-groups --group-ids "$sg" \
             --query "SecurityGroups[0].IpPermissions[?FromPort==\`$port\`].IpRanges[].CidrIp" --output text)
    case " $(echo $have) " in
      *" $myip/32 "*) ;;
      *)
        if [ "$missing" = 0 ]; then
          echo "this machine's public address $myip/32 is NOT allowed in by $sg ($SG_NAME): SSH and the RA port will time out."
          echo "To allow it (this script does not change the group; remove addresses you no longer use):"
        fi
        missing=1
        echo "  aws --region $REGION ec2 authorize-security-group-ingress --group-id $sg --protocol tcp --port $port --cidr $myip/32"
        ;;
    esac
  done
  [ "$missing" = 0 ] && echo "this machine's public address $myip/32 is allowed in by $sg on ports 22 and $RA_PORT"
  return 0
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

case "$ACTION" in
create)
  echo "region $REGION   type $TYPE"
  echo "creating (--yes given)"
  ensure_key_and_group
  AMI=$(latest_al2023 x86_64); [ "$AMI" != "None" ] || die "no Amazon Linux 2023 x86_64 AMI found in $REGION"
  echo "AMI $AMI (latest Amazon Linux 2023, x86_64)"
  for role in vendor auditor; do launch_role "$role" "$TYPE" "$AMI"; done
  echo; echo "waiting for them to be running..."
  aws ec2 wait instance-running --filters "Name=tag:Project,Values=$TAG" 2>/dev/null
  exec bash "$0" status
  ;;

create-arm64)
  echo "region $REGION   type $TYPE_ARM64 (arm64)"
  echo "creating (--yes given)"
  ensure_key_and_group
  AMI=$(latest_al2023 arm64); [ "$AMI" != "None" ] || die "no Amazon Linux 2023 arm64 AMI found in $REGION"
  echo "AMI $AMI (latest Amazon Linux 2023, arm64)"
  launch_role arm64 "$TYPE_ARM64" "$AMI"
  echo; echo "waiting for it to be running..."
  aws ec2 wait instance-running --filters "Name=tag:Project,Values=$TAG" "Name=tag:Role,Values=arm64" 2>/dev/null
  exec bash "$0" status
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
  echo
  check_my_address
  ;;

stop|start)
  ids=$(aws ec2 describe-instances --filters "Name=tag:Project,Values=$TAG" \
        "Name=instance-state-name,Values=pending,running,stopping,stopped" \
        --query 'Reservations[].Instances[].InstanceId' --output text)
  [ -n "$ids" ] || die "no instances tagged Project=$TAG"
  aws ec2 "$1-instances" --instance-ids $ids >/dev/null || die "$1-instances"
  echo "$1: $ids"
  # A started instance gets a new address, and this machine's may have changed too: say whether it can get in.
  if [ "$1" = start ]; then check_my_address; fi   # not "[ … ] && …": on stop that would end the script with exit 1
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

*) sed -n '2,19p' "$0"; exit 2 ;;
esac
