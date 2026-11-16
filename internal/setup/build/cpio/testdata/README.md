# Archives written by mkcpio.py

The two `.cpio` files here were written by `mkcpio.py`, the script the October-November 2026 SEV-SNP runs used to pack
the application into the initrd, and committed on 2026-11-11 (`88fc9f6`). **That script no longer exists:** it lived outside this
repository, its author has left the lab, and it is not available anywhere (`docs/status/build-info-gaps.md`, #5).

So these files are now the only record of what it wrote, and their job is to pin that equivalence:
`TestWrite_MatchesMkcpioGolden` requires `internal/setup/build/cpio` to write the same trees byte for byte, and
`TestFromDir_MatchesMkcpioGolden` requires the same after the tree has been written to disk and read back. If a
change to the package makes either fail, the package no longer writes what the archives of those runs were. Do not
regenerate these files from the Go code to make a test pass; they are the reference, not the output.

| File | Tree (see `cpio_test.go`) | Where mkcpio.py ran | sha256 |
|---|---|---|---|
| `mkcpio-golden-app.cpio` | `appTree()`: `.`, `opt`, `opt/app`, `opt/app/app` (0755), `opt/app/README` (0644) | Ubuntu under WSL 2, Python 3.14.4, umask 022 | `739fccac1f8a95f4dc8cb9256bca97102f409cc84fba6464dc15f9d6c4e57aeb` |
| `mkcpio-golden-flat.cpio` | `flatTree()`: `.`, `app`, `b`, both 0755 | Windows, Python 3.14.6 | `a251d6e3beec7c95dee8fc3eb0e430d734981501b4d9dcd31066203dfe9acf25` |

The flat tree exists because mkcpio.py was only correct on Windows for a tree with no subdirectories and only
executable files: there `os.path.relpath` joined with `\` and every existing file passed `os.access(X_OK)`.

Removed on 2026-11-15: a test that ran mkcpio.py live (`ATTESTAMP_MKCPIO=<path>`), since there is no script left to
run. Its `FromDir` comparison is kept as `TestFromDir_MatchesMkcpioGolden`, which, like the old test, runs on Linux
and macOS only.
