#!/usr/bin/env bash
# Create, inspect and remove the GCP Confidential VM instances for TDX or SEV-SNP.
#
#   bash e2e/cloud/gcp-cvm.sh tdx create [vendor] [auditor]   # one VM per role; default both (2 VMs)
#   bash e2e/cloud/gcp-cvm.sh snp create [vendor] [auditor]   # SEV-SNP; T10 (docs/plan/app-fixture-recording.md) needs a second VM
#   bash e2e/cloud/gcp-cvm.sh tdx status
#   bash e2e/cloud/gcp-cvm.sh tdx stop | start | delete
#
# Verified against the Google Cloud docs (2026-11):
#   SEV-SNP  N2D machine types only, CPU platform AMD Milan, --confidential-compute-type=SEV_SNP
#            zones: asia-southeast1-a/b/c, europe-west3-a/b/c, europe-west4-a/b/c, us-central1-a/b/c
#            the image needs the SEV_SNP_CAPABLE guest OS feature (Debian 12 has no SNP attestation)
#   TDX      C3 (Sapphire Rapids) or C4 (Granite Rapids), --confidential-compute-type=TDX,
#            and NO --min-cpu-platform. The image needs the TDX_CAPABLE guest OS feature.
#   Both     --maintenance-policy=TERMINATE; these machine types cannot live-migrate.
#
# vTPM: the Confidential VM docs do not state it, but our own runs confirmed /dev/tpm0 is present
# on a GCP Confidential VM without any extra flag. AWS is the one that needs a TPM-support AMI.
set -uo pipefail

PLAT=${1:-}; ACTION=${2:-}
[ -n "$PLAT" ] && [ -n "$ACTION" ] || { sed -n '2,20p' "$0"; exit 2; }
shift 2
ROLES=("$@"); [ ${#ROLES[@]} -gt 0 ] || ROLES=(vendor auditor)
# Every role is one billable VM, so only the two roles the project uses are accepted: a stray word on the command line
# (it happened in e2e/cloud/README.md: "create vendor auditor user y" would have made four VMs) stops the script.
if [ "$ACTION" = create ]; then
  for role in "${ROLES[@]}"; do
    case "$role" in vendor|auditor) ;; *) echo "unknown role '$role': each role is one VM; use vendor and/or auditor" >&2; exit 2 ;; esac
  done
fi

case "$PLAT" in
  tdx) CC_TYPE=TDX;     MACHINE=${MACHINE:-c3-standard-4}; ZONE=${ZONE:-us-central1-a}; FEATURE=TDX_CAPABLE;     MINCPU=() ;;
  snp) CC_TYPE=SEV_SNP; MACHINE=${MACHINE:-n2d-standard-2}; ZONE=${ZONE:-us-central1-a}; FEATURE=SEV_SNP_CAPABLE; MINCPU=(--min-cpu-platform="AMD Milan") ;;
  *) echo "first argument must be tdx or snp" >&2; exit 2 ;;
esac

IMAGE_PROJECT=${IMAGE_PROJECT:-ubuntu-os-cloud}
IMAGE_FAMILY=${IMAGE_FAMILY:-ubuntu-2404-lts-amd64}
PREFIX=${PREFIX:-attestamp-$PLAT}
NET_TAG=attestamp-ra
RA_PORT=${RA_PORT:-5000}

die() { echo "error: $*" >&2; exit 1; }
command -v gcloud >/dev/null 2>&1 || die "gcloud not found. Install it and run 'gcloud auth login'."
# On Windows (Git Bash) the gcloud CLI ends every output line with a carriage return, so a value captured with $(...)
#   keeps a trailing CR and the next command rejects it (2026-11-15: `gcp-cvm.sh tdx stop` with two guests). Strip it here, once,
#   for every call in this script; the exit status is gcloud's own.
gcloud() { command gcloud "$@" | tr -d '\r'; return "${PIPESTATUS[0]}"; }
gcloud config get-value project >/dev/null 2>&1 || die "no gcloud project set. Run 'gcloud config set project <id>'."

case "$ACTION" in
create)
  echo "platform $PLAT   machine $MACHINE   zone $ZONE   image $IMAGE_PROJECT/$IMAGE_FAMILY"
  echo "roles: ${ROLES[*]}"
  echo "This creates ${#ROLES[@]} billable instances. Ctrl-C now to stop."; sleep 4

  # Confirm the chosen image family really carries the guest OS feature this platform needs.
  if ! gcloud compute images describe-from-family "$IMAGE_FAMILY" --project "$IMAGE_PROJECT" \
        --flatten="guestOsFeatures[]" --format='value(guestOsFeatures.type)' 2>/dev/null \
        | grep -qx "$FEATURE"; then
    echo "warning: $IMAGE_PROJECT/$IMAGE_FAMILY does not advertise $FEATURE."
    echo "Pick one of these instead and re-run with IMAGE_PROJECT=… IMAGE_FAMILY=…:"
    gcloud compute images list --filter="guestOsFeatures[].type:($FEATURE)" \
      --format='value(family)' | sort -u
    die "image does not support $FEATURE"
  fi

  # One firewall rule for the RA port, limited to this machine's public IP.
  if ! gcloud compute firewall-rules describe "$NET_TAG" >/dev/null 2>&1; then
    MYIP=$(curl -fsS https://checkip.amazonaws.com 2>/dev/null | tr -d '[:space:]')
    [ -n "$MYIP" ] || die "could not determine your public IP; open port $RA_PORT by hand"
    gcloud compute firewall-rules create "$NET_TAG" \
      --allow="tcp:$RA_PORT" --target-tags="$NET_TAG" --source-ranges="$MYIP/32" \
      --description="attestamp: RA port, this machine only" >/dev/null || die "firewall-rules create"
    echo "firewall rule $NET_TAG: tcp:$RA_PORT from $MYIP/32"
  fi

  for role in "${ROLES[@]}"; do
    name="$PREFIX-$role"
    if gcloud compute instances describe "$name" --zone "$ZONE" >/dev/null 2>&1; then
      echo "$name already exists, skipping"; continue
    fi
    gcloud compute instances create "$name" \
      --machine-type="$MACHINE" \
      --zone="$ZONE" \
      "${MINCPU[@]}" \
      --confidential-compute-type="$CC_TYPE" \
      --maintenance-policy=TERMINATE \
      --image-project="$IMAGE_PROJECT" \
      --image-family="$IMAGE_FAMILY" \
      --boot-disk-size=64GB \
      --tags="$NET_TAG" \
      --labels="project=attestamp,platform=$PLAT,role=$role" || die "instances create ($role)"
  done
  exec "$0" "$PLAT" status
  ;;

status)
  gcloud compute instances list --filter="labels.project=attestamp AND labels.platform=$PLAT" \
    --format='table(name,zone.basename(),machineType.basename(),status,networkInterfaces[0].accessConfigs[0].natIP:label=EXTERNAL_IP)'
  echo
  echo "Confidential setting as the API reports it:"
  gcloud compute instances list --filter="labels.project=attestamp AND labels.platform=$PLAT" \
    --format='table(name,confidentialInstanceConfig.confidentialInstanceType,scheduling.onHostMaintenance)'
  echo
  echo "ssh in with:  gcloud compute ssh <name> --zone $ZONE"
  ;;

stop|start)
  names=$(gcloud compute instances list --filter="labels.project=attestamp AND labels.platform=$PLAT" --format='value(name)')
  [ -n "$names" ] || die "no $PLAT instances labelled project=attestamp"
  # shellcheck disable=SC2086
  gcloud compute instances "$ACTION" $names --zone "$ZONE" --quiet
  ;;

delete)
  names=$(gcloud compute instances list --filter="labels.project=attestamp AND labels.platform=$PLAT" --format='value(name)')
  [ -n "$names" ] || { echo "nothing to delete"; exit 0; }
  echo "deleting: $names"
  # shellcheck disable=SC2086
  gcloud compute instances delete $names --zone "$ZONE" --quiet
  echo "the firewall rule $NET_TAG was left alone (the other platform may still use it)."
  ;;

*) sed -n '2,20p' "$0"; exit 2 ;;
esac
