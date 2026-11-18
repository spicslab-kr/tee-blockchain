# Reproducible build on AWS Nitro Enclaves: the same PCR0, PCR1 and PCR2

The Vendor and the Auditor each build the same source on their own instance. Whoever registers the
value, it can only be trusted if both builds give the same measurements. On Nitro Enclaves the
measurements are PCR0, PCR1 and PCR2. The short version is in the README under
[Build](../../README.md#build). What both sides must match, with a check for each: [reproduce.md](reproduce.md).
Tools, which step calls them, and the versions used: [toolchain.md](toolchain.md)
(the instance's AMI, kernel, docker and nitro-cli for each baseline). The reference PCRs of the test application, at
commit `6146c10`, x86_64 and arm64: [docs/results/aws-nitro-2026-11-17-shared-aws.md](../results/aws-nitro-2026-11-17-shared-aws.md)
(earlier, each at its own commit: `83cb2dc`, [aws-nitro-2026-11-17.md](../results/aws-nitro-2026-11-17.md); `23b83f9`,
[aws-nitro-2026-11-16.md](../results/aws-nitro-2026-11-16.md); the first baseline, at `ab55208`,
[aws-nitro-2026-11-12.md](../results/aws-nitro-2026-11-12.md)).

## The three values

| value | what it covers (AWS Nitro Enclaves documentation) |
|---|---|
| PCR0 | the whole enclave image file (EIF) |
| PCR1 | the Linux kernel and boot ramdisk that Nitro puts in the image |
| PCR2 | the application: your software's files and the responder |

One differing file in the image, `/etc/shadow`, gave a different PCR0 and PCR2
([aws-nitro-2026-11-15-local-chain.md](../results/aws-nitro-2026-11-15-local-chain.md)). That a different `ENTRYPOINT`
with the same files changes them too was seen in the work before this repository (imported in `dcd8e90`); no record of
that build was kept.

## Before building

| what | why | who checks it |
|---|---|---|
| same CPU architecture | `amd64` and `arm64` give different kernels, machine code and base layers, so different values. Each registration names its architecture | `receive` compares it with the Vendor's `build_env` |
| Go go1.26.1 (pinned) | a different patch version gives a different `att-ra-responder`, so different values | `build` and `pack` refuse another version; `att-vendor version` and `att-auditor version` show it |
| `-trimpath` | without it the build folder's path ends up inside the binary | `build` refuses a responder built without it; `receive` warns if the Vendor built without it |
| `-buildvcs=false` | without it the git commit, its time and whether the tree had uncommitted changes end up inside the binary (see below) | `build` refuses a responder that carries them |
| `CGO_ENABLED=0` | without it a native Linux build with a C compiler links cgo and gives other bytes than a cross-build (see below) | `build` refuses a responder built with cgo (since `e57f02d`) |
| `-ldflags=-buildid=` | without it Go writes a build ID, a hash of every input of the build, so a change to any package the responder links moves its bytes even when no code it runs changed (`1967749d`; see below) | `build` refuses a responder that carries a build ID (since 2026-11-17) |
| one `att-ra-responder` file | it is layered into the image | `build` checks its Go version, `-trimpath`, `-buildvcs=false`, `CGO_ENABLED=0`, the empty build ID, OS and architecture and prints its sha256 |
| same OS | part of `build_env` | `receive` compares it |

**Version control stamping, measured.** Go stamps `vcs.revision`, `vcs.time` and `vcs.modified` into main binaries by
default (since Go 1.18; `-buildvcs=auto`). `-trimpath` does NOT remove them. Measured on 2026-11-11 with go1.26.1,
`GOOS=linux GOARCH=amd64 go build -trimpath ./cmd/ra-responder` in a clone of this repository: identical Go source
committed under two different commits (`3b21225` and `e883b62`; the responder's packages, `go.mod` and `go.sum` do not
differ between them) produced two different binaries (`c46d2919…79a0` and `c2e433f2…e44f`), and a tree with
uncommitted changes (`e883b62` plus one line appended to `README.md`) produced a third (`11802de5…714f`). With
`-trimpath -buildvcs=false` the first and the third gave the same file (`82aba5f6…9207`). `go version -m` shows the
stamp, and with it a module version derived from the commit (`v0.0.0-20261111045839-3b21225a283f`). Also observed: a
`git worktree` checkout of the same commits (where `.git` is a file, not a directory) built with no stamp at all, so
whether a build is stamped depends on how the source was checked out. `-buildvcs=false` removes that dependence. The
record of these builds, with the full hashes, is the night log of that day, no longer tracked but kept in the history:
`git show 31ed378:docs/plan/night-log.md`, section "Decision 2026-11-11: `-buildvcs=false`, enforced". It names the
two commits as they were called before the history was rewritten on 2026-11-17.

**cgo, measured.** Go links cgo by default when the build is native (target OS and architecture equal the host's)
and a C compiler is on PATH. `att-ra-responder` imports `net`, so with cgo it carries the cgo DNS resolver and links
libc dynamically. Measured on 2026-11-15 in a clean Ubuntu 24.04 under WSL (go1.26.1, gcc from `build-essential`),
commit `d469e12`, the same `-trimpath -buildvcs=false` command:

| build | `go version -m` | sha256 of `att-ra-responder` |
|---|---|---|
| native Linux build, no `CGO_ENABLED` (the README's command before `e57f02d`) | `CGO_ENABLED=1` | `a71bedfc6b81fc5c535d29fc938c57398626e48fa885f18784c1b559d764a186` |
| the same machine with `CGO_ENABLED=0` | `CGO_ENABLED=0` | `5573b5381ac72b1812263b886d04b667854bcb60c5d84573e1fcc34f8124d668` |
| cross-build from Windows (no C compiler; cgo is off) | `CGO_ENABLED=0` | `5573b5381ac72b1812263b886d04b667854bcb60c5d84573e1fcc34f8124d668` |

Rebuilt at `e57f02d` on 2026-11-16 the three gave the same hashes again. So before `e57f02d` the PCR0 and PCR2 of an
image depended on whether the machine that built the responder was Linux with gcc. Since `e57f02d` every build
command sets `CGO_ENABLED=0` and `build` refuses a responder that records anything else. These hashes are for the code
at `d469e12` and `e57f02d`; later code gives other bytes (from `1967749d` on the `CGO_ENABLED=0` responder is
`c61c988e…`, below), and the cgo comparison itself still holds.

**The Go build ID, measured.** `1967749d` added one function to `internal/cli`, which the responder links but never
calls, and the responder went from `5573b538…` to `c61c988e…`: the linker drops the unused code, but the build ID Go
writes into the binary is a hash of every input of the build. Measured on 2026-11-17 (go1.26.1, linux/amd64 cross-builds
from Windows, `CGO_ENABLED=0 -trimpath -buildvcs=false`): with `-ldflags=-buildid=` the responder with and without that
function is the same file (`215423b8…`, at `7bdd4d0`). With an empty build ID a comment reworded in
`cmd/ra-responder/main.go` without moving a line, or appended after its last function, left the bytes as they were; a
comment that added a line above code did not (`4913a62c…`), because the binary keeps the line numbers of its code. So
comments can be fixed without moving the measurements as long as no line of code moves. The full record is
`docs/status/build-info-gaps.md`, Q1. Since 2026-11-17 every build command passes `-ldflags=-buildid=` and `build`
refuses a responder that carries a build ID; Go does not record `-ldflags` in the build info of a `-trimpath` binary, so
`build` reads the ELF note `.note.go.buildid`, which the linker leaves out when the build ID is empty
(`go tool buildid <file>` prints the same value; nothing is right).

`receive` stops when OS, architecture or Go version differ from the Vendor's `build_env`.
`--ignore-env` turns that into a warning. Use it only to look around, since the values will not match.

## If you build without our `build`

Anything below that differs between the two builds changes the measurements. Without `build` you
have to take care of all of it. The sections after this one say which items `build` handles.

- the CPU architecture (`amd64` or `arm64`)
- the Go version, `-trimpath`, `-buildvcs=false`, `-ldflags=-buildid=` and `CGO_ENABLED=0` of `att-ra-responder`, and the very same responder file on both sides
- the file order, timestamps and owners inside the Docker layers, which `docker build` decides anew on every run
- `ENTRYPOINT`, `CMD` and `ENV` of the image
- files left in `/tmp` and `/var/tmp`
- package-manager logs, which record the install time
- apk virtual-package versions, which are the install time
- build caches (`ldconfig` aux-cache, `.wget-hsts`, gnupg `random_seed`, `/root/.cache`)
- a base image by tag instead of by digest
- packages, dependencies and `git clone` without a fixed version or commit
- scripts run straight from the network (`curl … | sh`)
- the time, host name or random values written into a file
- compilers that embed the time, host or path
- the versions of the build tools (`docker`, `nitro-cli`), the OS and the kernel

## What `build` changes in the image

`docker build` decides file order and timestamps in each layer anew on every run. With identical file
contents the layer bytes still differ, and PCR0 and PCR2 with them. After `docker build`, `build` and
`pack` therefore export the files and do the following. Each item answers a build that came out different. The
`/etc/shadow` row is recorded (2026-11-15, [aws-nitro-2026-11-15-local-chain.md](../results/aws-nitro-2026-11-15-local-chain.md)).
The others were found in the work before this repository (imported in `dcd8e90`), in the images named in brackets, and
no record of those builds was kept; the code and its tests (`internal/setup/build/aws/normalize_scrub_test.go`) are
what remains.

| step | what it removes |
|---|---|
| repack in name order, time 0, owner 0 (`tar --sort=name --mtime=@0 --owner=0 --group=0`), then restore `ENTRYPOINT`, `CMD`, `ENV` | file order and timestamps from `docker build` |
| empty `/tmp` and `/var/tmp` | random-named files left by `mktemp` (memcached) |
| remove package-manager logs: `/var/log/apk.log`, `dpkg.log`, `alternatives.log`, `apt`, `yum.log`, `dnf*.log`, `hawkey.log` | install times (nginx alpine-slim: only `apk.log` differed out of 725 files) |
| set apk virtual-package versions to 0 in `/etc/apk/world` and `/lib/apk/db/installed` | the install time used as a version (haproxy, memcached) |
| remove `/var/cache/ldconfig/aux-cache`, `/root/.wget-hsts`, `/root/.gnupg/random_seed`, `/root/.cache` | cache state: inodes, times, seeds (mariadb, tomcat) |
| empty field 3 ("last password change", days since 1970) of every line in `/etc/shadow` and `/etc/shadow-` (since `a402827`, 2026-11-15) | the **build day**, which `adduser` and `useradd` write. Measured: nginx alpine-slim built from a 2026-11-12 cache had `nginx:!:20769:…`, rebuilt on 2026-11-15 `20772`, and that one field changed PCR0 and PCR2. Empty is what the distributions' own base accounts carry; an enclave has no login, so password ageing does nothing there |

The software's own files and logs are not touched.

**This changes the measurements.** Every image with an account `adduser` made, the test application included, has
a different PCR0 and PCR2 from `a402827` on (PCR1, the kernel and init, does not change). PCR values recorded before
that commit are marked "before `a402827`" where they appear.

**The Nitro reference values were taken again later on 2026-11-17, at `6146c10`**
(`docs/results/aws-nitro-2026-11-17-shared-aws.md`), after the Nitro request code moved into a shared package
(`f0fb80d`). Earlier that day at `83cb2dc` (`docs/results/aws-nitro-2026-11-17.md`), after the empty build ID. Before
that, on 2026-11-16 at `23b83f9` (`docs/results/aws-nitro-2026-11-16.md`), after
three commits had made the earlier ones stale:

| commit | what it changes | effect on the test application's PCR0/PCR2 |
|---|---|---|
| `a402827` | `build` empties the build day in `/etc/shadow` (above) | moved |
| `e57f02d` | every build is `CGO_ENABLED=0` ("cgo, measured") | none on the recorded values: they were already built with the cgo-free responder `5573b538…` |
| `1967749d` | adds a function to `internal/cli`, which the responder links but does not call | moved: the responder became `c61c988e…` (`docs/build/ra-comparison.md`, "What covering the responder costs") |

Then two more, which is why they were taken again on 2026-11-17:

| commit | what it changes | effect on the test application's PCR0/PCR2 |
|---|---|---|
| `45887c8` | every build passes `-ldflags=-buildid=` (above, "The Go build ID, measured") | moved: the responder became `215423b8…` (at `7bdd4d0`) |
| `ec14e63` | corrects `cmd/ra-responder`'s comments and its printed usage text | moved: the responder became `d6e3422d…` (amd64), `f581d95c…` (arm64) |

And then one more, which is why they were taken a second time on 2026-11-17:

| commit | what it changes | effect on the test application's PCR0/PCR2 |
|---|---|---|
| `53bfd5e` | moves the Nitro build and launch into shared packages | none: the responder does not link them, and stayed `d6e3422d…` / `f581d95c…` |
| `f0fb80d` | moves the Nitro request code, which the responder links, into the shared `internal/tee/request/aws` | moved: the responder became `b9ae17f3…` (amd64), `294efd1b…` (arm64) |

**The current values**, at `6146c10`, test application `docker-nginx` `ef5a25a6` `mainline/alpine-slim`, responder
`b9ae17f3…` (amd64) / `294efd1b…` (arm64), 2026-11-17: x86_64, two machines identical: PCR0 `05662eae…`, PCR1
`4b4d5b36…`, PCR2 `a3049e0d…`. On arm64 (`c7g.xlarge`) all three differ: PCR0 `a653991e…`, PCR1 `3b4a7e1b…`, PCR2
`9660bc5e…`. PCR1 is the same as on 2026-11-16 on both (nitro-cli 1.5.0). The values at `83cb2dc` (responder
`d6e3422d…` / `f581d95c…`, the same images) were x86_64 PCR0 `bfa0a428…`, PCR2 `aa36ce6f…` and arm64 PCR0 `4f94d9c9…`,
PCR2 `79d0abea…`; at `23b83f9` (responder `c61c988e…`) x86_64 PCR0 `0c905480…`, PCR2 `6347a146…` and arm64 PCR0
`f1d4d679…`, PCR2 `6e865138…`. **Every Nitro value is written next to the commit
it was measured at**: the responder's bytes, and so PCR0 and PCR2, changed with any change to a package the responder
links until 2026-11-17, and still change with any change to code it can run or to the lines that code sits on, so a
value without its commit cannot be compared.

### Two builds compared (2026-11-15)

To find whatever else of this kind is left, the test application (`docker-nginx` `ef5a25a6`, `mainline/alpine-slim`)
was built twice with `docker build --no-cache` on one machine (Docker Desktop 29.8.0, BuildKit), both root file
systems were exported, and every file's type, mode, link target and sha256 compared (1,197 entries each). Then build A
was searched for the day number (`:20772:`), the epoch-seconds prefix (`17947`), the date as `2026-11-15`, `20261115`,
`Nov 15` and `15/Nov/2026`, and the build host name (`buildkitsandbox`).

| where | what | handled |
|---|---|---|
| `/var/log/apk.log` | the install time, on every line; the **only file that differed** between the two same-day builds | yes, removed |
| `/etc/shadow` | the build day (`:20772:`); same within a day, so the two-build comparison cannot show it; the day-number search did | yes, since `a402827` |
| anything else | no other file held the date, the day, the epoch seconds or the host name | — |

`/etc/hostname`, `/etc/hosts` and `/etc/resolv.conf` are in the export as empty files docker created; empty, so the
same every time. What this does **not** cover: other Dockerfiles (each must be checked with `--verify` and, across
days, by a rebuild on another day), a different builder (the instances use Amazon Linux's docker, not BuildKit), and
values that change between machines rather than between runs.

## What `build` cannot change

`build` scans the Dockerfile and prints each risky line with its line number, then goes on.
`--verify` shows whether the line actually changed anything.

| in the Dockerfile | what to do |
|---|---|
| `FROM image:tag` | pin it: `FROM image@sha256:…`. A tag can move between the two builds |
| `apk add`, `apt-get install`, `dnf install` without versions | give exact versions (`name=1.2.3`) |
| `pip`, `npm`, `yarn`, `gem`, `cargo`, `go get` | use a lockfile or exact versions |
| `git clone` | check out one commit |
| `curl … \| sh` | do not run a script that can change |
| `date`, `hostname`, `uname -n` | do not write the time or the host name into a file |
| `$RANDOM`, `/dev/urandom`, `openssl rand`, `uuidgen`, `ssh-keygen` | do not generate random values or keys at build time |
| `mktemp` outside `/tmp` and `/var/tmp` | keep scratch files in those two folders |
| `make`, `gcc`, `cmake`, `cargo build`, `go build` | compilers can embed the time, host or path (redis embeds both in `redis-server`); check with `--verify` |
| `adduser`, `useradd` setting a **password** | the build day in `/etc/shadow` is handled (above); a password hash is not: its salt is random, so the image differs on every build. Do not set a password at build time |

The build tools are not fixed either. `build` installs `docker` and `aws-nitro-enclaves-cli` with
`dnf` without a version. Since 2026-11-17 `att-vendor send` records, next to `os`, `arch`, the Go version and
`trimpath`, the Amazon Linux release (`ami_release`) and both package versions (`docker`, `nitro_cli`), and the
Auditor checks them twice. `receive`, before anything is installed, compares the release: a proxy, not a guarantee: each Amazon Linux release has its own package repository, but a `dnf update` with another release, `releasever=latest` in `/etc/dnf/vars`, or a package installed by hand moves the versions without moving the release.
`build --config <the Vendor's build-config.json>`, right after `dnf install`, compares the versions themselves and
stops before building anything if either differs; that is the real check. Run on two instances on 2026-11-17
(`e2e/nitro-tools-check.sh`, `docs/results/aws-nitro-2026-11-17.md`): all six verdicts as expected. Use the same AMI on both sides; to see the values by hand:

```
rpm -q docker aws-nitro-enclaves-cli
uname -r
cat /etc/os-release
```

`nitro-cli` makes the enclave image and supplies the kernel files that PCR1 covers, so a different
version may give a different PCR1. **We have not measured this.**

## Checking

Two equal builds on one instance do not prove that another instance gets the same values. So the
other side still rebuilds on its own instance and compares: the Auditor in flow 1, or an Auditor
checking a Vendor's self-registration later in flow 2.

| check | what it shows |
|---|---|
| `build --verify` (Vendor and Auditor) | builds the same Dockerfile again with `--no-cache`. Success: `✓ reproducible on this host`. Failure lists the files that differ and exits with an error |
| Auditor's own `build` | the only proof that another host gets the same values. Equal builds on one host do not prove it |
| `launch --build-measurements <file>` | measures a received image and starts it only if the values match the file |
| `e2e/toolchain-pin-check.sh` | an `att` binary built with another Go patch version refuses to `build` |
| `e2e/responder-check.sh` | `build --responder` refuses a responder without `-trimpath`, with version control stamping, with a Go build ID, or for the wrong architecture |
| `e2e/build-env-check.sh` | `receive` rejects a `build_env` that does not match the instance; `--ignore-env` only warns |
| `e2e/nitro-tools-check.sh` | `send` records the Amazon Linux release and the docker and nitro-cli versions; `receive` refuses another release; `build --config` refuses another docker version right after installing |
| `e2e/arch-measurement-compare.sh` | the same source measures the same on two x86_64 hosts and differently on arm64 |

## If two builds differ

| what you see | where to look |
|---|---|
| `--verify` fails and lists files | a Dockerfile line reads the time, the host or randomness (table above) |
| `--verify` passes, but the other side's values differ | architecture, Go version, responder file, base image content, or tool versions |
| only PCR1 differs | the kernel files, probably a different `nitro-cli` version (not measured) |
| PCR0 and PCR2 differ | different files or a different `ENTRYPOINT` in the image |

## Settings that matter

(The settings of every platform, and the hashes used, in one place: [settings-and-hashes.md](settings-and-hashes.md).)

| setting | what it does | can you leave it out |
|---|---|---|
| `-trimpath` (go build) | keeps the build folder path out of the binary | no for `att-ra-responder`: `build` refuses it |
| `-buildvcs=false` (go build) | keeps the git commit, its time and the uncommitted-changes flag out of the binary | no for `att-ra-responder`: `build` refuses it |
| `CGO_ENABLED=0` (environment of go build) | keeps cgo out, so a native Linux build gives the same file as a cross-build | no for `att-ra-responder`: `build` refuses it |
| `-ldflags=-buildid=` (go build) | keeps Go's build ID, a hash of every input of the build, out of the binary | no for `att-ra-responder`: `build` refuses it |
| `--verify` (`build`) | builds twice and compares | yes, but then you do not know whether the build is reproducible |
| `--ignore-env` (`receive`) | turns a `build_env` mismatch into a warning | only to look around; the values will not match |
| `--build-measurements` (`launch`) | starts a received image only if it measures the same | yes, but then a swapped image starts unchecked |
| `--memory`, `--cpus` (`launch`) | the enclave's memory and CPU count | they are set when the enclave starts, not part of the build; the image must fit in the memory |
| `--arch` (`register`, `self-register`, `audit`) | names the architecture of the registration; not a build setting | it defaults to `build-config.json` or this machine |

## Ports, vsock address and enclave resources

These values were only in the code (`internal/setup/{build,launch}/*/aws`, `cmd/{vendor,auditor}/main.go`). Defaults,
and where each one goes:

| value | default | set by | where it ends up |
|---|---|---|---|
| RA port inside the enclave | 5000 | `build`/`pack --port` | `ENV ATT_RA_PORT=<port>` in the image `build` layers on the software; `entrypoint.sh` starts `ra-serve --listen vsock:${ATT_RA_PORT:-5000} --extractor nsm`. It is in the image, so the Vendor and the Auditor must use the same `--port` (whether a different port changes PCR0/PCR2 was not measured) |
| RA port on the parent instance | the same port | `launch --port` ("must match the value given to build or pack") | `ra-proxy --listen <listen>:<port> --vsock <cid>:<port>`, started detached by `launch`; its log is `out/proxy.log` |
| listen address of the bridge | `0.0.0.0` | `launch --listen` | the TCP side of `ra-proxy`. The security group is a separate matter: see "Opening the RA port" below |
| source the RA port is opened to | **none**: nothing is opened | `launch --open-port-to <CIDR>` | the instance's first security group, only when given and when the instance role allows `ec2:AuthorizeSecurityGroupIngress`; see below |
| enclave vsock CID | 16 | `launch --cid` | `nitro-cli run-enclave --enclave-cid`, and the target of `ra-proxy --vsock` (default `16:5000`) |
| enclave CPUs | 2 | `launch --cpus` | `nitro-cli run-enclave --cpu-count`; `launch` raises `cpu_count` in `/etc/nitro_enclaves/allocator.yaml` if it is lower |
| enclave memory | four times the EIF size, rounded up to 512 MiB, at least 512 MiB (`MemoryFor`) | `launch --memory <MiB>` | `nitro-cli run-enclave --memory`; `launch` raises `memory_mib` in the allocator file if it is lower. Raising the allocator stops any running enclave |
| responder default when run by hand | `tcp:127.0.0.1:5000` | `att-ra-responder ra-serve --listen` | only for a manual run; inside the EIF the entrypoint always passes `vsock:` |

CID, CPUs and memory are given when the enclave starts and are not part of the measured image.

## Opening the RA port

Users and the Auditor reach the enclave through the bridge's TCP port on the parent instance, so the instance's
security group must allow that port from them. `launch` never widens the group on its own (changed 2026-11-12; until
then it opened the port to `0.0.0.0/0` whenever the instance role allowed it, without saying so beforehand):

| `launch` run with | what happens to the security group |
|---|---|
| nothing | **nothing changes.** `launch` says which sources the group allows on the port now (when the role may read the group) and prints the `aws ec2 authorize-security-group-ingress` command, with `<verifier-ip>/32` as the source |
| `--open-port-to 203.0.113.7/32` | that one source is added, if the role allows it and the group does not already have exactly that rule. The log line starts with `OPENED` and names the group, region, port, source and time |
| `--open-port-to 0.0.0.0/0` | the port is opened to everyone, because that is what was written; the log says so and prints the `revoke-security-group-ingress` command to undo it |
| a bare address, a CIDR with host bits set, or IPv6 | refused before anything starts, with the CIDR it probably meant |
| a source, but no permission | nothing changes; the command for that source is printed, and the console path |

Public access is allowed by the design: anyone may verify an enclave, and the evidence is not secret. But opening to
everyone has to be a decision someone writes down, not a side effect, and the usual source is the verifier's address.
The confidential VMs follow the same rule: `docs/build/tdx.md` and `docs/build/sev-snp.md` say to allow port 5000
only from the verifier's address, through the `attestamp-ra` firewall rule.

The e2e instances made by `e2e/cloud/aws-nitro.sh create` already allow ports 22 and 5000 from the creating
machine's address only, so the e2e scripts need no `--open-port-to`.
