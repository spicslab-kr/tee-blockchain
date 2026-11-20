# `send` and `receive` against a real remote, 2026-11-20

`att-vendor send` and `att-auditor receive` hand `build-config.json` from the Vendor to the Auditor through a git
repository. Before this run they had been run only against a local bare repository (`git init --bare`):

- by the Go tests: `cmd/buildenv_e2e_test.go`, and `internal/core/transfer/{Vendor,auditor}/git_test.go` and
  `git_common_test.go`;
- by four check scripts, on instances and guests: `e2e/build-env-check.sh`, `e2e/nitro-tools-check.sh`,
  `e2e/cvm/build-env-check.sh` and `e2e/arch-record-check.sh`, each with a `remote.git` in its work directory.

The flow scripts never call either command: they write `build-config.json` themselves and copy files with `scp`
(`e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh`, `e2e/flow2-vendor-self-registers/1-Vendor-build-self-register.sh`).
This run puts the two commands against GitHub. No TEE is needed.

**Status: run, apart from exit 11 and a second send on GitHub.** The failure paths ran against GitHub on 2026-11-20.
The round trip ran on 2026-11-21 (9): `send` pushed `build-config.json`, `receive` got the same bytes, and `--ref` picked
the sent commit. Two wrong exit codes found here were fixed and run again ("After the fixes"). Two more defects are
recorded and not fixed: `send` into an empty repository (7), and `send` of the same content twice (8). Exit 11 and
a second send against GitHub were not run ("Not run").

## Setup

- Repository at `dc94652`.
- Windows 11, Git Bash, `git version 2.46.0.windows.1`, `go version go1.26.1 windows/amd64`.
- Binaries built as in the README, "The binaries", with `.exe` names:
  `CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-vendor.exe ./cmd/vendor`, and the same
  for `./cmd/auditor`.
- git's credential helper is Git Credential Manager (`credential.helper=manager`, from the Git for Windows system
  config), holding a GitHub login.
- The Vendor's `build-config.json`, platform `intel-tdx`. `aws-nitro` was not used: `send` reads `ami_release`, `docker`
  and `nitro_cli` from the machine for it and refuses off a Nitro parent instance (`cmd/vendor/main.go`, `fillNitroEnv`).
  `tee_info` holds placeholders; `source` and `build_env` are empty, so `send` fills them in:

```json
{
  "schema": "attestamp.build-config/1",
  "service": "hello",
  "version": "1.0.0",
  "platform": "intel-tdx",
  "source": {
    "repo": "",
    "commit": ""
  },
  "tee_info": {
    "instance_name": "attestamp-tdx-vendor",
    "project_id": "example-project",
    "zone": "us-central1-a",
    "image": "ubuntu-2404-noble-amd64"
  },
  "build_env": {
    "os": "",
    "arch": "",
    "go": "",
    "trimpath": false
  }
}
```

The repositories used: `octocat/Hello-World` is GitHub's public example repository;
`octocat/attestamp-no-such-repo-20261120` does not exist. `<owner>/<private repository>` is an existing private
repository of the operator's, named here without its name: a private copy of this project, 260 commits. The round
trip (9) used it, with the operator's permission to add one commit of `build-config.json` at the root of `main` and
nothing else.

## What ran

Each command is as typed, from the directory holding the binaries; the Vendor's config is in `vendor/` below it. The
output is complete.

### 1. receive, a repository that does not exist: exit 9

```
$ ./att-auditor.exe receive --repo https://github.com/octocat/attestamp-no-such-repo-20261120 --into f1/src
att-auditor: cannot access the repository: check the URL and read permission, or ask the repository owner for access: remote: Repository not found.
fatal: repository 'https://github.com/octocat/attestamp-no-such-repo-20261120/' not found
exit=9
```

GitHub answers an unknown repository and one the signed-in user cannot read the same way (`Repository not found`), so
this is also what an Auditor without read access sees **when git has a GitHub login to offer**.

### 2. send, a repository that does not exist: exit 9

```
$ ./att-vendor.exe send --repo https://github.com/octocat/attestamp-no-such-repo-20261120 --config vendor/build-config.json
att-vendor: cannot access the repository: check the URL and read permission, or ask the repository owner for access: remote: Repository not found.
fatal: repository 'https://github.com/octocat/attestamp-no-such-repo-20261120/' not found
exit=9
```

### 3. receive with no credentials: exit 1, not 9

git's system and global configuration were switched off for this one command, which removes the credential helper.
That is the position of an Auditor with no login for the host:

```
$ GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null ./att-auditor.exe receive --repo https://github.com/<owner>/<private repository>.git --into f3/src
att-auditor: git: exit status 128: fatal: could not read Username for 'https://github.com': terminal prompts disabled
exit=1
```

The same with the repository that does not exist (`--into f2/src`) gave the same two lines and exit 1.

**Wrong exit code.** `cmd/README.md` gives 9 for "repository unreachable, URL or permissions". The cause:
`gitRunner.run` sets `GIT_TERMINAL_PROMPT=0` (`internal/core/transfer/*/git_common.go`), so git stops instead of
asking for a user name, and says `could not read Username … terminal prompts disabled`. None of `classify`'s patterns
match that text, so the error is unclassified and maps to 1. It is in both copies, `Vendor` and `auditor`, and so
affects `send` and `receive` of both binaries.

### 4. receive at a commit the repository does not have: exit 10

```
$ ./att-auditor.exe receive --repo https://github.com/octocat/Hello-World.git --ref 0123456789abcdef0123456789abcdef01234567 --into f4/src
att-auditor: the given commit is not in the repository: fatal: unable to read tree (0123456789abcdef0123456789abcdef01234567)
exit=10
```

### 5. receive, a repository with no build-config.json: exit 3, not 1

```
$ ./att-auditor.exe receive --repo https://github.com/octocat/Hello-World.git --into f5/src
att-auditor: https://github.com/octocat/Hello-World.git: open f5\src\build-config.json: The system cannot find the file specified.
exit=3
```

The clone of a real remote worked (as in 4, which cloned before checking out the commit).

**Wrong exit code, and not only here.** 3 is "network error" in `cmd/README.md`. `exitCode` in `cmd/vendor/main.go`,
`cmd/auditor/main.go` and `cmd/user/main.go` ends with `errors.As(err, &ne)` for a `net.Error`, and that test looks only
at methods. `syscall.Errno` has both `Timeout()` and `Temporary()`, so it satisfies `net.Error`, and `errors.As` finds
it under any `*fs.PathError`. **Every error a system call returns for a file**, not found, permission denied and the
rest, therefore exits 3 in all three binaries, wherever the file is read. A second case from this run, with no git
involved:

```
$ ./att-vendor.exe send --repo https://github.com/octocat/Hello-World.git --config vendor/no-such-config.json
att-vendor: open vendor/no-such-config.json: The system cannot find the file specified.
exit=3
```

### 6. receive with no git on PATH: exit 8

```
$ PATH=/nonexistent ./att-auditor.exe receive --repo https://github.com/octocat/Hello-World.git --into f6/src
att-auditor: git is not installed or not on PATH
exit=8
```

### 7. send into an empty repository: exit 10 (local bare repository, not GitHub)

Run to see what an empty remote does, before one existed on GitHub; the remote here is a local
`git init --bare --initial-branch=main empty.git`:

```
$ ./att-vendor.exe send --repo <scratch>/empty.git --config vendor/build-config.json
att-vendor: the given commit is not in the repository: fatal: ambiguous argument 'HEAD': unknown revision or path not in the working tree.
Use '--' to separate paths from revisions, like this:
'git <command> [<revision>...] -- [<file>...]'
exit=10
```

`cloneAt` has a branch for an empty repository, which returns no commit ("the send becomes its first commit"). The
Vendor's `Send` then runs `git rev-parse HEAD` itself, to fill in `source.commit`, and that fails on an unborn branch.
The branch is used by `att-auditor send` (the measurement record), whose `Send` commits without reading `HEAD` first;
that was not run here.

This is not a defect worth fixing as it stands, read from the code: `source.commit` names the source the Auditor builds,
and `att-auditor receive` refuses a config whose `source.commit` is not in the history it cloned
(`git merge-base --is-ancestor`, `internal/core/transfer/auditor/git.go`). An empty repository holds no source, so a
`build-config.json` sent into it could not be received even if `send` succeeded. What is wrong is the message: it says
"the given commit is not in the repository" where "the repository is empty: push the source first" is meant. The round
trip needs a repository with at least one commit.

### 8. send twice with the same content: no "nothing to commit" (local bare repository, not GitHub)

Run after the fixes below, with the binaries at `128dcd5` in `fixed/`, against a local bare repository `seeded.git`
holding one commit of one file (10b0843 there; the hashes in this section are that repository's, not this one's). First the config above, whose `source` is empty, twice:

```
$ fixed/att-vendor.exe send --repo <scratch>/seeded.git --config vendor/build-config.json
sent build-config.json → <scratch>/seeded.git @ c2469b5b69accc93efa2cc6d59c25f99a13e1946
build env: windows/amd64 go1.26.1 trimpath=true
exit=0
$ fixed/att-vendor.exe send --repo <scratch>/seeded.git --config vendor/build-config.json
sent build-config.json → <scratch>/seeded.git @ 01f92ec86502b2464bae787ef1dfd12113de471c
build env: windows/amd64 go1.26.1 trimpath=true
exit=0
```

The second is a new commit, not "nothing to commit": `send` fills an empty `source.commit` with the clone's `HEAD`, which
is now the first send's commit (`git show 01f92ec:build-config.json` has `"commit": "c2469b5…"`). So "the same config"
sent again is different content. With `source` written into the config (`vendor/pinned.json`: `source.commit` the seed
commit, `build_env` as `send` filled it in), twice:

```
$ fixed/att-vendor.exe send --repo <scratch>/seeded.git --config vendor/pinned.json
sent build-config.json → <scratch>/seeded.git @ 94598e934bd9d886a9ad2bc3ce4f8c98a9b93ae5
build env: windows/amd64 go1.26.1 trimpath=true
exit=0
$ fixed/att-vendor.exe send --repo <scratch>/seeded.git --config vendor/pinned.json
att-vendor: git: exit status 1: 
exit=1
```

**`ErrNothingToCommit` is never returned.** `git commit` with nothing staged writes `nothing to commit, working tree
clean` to **stdout**, and `classify` reads only stderr, which is empty: the error is unclassified, and its message ends
after the colon. No test covers `ErrNothingToCommit` in either copy. Not fixed here.

### 9. The round trip on GitHub, 2026-11-21

Binaries at `128dcd5` (`fixed/`, as in "After the fixes"); the code they are built from did not change after. The
private repository's `main` was at `dfd30be` and had no `build-config.json` at its root (`git ls-tree --name-only
origin/main`), so `send` added a file and changed none. `send` ran without `--workdir`, in its own temporary clone. The
Vendor's config is the one in "Setup".

**send**, from the binaries' directory:

```
$ fixed/att-vendor.exe send --repo https://github.com/<owner>/<private repository>.git --config vendor/build-config.json
sent build-config.json → https://github.com/<owner>/<private repository>.git @ 6e87e3f79511a02c82f2d683e836257c37df278a
build env: windows/amd64 go1.26.1 trimpath=true
exit=0
```

The pushed commit, seen from this repository after `git fetch origin` (`git show --stat`): parent `dfd30be`, message
`att: build-config for hello 1.0.0`, one file, `build-config.json | 22 ++++++++++++++++++++++`. Afterwards
`git ls-remote` showed `HEAD` and `refs/heads/main` at 6e87e3f and nothing else.

**receive**, from a separate directory `auditor/` next to `fixed/` (typed with the relative path up one
level and into `fixed/`; written `<bin>/` below, because such a path in these documents is read as one of the
repository's files):

```
$ <bin>/att-auditor.exe receive --repo https://github.com/<owner>/<private repository>.git --into work/src
received work\src\build-config.json (sha256 7fd8a7c02618)
hello 1.0.0 intel-tdx · source https://github.com/<owner>/<private repository>.git @ dfd30be305bc
  tee_info.image = ubuntu-2404-noble-amd64
  tee_info.instance_name = attestamp-tdx-vendor
  tee_info.project_id = example-project
  tee_info.zone = us-central1-a
build env: windows/amd64 go1.26.1 trimpath=true (Vendor)
build env: windows/amd64 go1.26.1 trimpath=true (this instance)
build env matches the Vendor's
next: att-auditor audit --config work\src\build-config.json --tee-addr <this-instance>:5000 --build-measurements out/measurements.json --rpc <url> --registry <0x…> --signing-key-file <key>
exit=0
```

The clone's `HEAD` was 6e87e3f. `send` filled `source.repo` with the URL, `source.commit` with the remote's `HEAD`
before the push (`dfd30be`), and `build_env` with the machine's; `receive` found that commit in the history it cloned.

**The byte comparison.** The pushed blob was taken without `att`: `git show 6e87e3f…:build-config.json` in this
repository after the fetch, saved as `pushed.json`.

```
$ cmp pushed.json auditor/work/src/build-config.json; echo "cmp exit=$?"
cmp exit=0
$ sha256sum pushed.json auditor/work/src/build-config.json vendor/build-config.json
7fd8a7c026180bd0fdc0e3b4100824ccdcefbc7feefd28c7ed70e4950e74315f *pushed.json
7fd8a7c026180bd0fdc0e3b4100824ccdcefbc7feefd28c7ed70e4950e74315f *auditor/work/src/build-config.json
5f055b2677c414180723b4d91fcab468ef23a30c0fb7b125826cfc8a83d03cb5 *vendor/build-config.json
```

**What was pushed and what was received are the same bytes.** The Vendor's input file differs from both, as it
should: `send` fills in `source` and `build_env` and writes `tee_info` with its keys sorted. The received file:

```json
{
  "schema": "attestamp.build-config/1",
  "service": "hello",
  "version": "1.0.0",
  "platform": "intel-tdx",
  "source": {
    "repo": "https://github.com/<owner>/<private repository>.git",
    "commit": "dfd30be305bc277580cbc2f3fe6ef0a991dd2ad9"
  },
  "tee_info": {
    "image": "ubuntu-2404-noble-amd64",
    "instance_name": "attestamp-tdx-vendor",
    "project_id": "example-project",
    "zone": "us-central1-a"
  },
  "build_env": {
    "os": "windows",
    "arch": "amd64",
    "go": "go1.26.1",
    "trimpath": true
  }
}
```

**`--ref` to the sent commit:**

```
$ <bin>/att-auditor.exe receive --repo https://github.com/<owner>/<private repository>.git --ref 6e87e3f79511a02c82f2d683e836257c37df278a --into work/src-ref
received work\src-ref\build-config.json (sha256 7fd8a7c02618)
hello 1.0.0 intel-tdx · source https://github.com/<owner>/<private repository>.git @ dfd30be305bc
  tee_info.instance_name = attestamp-tdx-vendor
  tee_info.project_id = example-project
  tee_info.zone = us-central1-a
  tee_info.image = ubuntu-2404-noble-amd64
build env: windows/amd64 go1.26.1 trimpath=true (Vendor)
build env: windows/amd64 go1.26.1 trimpath=true (this instance)
build env matches the Vendor's
next: att-auditor audit --config work\src-ref\build-config.json --tee-addr <this-instance>:5000 --build-measurements out/measurements.json --rpc <url> --registry <0x…> --signing-key-file <key>
exit=0
```

The clone's `HEAD` was 6e87e3f, and `cmp pushed.json auditor/work/src-ref/build-config.json` exited 0. The `tee_info`
lines come out in another order than in the first receive: `receive` prints them by ranging over a Go map, whose order
is random. Only the printout; the file is the same.

**`--ref` to the commit before the send**, which has no `build-config.json`, to see that `--ref` really picks the
commit:

```
$ <bin>/att-auditor.exe receive --repo https://github.com/<owner>/<private repository>.git --ref dfd30be305bc277580cbc2f3fe6ef0a991dd2ad9 --into work/src-before
att-auditor: https://github.com/<owner>/<private repository>.git: open work\src-before\build-config.json: The system cannot find the file specified.
exit=1
```

Exit 1, a file error, since the fix; at `dc94652` this was 3 (5).

## Exit codes, against `cmd/README.md`

| case | documented | got at `dc94652` | got at `128dcd5` (after the fixes) |
|---|---|---|---|
| repository not found (send, receive) | 9 | 9 | not run again |
| no credentials for the host | 9 | **1** | 9 |
| commit not found (`--ref`) | 10 | 10 | not run again |
| `git` not on PATH | 8 | 8 | not run again |
| push rejected | 11 | not run | not run |
| a file that does not exist (`build-config.json` after the clone; `--config`) | 1 | **3** | 1 |
| empty repository (`send`, local) | none given | 10, with a misleading message | not run again |
| the same content sent twice (local) | 1, "nothing changed, nothing to commit" | not run | 1, with an empty message (8) |
| the round trip: send, receive, `--ref` (GitHub) | 0 | not run | 0, 0, 0 (9) |
| `--ref` to a commit without `build-config.json` (GitHub) | 1 | not run | 1 (9) |

## After the fixes

Two commits fixed the two wrong codes, each with a test:

- `fix(core/transfer): classify a missing login as a repository access error`: `classify` takes `could not read
  Username`, `could not read Password` and `terminal prompts disabled` as `ErrRepoAccess`, in both copies
  (`TestClassify_NoCredentialsIsRepoAccess` in each `git_common_test.go`).
- `fix(cmd): stop file errors from exiting 3, the network error code`: in `exitCode` of all three binaries a
  `net.Error` that is a bare `syscall.Errno` no longer counts as a network error; a `*net.OpError` or `*net.DNSError`
  still exits 3 (`TestExitCode_FileErrorIsNotNetwork` in `cmd/{vendor,auditor,user}/exitcode_test.go`, which fails
  with exit 3 for the three file errors when the fix is taken out).

`att-ra-responder` links neither package (`go list -deps ./cmd/ra-responder`), so no measurement moves.

Built again at `128dcd5` into `fixed/`, the cases 3 and 5 again:

```
$ GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null fixed/att-auditor.exe receive --repo https://github.com/<owner>/<private repository>.git --into g3/src
att-auditor: cannot access the repository: check the URL and read permission, or ask the repository owner for access: fatal: could not read Username for 'https://github.com': terminal prompts disabled
exit=9
$ fixed/att-auditor.exe receive --repo https://github.com/octocat/Hello-World.git --into g5/src
att-auditor: https://github.com/octocat/Hello-World.git: open g5\src\build-config.json: The system cannot find the file specified.
exit=1
$ fixed/att-vendor.exe send --repo https://github.com/octocat/Hello-World.git --config vendor/no-such-config.json
att-vendor: open vendor/no-such-config.json: The system cannot find the file specified.
exit=1
```

## Fixed afterwards, 2026-11-21

The defects of 7, 8 and the printout of 9 were fixed after this record was written, each with a test that fails
without the fix (`docs/status/build-info-gaps.md`, row 39):

- `fix(core/transfer): say an empty repository needs the source first, on the Vendor's send` (7);
- `fix(core/transfer): treat a send of unchanged content as up to date` (8), both copies;
- `fix(cmd/auditor): print tee_info sorted by key in receive` (9).

The two `send` cases again, with `att-vendor` built after the second of these, in `fix3/` (7 with a build after the
first, in `fix2/`), against the same local bare repositories:

```
$ fix2/att-vendor.exe send --repo <scratch>/empty.git --config vendor/build-config.json
att-vendor: <scratch>/empty.git: the repository is empty: push the source the Auditor is to build first, then send build-config.json
exit=10
$ fix3/att-vendor.exe send --repo <scratch>/seeded.git --config vendor/pinned.json
already up to date: build-config.json in <scratch>/seeded.git @ 94598e934bd9d886a9ad2bc3ce4f8c98a9b93ae5 is this content; nothing new committed
build env: windows/amd64 go1.26.1 trimpath=true
exit=0
```

94598e9 is the commit the first send of `vendor/pinned.json` made in 8. Neither case was run again against GitHub.

## Not run

- **A second `send` of the same content on GitHub.** The repository of 9 is the operator's private copy of this
  project, and only one commit was to be added to it; a second send would add another whenever the content differs
  (8 shows that an empty `source` makes it differ). On a local repository it does not give `nothing changed, nothing
  to commit` (8).
- **Exit 11** (push rejected). It needs a repository the sender cannot write to; skipped on the operator's decision.
  Pushing to someone else's public repository to provoke it was not done on purpose.
- **A throwaway repository** created and deleted for the test: `gh` is not installed on this machine, and the stored
  GitHub login was not to be read out for the GitHub API. The operator's private repository was used instead (9).
- **SSH remotes** (`git@github.com:…`): only HTTPS was used.
- **Linux:** this ran on Windows only. The two wrong exit codes come from code that is the same on Linux
  (`syscall.Errno` has both methods there too), but they were not run there.
