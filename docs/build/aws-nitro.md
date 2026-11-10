# Reproducible build on AWS Nitro Enclaves: the same PCR0, PCR1 and PCR2

The Vendor and the Auditor each build the same source on their own instance. Whoever registers the
value, it can only be trusted if both builds give the same measurements. On Nitro Enclaves the
measurements are PCR0, PCR1 and PCR2. The short version is in the README under
[Build](../../README.md#build).

## The three values

| value | what it covers (AWS Nitro Enclaves documentation) |
|---|---|
| PCR0 | the whole enclave image file (EIF) |
| PCR1 | the Linux kernel and boot ramdisk that Nitro puts in the image |
| PCR2 | the application: your software's files and the responder |

In our measurements a different file in the image changed PCR0 and PCR2, and so did a different
`ENTRYPOINT` with the same files.

## Before building

| what | why | who checks it |
|---|---|---|
| same CPU architecture | `amd64` and `arm64` give different kernels, machine code and base layers, so different values. Each registration names its architecture | `receive` compares it with the Vendor's `build_env` |
| Go go1.26.1 (pinned) | a different patch version gives a different `att-ra-responder`, so different values | `build` and `pack` refuse another version; `att-vendor version` and `att-auditor version` show it |
| `-trimpath` | without it the build folder's path ends up inside the binary | `build` refuses a responder built without it; `receive` warns if the Vendor built without it |
| one `att-ra-responder` file | it is layered into the image | `build` checks its Go version, `-trimpath`, OS and architecture and prints its sha256 |
| same OS | part of `build_env` | `receive` compares it |

`receive` stops when OS, architecture or Go version differ from the Vendor's `build_env`.
`--ignore-env` turns that into a warning. Use it only to look around, since the values will not match.

## If you build without our `build`

Anything below that differs between the two builds changes the measurements. Without `build` you
have to take care of all of it. The sections after this one say which items `build` handles.

- the CPU architecture (`amd64` or `arm64`)
- the Go version and `-trimpath` of `att-ra-responder`, and the very same responder file on both sides
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
`pack` therefore export the files and do the following. Each item was found by measurement.

| step | what it removes |
|---|---|
| repack in name order, time 0, owner 0 (`tar --sort=name --mtime=@0 --owner=0 --group=0`), then restore `ENTRYPOINT`, `CMD`, `ENV` | file order and timestamps from `docker build` |
| empty `/tmp` and `/var/tmp` | random-named files left by `mktemp` (memcached) |
| remove package-manager logs: `/var/log/apk.log`, `dpkg.log`, `alternatives.log`, `apt`, `yum.log`, `dnf*.log`, `hawkey.log` | install times (nginx alpine-slim: only `apk.log` differed out of 725 files) |
| set apk virtual-package versions to 0 in `/etc/apk/world` and `/lib/apk/db/installed` | the install time used as a version (haproxy, memcached) |
| remove `/var/cache/ldconfig/aux-cache`, `/root/.wget-hsts`, `/root/.gnupg/random_seed`, `/root/.cache` | cache state: inodes, times, seeds (mariadb, tomcat) |

The software's own files and logs are not touched.

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

The build tools are not fixed either. `build` installs `docker` and `aws-nitro-enclaves-cli` with
`dnf` without a version, and `build-config.json` records only `os`, `arch`, the Go version and
`trimpath`. Use the same AMI on both sides and write these down:

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
| `e2e/responder-check.sh` | `build --responder` refuses a responder without `-trimpath` or for the wrong architecture |
| `e2e/build-env-check.sh` | `receive` rejects a `build_env` that does not match the instance; `--ignore-env` only warns |
| `e2e/arch-measurement-compare.sh` | the same source measures the same on two x86_64 hosts and differently on arm64 |

## If two builds differ

| what you see | where to look |
|---|---|
| `--verify` fails and lists files | a Dockerfile line reads the time, the host or randomness (table above) |
| `--verify` passes, but the other side's values differ | architecture, Go version, responder file, base image content, or tool versions |
| only PCR1 differs | the kernel files, probably a different `nitro-cli` version (not measured) |
| PCR0 and PCR2 differ | different files or a different `ENTRYPOINT` in the image |

## Settings that matter

| setting | what it does | can you leave it out |
|---|---|---|
| `-trimpath` (go build) | keeps the build folder path out of the binary | no for `att-ra-responder`: `build` refuses it |
| `--verify` (`build`) | builds twice and compares | yes, but then you do not know whether the build is reproducible |
| `--ignore-env` (`receive`) | turns a `build_env` mismatch into a warning | only to look around; the values will not match |
| `--build-measurements` (`launch`) | starts a received image only if it measures the same | yes, but then a swapped image starts unchecked |
| `--memory`, `--cpus` (`launch`) | the enclave's memory and CPU count | they are set when the enclave starts, not part of the build; the image must fit in the memory |
| `--arch` (`register`, `self-register`, `audit`) | names the architecture of the registration; not a build setting | it defaults to `build-config.json` or this machine |
