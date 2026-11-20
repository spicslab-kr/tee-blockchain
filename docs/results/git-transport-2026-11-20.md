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

**Status: partly run.** The failure paths ran against GitHub. The round trip (send, receive, the byte comparison,
`--ref` to the sent commit, a second send with the same content) **has not run**: it needs a private throwaway
repository, and none was available for this session (below, "Not run"). Two wrong exit codes found here were fixed
afterwards and run again ("After the fixes"); a third defect, in `send` run twice, is recorded and not fixed (8).

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
repository of the operator's, named here without its name.

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

## Not run

- **The round trip on GitHub:** `att-vendor send` into a private throwaway repository, `att-auditor receive` from it,
  the byte comparison of the received `build-config.json` with the pushed one, `receive --ref` to that commit, and a
  second `send` with the same content (on a local repository it does not give `nothing changed, nothing to commit`:
  8). No repository could be created here: `gh` is not installed on this machine, and the stored GitHub login was not
  to be read out for the GitHub API. The URL meant to be given for it did not arrive. It needs a private repository
  with at least one commit (7).
- **Exit 11** (push rejected). It needs a repository where the push is refused: a stale `--workdir` clone of the
  throwaway repository would do. Pushing to someone else's public repository to provoke it was not done on purpose.
- **SSH remotes** (`git@github.com:…`): only HTTPS was used.
- **Linux:** this ran on Windows only. The two wrong exit codes come from code that is the same on Linux
  (`syscall.Errno` has both methods there too), but they were not run there.
