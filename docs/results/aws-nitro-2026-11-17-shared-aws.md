# Nitro baseline after the shared Nitro request package, 2026-11-17 (second run)

`cc1be67` moved the Nitro request code into the shared `internal/tee/request/aws`, with thin role adapters, and
`att-ra-responder` links it, so the responder is another file and the values taken earlier the same day
([`aws-nitro-2026-11-17.md`](aws-nitro-2026-11-17.md), at `82cbe9b`) are stale for x86_64 and arm64 alike. `61f7515`
moved the Nitro build and launch the same way; the responder does not link them, and it was unchanged after that
commit (`d6e3422d…` / `f581d95c…`). This run takes the values again with the same scripts as the first run. Every
verdict is in [`aws-nitro-2026-11-17-shared-aws.jsonl`](aws-nitro-2026-11-17-shared-aws.jsonl), 32 lines: 31 written
by the scripts and copied unchanged from `e2e-work/results/aws-nitro.jsonl` (each script has its own `run` id), and
one written by hand (below). *(Added later on 2026-11-17.)* Six more lines follow them, 38 in all: a later
`responder-check.sh` run with longer SSH retry waits (last section).

## What ran, from which code

| | |
|---|---|
| code | binaries built at **`9ca51bb`** by `0-Setup-local-upload.sh`, and every script at the same commit; `9ca51bb` changes only `STRUCTURE.md` after `cc1be67` |
| `att-ra-responder` | sha256 **`b9ae17f365e59b0619398619e6fc59d309f5d27c5bfbadb7076df2fe6fb766d9`** (amd64) and **`294efd1b92716ae31b71b87b7c4b053d06ab44424089b5dd0995365db524e10e`** (arm64), go1.26.1, `CGO_ENABLED=0 -trimpath -buildvcs=false -ldflags=-buildid=`; it has been this since `cc1be67`. The upload printed `b9ae17f3…` on both x86_64 instances |
| x86_64 instances | two new `m6i.xlarge` (`attestamp-e2e`, eu-north-1), made with `aws-nitro.sh create --yes` |
| x86_64 image | `ami-0f71b90bee7050a75` (the same as in the first run): `PRETTY_NAME="Amazon Linux 2023.12.20261112"`, kernel `6.18.62-127.179.amzn2023.x86_64`, `aws-nitro-enclaves-cli-1.5.0-0.amzn2023` (`Nitro CLI 1.5.0`), `docker-25.0.18-1.amzn2023.0.3` (`Docker version 25.0.14`), `git-2.50.1-1.amzn2023.0.2`, on both |
| arm64 instance | one new `c7g.xlarge`, made with `aws-nitro.sh create-arm64 --yes` |
| arm64 image | `ami-0ce2f8dd9b0fcfb35` (the same as in the first run), the same release, kernel `6.1.190-237.442.amzn2023.aarch64`, the same nitro-cli, docker and git versions |
| chain | none (these scripts send no transaction; the `chain` field carries `NET`'s default) |
| network | a new security group allows ports 22 and 5000 from this machine's address only (`/32`); no `0.0.0.0/0` |
| state now | all three **stopped** with `aws-nitro.sh stop` (`describe-instances`: `stopped`), none terminated. *(Updated later on 2026-11-17.)* Started once more for the SSH test below, then **terminated** with `terminate-instances` (not `aws-nitro.sh delete`, which would also delete the security group). Each had one root volume with `DeleteOnTermination` true; afterwards the three volume IDs were not found, and every enabled region (17) had **0 instances and 0 volumes**. The security group and the key pair `attestamp-e2e` were kept for the next run |

## The reference values (test application)

`https://github.com/nginx/docker-nginx` at `ef5a25a6314e652a2dcfbce6074bef729f4d9639`, folder `mainline/alpine-slim`,
built with `att-vendor build`, **at commit `9ca51bb` with responder `b9ae17f3…` (amd64) / `294efd1b…` (arm64)**, on
2026-11-17:

| | x86_64 (Vendor and Auditor, two checkout paths: identical) | arm64 |
|---|---|---|
| PCR0 | `05662eae5e1a7a3b7fb035350df308b4e5a434bd4bb60377685daef0eba0c7dfebb382bbc06b63a636c5a9a1ff593f17` | `a653991e14780297477face5fe3ee02f23a1337d4116d6778bdb67c52b39fc62c69ae63a9de4888486e3dab925663b27` |
| PCR1 | `4b4d5b3661b3efc12920900c80e126e4ce783c522de6c02a2a5bf7af3a2b9327b86776f188e4be1c1c404a129dbda493` (the same as before: nitro-cli 1.5.0's kernel and init) | `3b4a7e1b5f13c5a1000b3ed32ef8995ee13e9876329f9bc72650b918329ef9cf4e2e4d1e1e37375dab0ba56ba0974d03` (the same as before) |
| PCR2 | `a3049e0dcb0bb04d1f4d205d8d61067a8d14a677154deee22d7f1eef8393fa67d7d0e85612d2f852f307bee04f021d5c` | `9660bc5e100201d469142bdc55e700f3acb74c4544c5ba62485b9c2f9e5ef7bca5f62fdef38a999aebd0727a570f596a` |

How they relate to the first run of the day, at `82cbe9b` (responder `d6e3422d…` / `f581d95c…`): x86_64 PCR0
`bfa0a428…`, PCR2 `aa36ce6f…`, arm64 PCR0 `4f94d9c9…`, PCR2 `79d0abea…`. PCR0 and PCR2 moved because the responder
changed (`cc1be67`); PCR1 did not, on either architecture. The images, kernels and tool versions were the same in both
runs, so the responder is the only difference.

These hold for this commit and this day. The Dockerfile starts `FROM alpine:3.24` (a tag) and installs `apk` packages
without versions, so a later day may give other PCR0/PCR2 at the same commit.

## Results

| script | row | result |
|---|---|---|
| `arch-measurement-compare.sh` | T1, T3 | x86_64: the Vendor and the Auditor (another checkout path) built **identical** PCR0/1/2; arm64 **differs** in all three, as expected. No SSH reset this time |
| `toolchain-pin-check.sh` | T2 | an `att-vendor` built with go1.26.2 is **refused** ("built with go1.26.2 but the project pins go1.26.1"); the pinned one passes; the responders **differ**: `b9ae17f3…` (go1.26.1) vs `fde4857c…` (go1.26.2) |
| `build-env-check.sh` | T7 | x86_64 and arm64: a config declaring the other architecture **refused** (both named), `--ignore-env` only warns, the own one accepted |
| `responder-check.sh` | T3 | without `-trimpath`, with VCS stamping, and with a Go build ID: each **refused** (exit 1, naming the reason); the correct one accepted, its sha256 `b9ae17f3…` in the build log; the arm64 responder **refused** by hand (below) |
| `nitro-tools-check.sh` | gap 27 | `send` recorded `ami_release` `2023.12.20261112`, `docker` `25.0.18-1.amzn2023.0.3`, `nitro_cli` `1.5.0-0.amzn2023`; `receive` passed the same release, **refused** another (exit 1, naming it) and only warned with `--ignore-env`; `build --config` passed the same versions and **stopped** on another docker right after `dnf install` (exit 1). All six as expected on the first run |
| `arch-record-check.sh` | T7 | `uname -m` maps `x86_64` → `amd64` and `aarch64` → `arm64`; `send` without `build_env` refused; the record carries `build_env`; `send --config --measurements` builds the record from the config; `register --arch x86_64` refused |

**The one line whose result is not the expected one, and why.** `responder-check.sh`,
`arm64-responder-refused-on-amd64`, `accepted`: step 7's SSH failed three times with `kex_exchange_identification:
read: Connection reset by peer`, so `build` never ran there and no exit code was printed ("exit ?"), the same step and
message as in every earlier run on this network. Step 7's remote command was then run by hand on the Vendor: refused,
exit 1, "att-ra-responder was built for GOARCH=arm64 but this instance is amd64; the enclave runs the parent
instance's architecture". That is the hand-written line (its `at` is approximate).

## The SSH resets: two fixes tried (added later on 2026-11-17)

Step 7 had to be run by hand on every run on this network (2026-11-12, -16, and both runs of -17). With the same three
instances started again (new addresses), two fixes were tried.

**ControlMaster (one shared SSH connection per host): does not work in Git Bash.** Git Bash's own OpenSSH 9.8p1
started the master and answered `ssh -O check` ("Master running"), but every session through it failed with
`mux_client_request_session: read from master failed` (`Connection reset by peer`, then `Software caused connection
abort`). With `ControlMaster=auto` the command failed (exit 255, "Failed to connect to new control master"); with
`ControlMaster=no` ssh quietly opened a new connection instead, so nothing was shared. The cause: a session through
the master hands its stdin, stdout and stderr to the master over the Unix socket (file descriptor passing), which
MSYS, under Git Bash, does not provide; Windows' own OpenSSH has no ControlMaster at all. Only the control commands,
which pass no descriptors, work. This is a known problem: not worth trying again on Windows; on Linux or macOS it
should work. `env.sh` has a comment saying so and no ControlMaster setting.

**Longer waits between attempts: passed once.** `net_retry` in `e2e/env.sh` now makes 4 attempts 10 s, 20 s and 40 s
apart (it was 3 attempts, 3 s and 6 s apart). Measured once, on 2026-11-17: `responder-check.sh` ran from start to end
and **step 7 passed without manual intervention**. Its first connection was reset again ("attempt 1 of 4; trying
again in 10 s"), the second got through, and `build` refused the arm64 responder, exit 1, "built for GOARCH=arm64 but
this instance is amd64". All six verdicts were as expected; they are the last six lines of the `.jsonl` (run
`20261117T095615Z`, copied unchanged), with responder `b9ae17f3…`, the scripts at `9ca51bb` plus this change to
`env.sh`. It is the first of the four runs in which step 7 needed no hand run, and it could not be repeated: the
instances were deleted afterwards. So this network resets a burst of new SSH connections, and on that one run ten
seconds were enough for it to let the next one through.
