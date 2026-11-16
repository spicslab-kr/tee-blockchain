#!/bin/sh
# Makes a GRUB entry for an app-fixture recording (docs/build/app-fixture-recording.md) from the stock Ubuntu entry,
# changing only the title, the initrd file name and, if given, appending kernel arguments. Makes it the default
# (GRUB_DEFAULT=<id>) rather than a one-shot grub-reboot entry, which a maintenance restart would lose.
#
# Usage, as root, with the stock entry saved as stock-entry.txt:
#   sh make-grub-entry.sh <id> <initrd file name> [extra kernel arguments]
# Writes /etc/grub.d/41_<id> and /etc/default/grub.d/99-attestamp.cfg, runs update-grub and prints the new entry.
set -eu
ID=$1
INITRD=$2
EXTRA=${3:-}
TEMPLATE=${TEMPLATE:-stock-entry.txt}
OUT=$(mktemp)
printf '#!/bin/sh\nexec tail -n +3 $0\n' > "$OUT"
sed -e "s/^menuentry 'Ubuntu'.*{\$/menuentry '$ID' --id $ID {/" \
    -e "s#initrd\.img-[^ ]*#$INITRD#" \
    -e "/^[[:space:]]*linux[[:space:]]/ s#\$#${EXTRA:+ $EXTRA}#" "$TEMPLATE" >> "$OUT"
install -m 755 "$OUT" "/etc/grub.d/41_$ID"
rm -f "$OUT"
echo "GRUB_DEFAULT=$ID" > /etc/default/grub.d/99-attestamp.cfg
update-grub
grep -n -A12 "menuentry '$ID'" /boot/grub/grub.cfg
