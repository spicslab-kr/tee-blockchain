#!/usr/bin/env bash
# Setup for the CVM flow - prepares the Vendor's and the Auditor's guest so that the flow scripts can run, and can be run
# again before every take: what is already done is left as it is. The guests themselves are made beforehand with
#   bash e2e/cloud/gcp-cvm.sh <tdx|snp> create --yes          (two guests, <PREFIX>-vendor and <PREFIX>-auditor)
# This script creates nothing in the cloud.
#
#   1. both guests must be the same image and machine type ("The same inputs" in docs/build/tdx.md and sev-snp.md)
#   2. builds the Linux binaries into linux/ and this machine's into e2e-work/bin (README.md, "The binaries")
#   3. copies att-<role>, att-ra-responder and the guest-side script to each guest, and the Option 2 hooks to the Vendor
#   4. prepares each guest (step 4 of the guides; the Auditor's is A3): an initrd boot, the base initrd, packages
#      For Option 2 the Vendor makes base-initrd-v.img and the Auditor gets the Vendor's copy, never its own.
#   5. the base initrd must be the same file on both guests
#
#   Run from the repository root:  bash e2e/cvm/0-Setup-guests.sh <intel-tdx [1|2] | amd-sev-snp-initrd | amd-sev-snp-verity>
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
. "$here/env-cvm.sh" "$@"

step_start "CHECKING THE TWO GUESTS ($VENDOR_VM, $AUDITOR_VM)"
# image_of, type_of <vm>: the source image of the guest's boot disk (gcp-cvm.sh names it after the VM) and its machine type.
image_of() { gcloud compute disks describe "$1" --zone "$ZONE" --format='value(sourceImage.basename())'; }
type_of() { gcloud compute instances describe "$1" --zone "$ZONE" --format='value(machineType.basename())'; }
vimg=$(image_of "$VENDOR_VM") && vtype=$(type_of "$VENDOR_VM") \
  || { echo "no guest $VENDOR_VM in $ZONE: make the guests with bash e2e/cloud/gcp-cvm.sh $PLAT create --yes" >&2; exit 1; }
aimg=$(image_of "$AUDITOR_VM") && atype=$(type_of "$AUDITOR_VM") \
  || { echo "no guest $AUDITOR_VM in $ZONE: make the guests with bash e2e/cloud/gcp-cvm.sh $PLAT create --yes" >&2; exit 1; }
echo "$VENDOR_VM: image $vimg, machine type $vtype"
echo "$AUDITOR_VM: image $aimg, machine type $atype"
if [ "$vimg" != "$aimg" ] || [ "$vtype" != "$atype" ]; then
  echo "the two guests differ in image or machine type, so the Auditor cannot reproduce the Vendor's values; make the" >&2
  echo "Auditor's guest with IMAGE=<the Vendor's image> MACHINE=<the Vendor's type> (docs/build/tdx.md, A2)" >&2
  exit 1
fi
step_stop

# The same build settings as README.md, "The binaries", and e2e/0-Setup-local-upload.sh.
step_start "BUILDING BINARIES"
WANT_GO="$(tr -d '[:space:]' < internal/setup/build/GO_TOOLCHAIN)"
HAVE_GO="$(go env GOVERSION)"
[ "$HAVE_GO" = "$WANT_GO" ] || { echo "Go is $HAVE_GO; this project is pinned to exactly $WANT_GO (internal/setup/build/GO_TOOLCHAIN)" >&2; exit 1; }
mkdir -p linux "$BIN"
for p in vendor auditor ra-responder; do
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "linux/att-$p" "./cmd/$p" || exit 1
done
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$VENDOR_BIN" ./cmd/vendor || exit 1
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$AUDITOR_BIN" ./cmd/auditor || exit 1
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$USER_BIN" ./cmd/user || exit 1
step_stop

step_start "COPYING TO THE GUESTS"
guest_put "$VENDOR_VM" linux/att-vendor att-vendor || exit 1
guest_put "$VENDOR_VM" linux/att-ra-responder att-ra-responder || exit 1
guest_put "$AUDITOR_VM" linux/att-auditor att-auditor || exit 1
guest_put "$AUDITOR_VM" linux/att-ra-responder att-ra-responder || exit 1
verity=0; { [ "$FORMAT" = amd-sev-snp-verity ] || { [ "$FORMAT" = intel-tdx ] && [ "$OPTION" = 2 ]; }; } && verity=1
if [ "$verity" = 1 ]; then
  hooks=internal/tee/parse/$PLAT/testdata/record
  guest_put "$VENDOR_VM" "$hooks/initramfs-hook.sh" initramfs-hook.sh || exit 1
  guest_put "$VENDOR_VM" "$hooks/initramfs-local-bottom.sh" initramfs-local-bottom.sh || exit 1
fi
step_stop

tstep "PREPARING THE VENDOR'S GUEST" guest_step "$VENDOR_VM" vendor prepare || exit 1
if [ "$verity" = 1 ]; then
  step_start "COPYING THE VENDOR'S base-initrd-v.img TO THE AUDITOR"
  guest_get "$VENDOR_VM" base-initrd-v.img "$CVM_WORK/vendor-base-initrd-v.img" || exit 1
  guest_put "$AUDITOR_VM" "$CVM_WORK/vendor-base-initrd-v.img" base-initrd-v.img || exit 1
  step_stop
fi
tstep "PREPARING THE AUDITOR'S GUEST" guest_step "$AUDITOR_VM" auditor prepare || exit 1

step_start "COMPARING THE BASE INITRD"
base=base-initrd.img; [ "$verity" = 1 ] && base=base-initrd-v.img
hv=$(guest_run "$VENDOR_VM" "sha256sum $base" | awk '{print $1}')
ha=$(guest_run "$AUDITOR_VM" "sha256sum $base" | awk '{print $1}')
echo "Vendor $hv"; echo "Auditor $ha"
[ -n "$hv" ] && [ "$hv" = "$ha" ] || { echo "the base initrd differs between the guests; the builds cannot agree" >&2; exit 1; }
step_stop
echo "both guests ready for $FORMAT (option $OPTION). next: bash e2e/cvm/flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh $FORMAT $OPTION"
