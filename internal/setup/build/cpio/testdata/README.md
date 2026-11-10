# Archives written by mkcpio.py

`cpio_test.go` checks that this package writes the same bytes as `option1-initrd/mkcpio.py`, the script the
October-November SEV-SNP runs used to pack the application. Each file here was written by that script.

| File | Tree (see `cpio_test.go`) | Where mkcpio.py ran | sha256 |
|---|---|---|---|
| `mkcpio-golden-app.cpio` | `appTree()`: `.`, `opt`, `opt/app`, `opt/app/app` (0755), `opt/app/README` (0644) | Ubuntu under WSL 2, Python 3.14.4, umask 022 | `739fccac1f8a95f4dc8cb9256bca97102f409cc84fba6464dc15f9d6c4e57aeb` |
| `mkcpio-golden-flat.cpio` | `flatTree()`: `.`, `app`, `b`, both 0755 | Windows, Python 3.14.6 | see `sha256sum` |

The flat tree exists because mkcpio.py is only correct on Windows for a tree with no subdirectories and only
executable files: there `os.path.relpath` joins with `\` and every existing file passes `os.access(X_OK)`.

To compare against the script live instead, point `ATTESTAMP_MKCPIO` at it:

```
ATTESTAMP_MKCPIO=/path/to/mkcpio.py go test -run Live ./internal/setup/build/cpio/
```
