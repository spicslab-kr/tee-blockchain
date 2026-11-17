#!/usr/bin/env bash
# Runs every action of aws-nitro.sh, aws-snp.sh and gcp-cvm.sh against stand-in aws, gcloud and curl commands that only
# log their arguments, and checks each action's exit status and the CLI calls it made. Nothing is created and no
# network is used.
#
#   bash e2e/cloud/check-scripts.sh          # also run by go test ./cmd (TestCloudScripts)
#
# Why every action, as far as its CLI call: on 2026-11-16 the scripts were checked this way by hand for create only,
# and `aws-nitro.sh stop` and `start` (and the same two in aws-snp.sh) still ended at once with "$1: unbound
# variable", found on 2026-11-17 when the instances had to be stopped (fixed in bc87898). So a script action that no
# case below runs up to its aws or gcloud call fails this check, and a new action fails it until a case is added.
#
# Safety: PATH is the stand-in folder and /usr/bin:/bin only, and the check stops before running any script unless
# `command -v aws gcloud curl` are the stand-ins. On Windows the real CLIs are under C:/Program Files and cannot be
# found at all; on Linux the stand-ins come first. As a second barrier the CLIs' credentials and configuration are
# pointed at nothing, so a real CLI reached anyway would stop at the scripts' "not authenticated" check. Key files go
# to a temporary folder, never to the repository's pem2.pem or snp.pem.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp=$(mktemp -d) || exit 1
trap 'rm -rf "$tmp"' EXIT
fake="$tmp/bin"; mkdir -p "$fake"
log="$tmp/calls.log"

# The stand-ins. INSTANCES is what a query for instance IDs or VM names answers; the rest is fixed. Addresses are from
# the documentation ranges (RFC 5737), so none of them is anyone's machine.
cat > "$fake/aws" <<'EOF'
#!/usr/bin/env bash
printf 'aws %s\n' "$*" >> "$STANDIN_LOG"
a=" $* "
case "$a" in
  *" sts get-caller-identity "*) echo '{}' ;;
  *" ec2 describe-instances "*PublicIpAddress*InstanceId*|*" ec2 describe-instances "*InstanceId*PublicIpAddress*)
    for i in $INSTANCES; do printf 'vendor\t%s\trunning\t192.0.2.1\n' "$i"; done ;;
  *" ec2 describe-instances "*InstanceId*) echo $INSTANCES ;;
  *" ec2 describe-instances "*PublicIpAddress*) [ -n "$INSTANCES" ] && echo 192.0.2.1 ;;
  *" ec2 describe-instances "*RootDeviceName*) echo /dev/sda1 ;;
  *" ec2 describe-instances "*VolumeId*) echo vol-standin ;;
  *" ec2 describe-instances "*) echo '[]' ;;
  *" ec2 describe-images "*) echo ami-standin ;;
  *" ec2 describe-security-groups "*CidrIp*) echo 198.51.100.7/32 ;;
  *" ec2 describe-security-groups "*) echo sg-standin ;;
  *" ec2 create-key-pair "*) echo 'not a key' ;;
  *" ec2 run-instances "*) echo i-standin-new ;;
  *" ec2 create-snapshot "*) echo snap-standin ;;
  *" ec2 register-image "*) echo ami-standin-tpm ;;
esac
exit 0
EOF
cat > "$fake/gcloud" <<'EOF'
#!/usr/bin/env bash
printf 'gcloud %s\n' "$*" >> "$STANDIN_LOG"
a=" $* "
case "$a" in
  *" config get-value project "*) echo standin-project ;;
  *" compute images describe-from-family "*) echo ubuntu-2404-noble-amd64-v00000000 ;;
  *" compute images describe "*guestOsFeatures*) printf 'TDX_CAPABLE\nSEV_SNP_CAPABLE\n' ;;
  *" compute images describe "*) echo "$4" ;;
  *" compute instances describe "*) exit 1 ;;   # no VM of that name yet, so create makes it
  *" compute instances list "*"value(name)"*) for n in $INSTANCES; do echo "$n"; done ;;
  *" compute instances list "*|*" compute disks list "*) echo NAME ;;
esac
exit 0
EOF
cat > "$fake/curl" <<'EOF'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >> "$STANDIN_LOG"
echo 198.51.100.7
EOF
chmod +x "$fake/aws" "$fake/gcloud" "$fake/curl"

export PATH="$fake:/usr/bin:/bin"
for c in aws gcloud curl; do
  if [ "$(command -v "$c")" != "$fake/$c" ]; then
    echo "check-scripts: '$c' resolves to '$(command -v "$c")', not the stand-in; stopping before running any script" >&2
    exit 1
  fi
done
unset AWS_PROFILE AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN CLOUDSDK_CORE_PROJECT
# The scripts' own settings, so that the expectations below hold whatever the caller's environment sets.
unset REGION TYPE TYPE_ARM64 KEY_NAME SG_NAME TAG RA_PORT AMI_NAME PREFIX IMAGE IMAGE_FAMILY IMAGE_PROJECT MACHINE ZONE
export AWS_CONFIG_FILE="$tmp/none" AWS_SHARED_CREDENTIALS_FILE="$tmp/none" CLOUDSDK_CONFIG="$tmp/gcloud-config"
export STANDIN_LOG="$log"
export PEM_OUT="$tmp/standin.pem"

fails=0; n=0
covered=" "

# check <script> <want exit> <instances> [<regex the call log must match>...] -- <script arguments...>
# With no regex the script must not have called aws or gcloud at all.
check() {
  local script=$1 want=$2 instances=$3; shift 3
  local pats=()
  while [ "$1" != -- ]; do pats+=("$1"); shift; done; shift
  n=$((n + 1))
  : > "$log"; rm -f "$PEM_OUT"
  local out got
  out=$(INSTANCES="$instances" bash "$here/$script" "$@" 2>&1 </dev/null); got=$?
  local what="$script $*" bad=""
  [ "$got" = "$want" ] || bad="exit $got, want $want"
  if [ ${#pats[@]} -eq 0 ]; then
    grep -qE '^(aws|gcloud) ' "$log" && bad="${bad:+$bad; }it called the CLI, want no call"
  else
    local p
    for p in "${pats[@]}"; do grep -qE -- "$p" "$log" || bad="${bad:+$bad; }no call matching /$p/"; done
  fi
  if [ -n "$bad" ]; then
    fails=$((fails + 1))
    echo "FAIL  $what: $bad"
    echo "      output:"; printf '%s\n' "$out" | sed 's/^/        /'
    echo "      calls:";  sed 's/^/        /' "$log"
    return
  fi
  echo "ok    $what"
  # An action is covered once a case ran it with success as far as a CLI call it had to make.
  if [ "$want" = 0 ] && [ ${#pats[@]} -gt 0 ]; then
    local action=$1; [ "$script" = gcp-cvm.sh ] && action=$2
    covered="$covered$script:$action "
  fi
}

two="i-standin-1 i-standin-2"

# aws-nitro.sh
check aws-nitro.sh 2 ""                                                      --
check aws-nitro.sh 2 ""                                                      -- reboot
check aws-nitro.sh 3 ""                                                      -- create
check aws-nitro.sh 0 ""                                                      -- create --dry-run
check aws-nitro.sh 0 "" 'ec2 create-key-pair' 'ec2 run-instances .*m6i\.xlarge.*Role,Value=vendor' \
                        'ec2 run-instances .*m6i\.xlarge.*Role,Value=auditor' 'ec2 describe-security-groups'        -- create --yes
check aws-nitro.sh 3 ""                                                      -- create-arm64
check aws-nitro.sh 0 "" 'ec2 run-instances .*c7g\.xlarge.*Role,Value=arm64'  -- create-arm64 --yes
check aws-nitro.sh 0 "$two" 'ec2 describe-instances' 'ec2 describe-security-groups .*CidrIp'                     -- status
check aws-nitro.sh 0 "$two" "ec2 stop-instances --instance-ids $two"         -- stop
check aws-nitro.sh 0 "$two" "ec2 start-instances --instance-ids $two"        -- start
check aws-nitro.sh 1 ""     'ec2 describe-instances'                         -- stop
check aws-nitro.sh 0 "$two" "ec2 terminate-instances --instance-ids $two" 'ec2 delete-security-group'            -- delete
check aws-nitro.sh 2 "$two"                                                  -- stop --yes
check aws-nitro.sh 2 "$two"                                                  -- delete --dry-run

# aws-snp.sh
check aws-snp.sh 2 ""                                                        --
check aws-snp.sh 3 ""                                                        -- base
check aws-snp.sh 0 ""                                                        -- base --dry-run
check aws-snp.sh 0 "" 'ec2 create-key-pair' 'ec2 run-instances .*AmdSevSnp=enabled.*Role,Value=base'             -- base --yes
check aws-snp.sh 3 "i-standin-1"                                             -- mkami
check aws-snp.sh 0 "i-standin-1" 'ec2 stop-instances --instance-ids i-standin-1' 'ec2 create-snapshot --volume-id vol-standin' \
                                 'ec2 register-image .*--tpm-support v2\.0'  -- mkami --yes
check aws-snp.sh 0 "" 'ec2 describe-images --owners self' 'ec2 run-instances .*AmdSevSnp=enabled.*Role,Value=vm' -- vm --yes
check aws-snp.sh 0 "$two" 'ec2 describe-instances' 'ec2 describe-images --owners self'                           -- status
check aws-snp.sh 0 "$two" "ec2 stop-instances --instance-ids $two"           -- stop
check aws-snp.sh 0 "$two" "ec2 start-instances --instance-ids $two"          -- start
check aws-snp.sh 0 "$two" "ec2 terminate-instances --instance-ids $two"      -- delete
check aws-snp.sh 2 "$two"                                                    -- delete --yes

# gcp-cvm.sh
vms="attestamp-tdx-vendor attestamp-tdx-auditor"
check gcp-cvm.sh 2 ""                                                        --
check gcp-cvm.sh 2 ""                                                        -- tdx reboot
check gcp-cvm.sh 3 ""                                                        -- tdx create
check gcp-cvm.sh 0 ""                                                        -- tdx create --dry-run
check gcp-cvm.sh 2 ""                                                        -- tdx create vendor vendor --yes
check gcp-cvm.sh 0 "" 'compute instances create attestamp-tdx-vendor .*--confidential-compute-type=TDX' \
                      'compute instances create attestamp-tdx-auditor '      -- tdx create --yes
check gcp-cvm.sh 0 "" 'compute instances create attestamp-snp-vendor .*--confidential-compute-type=SEV_SNP' -- snp create vendor --yes
check gcp-cvm.sh 0 "$vms" 'compute instances list' 'compute disks list'      -- tdx status
check gcp-cvm.sh 0 "$vms" "compute instances stop $vms --zone"               -- tdx stop
check gcp-cvm.sh 0 "$vms" "compute instances start $vms --zone"              -- tdx start
check gcp-cvm.sh 1 ""     'compute instances list'                           -- tdx stop
check gcp-cvm.sh 0 "$vms" "compute instances delete $vms --zone"             -- tdx delete
check gcp-cvm.sh 2 "$vms"                                                    -- tdx delete vendor
check gcp-cvm.sh 2 "$vms"                                                    -- tdx delete --dry-run

# Every action a script dispatches on must be covered above. The actions are the labels of the script's last case
# statement, the one written at column 0 ("stop|start)", "delete)").
for script in aws-nitro.sh aws-snp.sh gcp-cvm.sh; do
  actions=$(grep -E '^[a-z0-9|-]+\)$' "$here/$script" | tr -d ')' | tr '|' '\n')
  if [ "$(printf '%s\n' "$actions" | grep -c .)" -lt 4 ]; then
    fails=$((fails + 1)); echo "FAIL  $script: found only these actions at column 0: $(echo $actions); the check reads them there"
    continue
  fi
  for a in $actions; do
    case "$covered" in
      *" $script:$a "*) ;;
      *) fails=$((fails + 1)); echo "FAIL  $script $a: no case runs this action as far as its CLI call; add one above" ;;
    esac
  done
done

echo
if [ "$fails" -gt 0 ]; then echo "check-scripts: $fails of the checks failed ($n cases)"; exit 1; fi
echo "check-scripts: all $n cases passed, and every action of the three scripts reached its CLI call"
