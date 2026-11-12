#!/usr/bin/env python3
"""재현 가능한 newc cpio 만들기: 입력 디렉터리 -> 항상 같은 바이트 (정렬, 시간 0, 소유자 0, 번호 고정)."""
import os, sys

root, out = sys.argv[1], sys.argv[2]
entries = []
for dp, dn, fn in os.walk(root):
    rel = os.path.relpath(dp, root)
    entries.append(('.' if rel == '.' else rel, dp, True))
    for f in fn:
        p = os.path.join(dp, f)
        entries.append((os.path.relpath(p, root), p, False))
entries.sort(key=lambda e: e[0].encode())


def pad(n):
    return b'\0' * ((4 - n % 4) % 4)


buf = bytearray()
for ino, (name, path, isdir) in enumerate(entries, 1):
    data = b'' if isdir else open(path, 'rb').read()
    mode = 0o040755 if isdir else (0o100755 if os.access(path, os.X_OK) else 0o100644)
    nm = name.encode() + b'\0'
    hdr = '070701' + ''.join('%08X' % v for v in (ino, mode, 0, 0, 2 if isdir else 1, 0, len(data), 0, 0, 0, 0, len(nm), 0))
    buf += hdr.encode() + nm + pad(110 + len(nm)) + data + pad(len(data))
nm = b'TRAILER!!!\0'
hdr = '070701' + ''.join('%08X' % v for v in (0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, len(nm), 0))
buf += hdr.encode() + nm + pad(110 + len(nm))
buf += b'\0' * ((512 - len(buf) % 512) % 512)
open(out, 'wb').write(buf)
