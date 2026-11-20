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
repository, and none was available for this session (below, "Not run").

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

## Exit codes, against `cmd/README.md`

| case | documented | got |
|---|---|---|
| repository not found (send, receive) | 9 | 9 |
| no credentials for the host | 9 | **1** |
| commit not found (`--ref`) | 10 | 10 |
| `git` not on PATH | 8 | 8 |
| push rejected | 11 | not run |
| a file that does not exist (`build-config.json` after the clone; `--config`) | 1 | **3** |
| empty repository (`send`, local) | none given | 10, with a misleading message |

## Not run

- **The round trip on GitHub:** `att-vendor send` into a private throwaway repository, `att-auditor receive` from it,
  the byte comparison of the received `build-config.json` with the pushed one, `receive --ref` to that commit, and a
  second `send` with the same content (expected: `nothing changed, nothing to commit`). No repository could be created
  here: `gh` is not installed on this machine, and the stored GitHub login was not to be read out for the GitHub API.
  It needs a private repository with at least one commit (7).
- **Exit 11** (push rejected). It needs a repository where the push is refused: a stale `--workdir` clone of the
  throwaway repository would do. Pushing to someone else's public repository to provoke it was not done on purpose.
- **SSH remotes** (`git@github.com:…`): only HTTPS was used.
- **Linux:** this ran on Windows only. The two wrong exit codes come from code that is the same on Linux
  (`syscall.Errno` has both methods there too), but they were not run there.
