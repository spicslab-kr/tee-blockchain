#!/usr/bin/env bash
# Create, inspect and remove the GCP Confidential VM instances for TDX or SEV-SNP.
#
#   bash e2e/cloud/gcp-cvm.sh tdx create [vendor] [auditor] --yes   # one VM per role; default both (2 VMs)
#   bash e2e/cloud/gcp-cvm.sh snp create [vendor] [auditor] --yes   # SEV-SNP; T10 (docs/build/app-fixture-recording.md) needs a second VM
#   bash e2e/cloud/gcp-cvm.sh tdx status
#   bash e2e/cloud/gcp-cvm.sh tdx stop | start | delete
#
# create makes VMs only with --yes. Without it, it prints how many VMs it would make, with their names, and stops;
# --dry-run prints the same without calling gcloud at all. The roles are vendor and auditor, each at most once; any
# other word stops the script before anything runs.
#
# Names: each VM is <PREFIX>-<role>, PREFIX default attestamp-<tdx|snp>. If the project already has VMs of that name
# (made by someone else, or by you earlier), create reuses them; choose your own with PREFIX=<name>, e.g.
#   PREFIX=alice-tdx bash e2e/cloud/gcp-cvm.sh tdx create vendor --yes
# and pass the same PREFIX to status, stop, start and delete: they act only on VMs whose name starts with it.
#
# Image: IMAGE=<exact image name> pins one image version, e.g.
#   IMAGE=ubuntu-2404-noble-amd64-v20261105 bash e2e/cloud/gcp-cvm.sh tdx create auditor --yes
# Without IMAGE the family (IMAGE_FAMILY, default ubuntu-2404-lts-amd64) is resolved once to the image it points at
# now, that exact name is printed, and every VM of this call is made from it. The Vendor's and the Auditor's guests
# must be made from the same exact image: the root file system UUID, the partition UUID and the stock initrd come with
# the image, and they go into the kernel command line (TDX RTMR2, SEV-SNP CMDLINE) and the base initrd. A family moves
# to a new version every few weeks, so creating the second side later from the family can give another image and
# other values. Record the printed name and pass it as IMAGE= when creating the other side. `status` shows each
# disk's image.
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
[ -n "$PLAT" ] && [ -n "$ACTION" ] || { sed -n '2,37p' "$0"; exit 2; }
shift 2
# --yes: really create. --dry-run: print the plan and call nothing, not even gcloud. Anything else is a role.
YES=0; DRY=0; ROLES=()
for arg in "$@"; do
  case "$arg" in
    --yes) YES=1 ;;
    --dry-run) DRY=1 ;;
    -*) echo "unknown option '$arg': the options are --yes and --dry-run" >&2; exit 2 ;;
    *) ROLES+=("$arg") ;;
  esac
done
case "$ACTION" in create|status|stop|start|delete) ;; *) echo "unknown action '$ACTION'" >&2; sed -n '2,37p' "$0"; exit 2 ;; esac
# Only create takes roles and the two options. Elsewhere they are refused rather than ignored: "delete vendor" would
# otherwise delete every VM of the prefix, and "delete --dry-run" would really delete.
if [ "$ACTION" != create ] && { [ ${#ROLES[@]} -gt 0 ] || [ "$YES" = 1 ] || [ "$DRY" = 1 ]; }; then
  echo "$ACTION takes no roles and no options (it acts on every VM whose name starts with ${PREFIX:-attestamp-$PLAT}-; choose them with PREFIX=)" >&2
  exit 2
fi
[ ${#ROLES[@]} -gt 0 ] || ROLES=(vendor auditor)
# Every role is one billable VM, so only the two roles the project uses are accepted, each once. A stray word on the
# command line stops the script before anything else runs: e2e/cloud/README.md once had "create vendor auditor user y",
# which would have made four VMs (docs/results/build-info-gaps-2026-11.md, #20).
if [ "$ACTION" = create ]; then
  seen=" "
  for role in "${ROLES[@]}"; do
    case "$role" in vendor|auditor) ;; *) echo "unknown role '$role': each role is one VM; the roles are vendor and auditor" >&2; exit 2 ;; esac
    case "$seen" in *" $role "*) echo "role '$role' given twice: each role is one VM" >&2; exit 2 ;; esac
    seen="$seen$role "
  done
fi

case "$PLAT" in
  tdx) CC_TYPE=TDX;     MACHINE=${MACHINE:-c3-standard-4}; ZONE=${ZONE:-us-central1-a}; FEATURE=TDX_CAPABLE;     MINCPU=() ;;
  snp) CC_TYPE=SEV_SNP; MACHINE=${MACHINE:-n2d-standard-2}; ZONE=${ZONE:-us-central1-a}; FEATURE=SEV_SNP_CAPABLE; MINCPU=(--min-cpu-platform="AMD Milan") ;;
  *) echo "first argument must be tdx or snp" >&2; exit 2 ;;
esac

IMAGE_PROJECT=${IMAGE_PROJECT:-ubuntu-os-cloud}
IMAGE_FAMILY=${IMAGE_FAMILY:-ubuntu-2404-lts-amd64}
IMAGE=${IMAGE:-}   # an exact image name in IMAGE_PROJECT; empty: resolve IMAGE_FAMILY once at create
PREFIX=${PREFIX:-attestamp-$PLAT}
NET_TAG=attestamp-ra
RA_PORT=${RA_PORT:-5000}

# create: say what would be made, before any gcloud call, and make it only with --yes. On 2026-11-16 a run meant as a
# test, with no terminal to press Ctrl-C in, went through the old 4-second pause and created two VMs (docs/results/build-info-gaps-2026-11.md, #20).
if [ "$ACTION" = create ]; then
  names=""; for role in "${ROLES[@]}"; do names="$names $PREFIX-$role"; done
  echo "create: ${#ROLES[@]} billable VM(s):$names"
  echo "        $PLAT, $MACHINE, zone $ZONE, image ${IMAGE:-the one family $IMAGE_FAMILY points at now}"
  if [ "$DRY" = 1 ]; then
    echo "dry run: nothing was created and gcloud was not called."
    exit 0
  fi
  if [ "$YES" != 1 ]; then
    echo "Nothing created. To create these ${#ROLES[@]} VM(s), run the same command with --yes." >&2
    exit 3
  fi
fi

die() { echo "error: $*" >&2; exit 1; }
command -v gcloud >/dev/null 2>&1 || die "gcloud not found. Install it and run 'gcloud auth login'."
# On Windows (Git Bash) the gcloud CLI ends every output line with a carriage return, so a value captured with $(...)
#   keeps a trailing CR and the next command rejects it (2026-11-15: `gcp-cvm.sh tdx stop` with two guests). Strip it here, once,
#   for every call in this script; the exit status is gcloud's own.
gcloud() { command gcloud "$@" | tr -d '\r'; return "${PIPESTATUS[0]}"; }
gcloud config get-value project >/dev/null 2>&1 || die "no gcloud project set. Run 'gcloud config set project <id>'."

case "$ACTION" in
create)
  # One exact image for every VM of this call, and a name to record for the other side.
  if [ -n "$IMAGE" ]; then
    gcloud compute images describe "$IMAGE" --project "$IMAGE_PROJECT" --format='value(name)' >/dev/null \
      || die "image $IMAGE_PROJECT/$IMAGE not found (an old version can be deleted; then both sides move to a new one together)"
    from="given as IMAGE"
  else
    IMAGE=$(gcloud compute images describe-from-family "$IMAGE_FAMILY" --project "$IMAGE_PROJECT" --format='value(name)')
    [ -n "$IMAGE" ] || die "cannot resolve the image family $IMAGE_PROJECT/$IMAGE_FAMILY"
    from="what the family $IMAGE_FAMILY points at now"
  fi
  echo "platform $PLAT   machine $MACHINE   zone $ZONE"
  echo "image    $IMAGE_PROJECT/$IMAGE   ($from)"
  echo "roles: ${ROLES[*]}"
  echo "Record the image name: the other side's guest must be made from the same one, with"
  echo "    IMAGE=$IMAGE bash e2e/cloud/gcp-cvm.sh $PLAT create <role> --yes"
  echo "Creating ${#ROLES[@]} billable instance(s) (--yes given)."

  # Confirm the chosen image really carries the guest OS feature this platform needs.
  if ! gcloud compute images describe "$IMAGE" --project "$IMAGE_PROJECT" \
        --flatten="guestOsFeatures[]" --format='value(guestOsFeatures.type)' 2>/dev/null \
        | grep -qx "$FEATURE"; then
    echo "warning: $IMAGE_PROJECT/$IMAGE does not advertise $FEATURE."
    echo "Pick one of these families instead and re-run with IMAGE_PROJECT=… IMAGE_FAMILY=… (or an exact IMAGE=…):"
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
      echo "$name already exists, reusing it (not created). If it is not yours, run again with PREFIX=<your-name>-$PLAT."
      continue
    fi
    gcloud compute instances create "$name" \
      --machine-type="$MACHINE" \
      --zone="$ZONE" \
      "${MINCPU[@]}" \
      --confidential-compute-type="$CC_TYPE" \
      --maintenance-policy=TERMINATE \
      --image-project="$IMAGE_PROJECT" \
      --image="$IMAGE" \
      --boot-disk-size=64GB \
      --tags="$NET_TAG" \
      --labels="project=attestamp,platform=$PLAT,role=$role" || die "instances create ($role)"
  done
  exec bash "$0" "$PLAT" status
  ;;

status)
  gcloud compute instances list --filter="labels.project=attestamp AND labels.platform=$PLAT AND name~^$PREFIX-" \
    --format='table(name,zone.basename(),machineType.basename(),status,networkInterfaces[0].accessConfigs[0].natIP:label=EXTERNAL_IP)'
  echo
  echo "Confidential setting as the API reports it:"
  gcloud compute instances list --filter="labels.project=attestamp AND labels.platform=$PLAT AND name~^$PREFIX-" \
    --format='table(name,confidentialInstanceConfig.confidentialInstanceType,scheduling.onHostMaintenance)'
  echo
  echo "Image of each boot disk (the Vendor's and the Auditor's must be the same):"
  gcloud compute disks list --filter="name~^$PREFIX-" --format='table(name,zone.basename(),sourceImage.basename():label=IMAGE)'
  echo
  echo "ssh in with:  gcloud compute ssh <name> --zone $ZONE"
  echo "              add --tunnel-through-iap where port 22 is open only to Google's IAP range (e2e/cloud/README.md)"
  ;;

stop|start)
  names=$(gcloud compute instances list --filter="labels.project=attestamp AND labels.platform=$PLAT AND name~^$PREFIX-" --format='value(name)')
  [ -n "$names" ] || die "no $PLAT instances labelled project=attestamp"
  # shellcheck disable=SC2086
  gcloud compute instances "$ACTION" $names --zone "$ZONE" --quiet
  ;;

delete)
  names=$(gcloud compute instances list --filter="labels.project=attestamp AND labels.platform=$PLAT AND name~^$PREFIX-" --format='value(name)')
  [ -n "$names" ] || { echo "nothing to delete"; exit 0; }
  echo "deleting: $names"
  # shellcheck disable=SC2086
  gcloud compute instances delete $names --zone "$ZONE" --quiet
  echo "the firewall rule $NET_TAG was left alone (the other platform may still use it)."
  ;;

*) sed -n '2,37p' "$0"; exit 2 ;;
esac
