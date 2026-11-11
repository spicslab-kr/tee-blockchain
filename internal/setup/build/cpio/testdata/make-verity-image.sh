#!/bin/sh
# Builds a reproducible read-only application image with a dm-verity hash tree, for the Option 2 app fixtures
# (docs/plan/app-fixture-recording.md). The settings are the ones docs/build/sev-snp.md, "Variables", found to give
# the same image and root hash on two machines:
#   - squashfs, not ext4 (ext4 differs between two builds even with a fixed UUID and hash_seed)
#   - mksquashfs -mkfs-time 0 -all-time 0, every file and directory time set to 0, owners root
#   - veritysetup format with a fixed --salt and --uuid (it picks both at random otherwise)
#   - umask 022 and explicit modes: folder modes 755 and 775 gave different root hashes, an open issue
# The image layout and the fixed parameters follow the 2026-11 script option2-dm-verity/mkverity.sh (outside this
# repository).
#
# Usage, as root on the guest:  sh make-verity-image.sh "<one line of application text>" <out dir>
# Writes <out dir>/app.img, <out dir>/app.hash and <out dir>/roothash, and prints their sha256 and the tool versions.
set -eu
umask 022
TEXT=$1
OUT=$2
WORK=$(mktemp -d)
mkdir -p "$WORK/opt/app" "$OUT"
printf '%s\n' "$TEXT" > "$WORK/opt/app/app"
chmod 755 "$WORK" "$WORK/opt" "$WORK/opt/app" "$WORK/opt/app/app"
touch -h -d @0 "$WORK/opt/app/app" "$WORK/opt/app" "$WORK/opt" "$WORK"
rm -f "$OUT/app.img" "$OUT/app.hash" "$OUT/roothash"
mksquashfs "$WORK" "$OUT/app.img" -noappend -no-xattrs -all-root -mkfs-time 0 -all-time 0 \
  -noI -noD -noF -noX -no-fragments -no-duplicates -processors 1 -b 4096 >/dev/null
SZ=$(stat -c %s "$OUT/app.img")
PAD=$(( (4096 - SZ % 4096) % 4096 ))
if [ "$PAD" -gt 0 ]; then head -c "$PAD" /dev/zero >> "$OUT/app.img"; fi
veritysetup format "$OUT/app.img" "$OUT/app.hash" --hash=sha256 --data-block-size=4096 --hash-block-size=4096 \
  --salt=00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff \
  --uuid=11111111-2222-3333-4444-555555555555 > "$WORK/verity.txt"
awk '/^Root hash:/ {print $3}' "$WORK/verity.txt" > "$OUT/roothash"
rm -rf "$WORK"
echo "umask $(umask)"
sha256sum "$OUT/app.img" "$OUT/app.hash"
echo "roothash $(cat "$OUT/roothash")"
mksquashfs -version | head -1
veritysetup --version
