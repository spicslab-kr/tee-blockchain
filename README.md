# tee-blockchain

Remote attestation for software running in a TEE (currently AWS Nitro Enclaves), with the
attested measurements registered on an Ethereum-compatible chain so anyone can look them up
and verify them later.

**Vendor** runs the software in an enclave. **Auditor** builds the same source independently,
runs it in its own enclave, and registers the measurements it vouches for on-chain. **User**
asks the Vendor's enclave to attest itself, then checks that result against what's on-chain
before trusting it.

Each role gets its own binary (`tb-vendor`, `tb-auditor`, `tb-user`). A role's binary only
contains that role's commands — User's binary has no `register` at all. The chain-side work of
each role collapses to one `run` command; the steps it wraps are listed under each role so you
can see exactly what it does, or drive them by hand.

## 0. Build the binaries and put them where they run

```
go build -o tb-vendor  ./cmd/vendor
go build -o tb-auditor ./cmd/auditor
go build -o tb-user    ./cmd/user
go build -o tb-ra-responder ./cmd/ra-responder
```
Requires Go 1.26+. Cross-compile for the Nitro instances (Linux) with `GOOS=linux GOARCH=amd64`.

- **Vendor's Nitro instance**: `tb-vendor` **and** `tb-ra-responder`, in the same directory.
- **Auditor's Nitro instance**: `tb-auditor` **and** `tb-ra-responder`, in the same directory.
- **User**: `tb-user`, anywhere with network access — no Nitro, no Docker, no keys.

`tb-ra-responder` is the program that answers attestation requests *inside* the enclave. `build`
layers it onto the software image, so it must be present next to the role binary. It is one
shared binary for every role, and both instances must get **the same file** — if Vendor and
Auditor embed different responders their measurements can never match. `build`, `launch` and
the responder only work on a Nitro-enabled EC2 instance (Linux); everything else runs anywhere.

## 1. Vendor

### 1.1 Build and launch the enclave — Case A or Case B

Both cases must end with the same three things, because that is all the rest of the system uses:

| what | where it goes |
|---|---|
| the enclave image (`.eif`) | `launch`, and the M1 hand-off |
| the measurements, nitro-cli's JSON (`{"Measurements":{"PCR0":…,"PCR1":…,"PCR2":…}}`) | `run --measurements`, `register --measurements`, `compare --b` |
| the RA address `host:port` the enclave answers on | `--tee` for Auditor and User |

Building is optional for the Vendor — an EIF received from the Auditor works too (see 1.3); then
only the launch half applies.

#### Case A — you do it yourself

The design's "document it" default: your Dockerfile, your tools; you write the three values down.

Build the software image as you normally would:
```
docker build -t app:base <dir>
```

Layer the responder on top and make it the entrypoint. In a scratch directory put `tb-ra` (a
copy of `tb-ra-responder`), `entrypoint.sh` (copy `internal/setup/build/Vendor/aws/entrypoint.sh`
— it starts the responder on vsock, then execs the software's original command), and this
Dockerfile:
```
FROM app:base
COPY tb-ra /tb-ra
COPY entrypoint.sh /tb-entrypoint.sh
ENV TB_RA_PORT=5000
ENTRYPOINT ["/tb-entrypoint.sh"]
```
```
docker build -t app:tb .
```

If an Auditor is going to rebuild this and compare, normalize the image now or the measurements
will differ: export the container filesystem, drop package-manager logs (`/var/log/apk.log`,
`dpkg.log`, `apt/`, `dnf*.log` — they embed the build time), empty `/tmp` and `/var/tmp`, pin apk
virtual-package versions (`.name=20260927.082249` in `/etc/apk/world` is the install time), repack with
`tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner`, and `docker import` it under
the same tag with the same ENTRYPOINT/ENV/CMD. `build` does exactly this; `setup/vendor/build.md`
spells it out. Skip it only if nobody will ever rebuild.

Make the EIF. nitro-cli's output *is* your measurements file — keep it:
```
nitro-cli build-enclave --docker-uri app:tb --output-file app.eif > measurements.json
```

Run the enclave:
```
nitro-cli run-enclave --eif-path app.eif --cpu-count 2 --memory <MiB> --enclave-cid 16
```

Bridge TCP on the parent instance to the enclave's vsock (this is what `--tee host:port` talks to):
```
tb-vendor ra-proxy --listen 0.0.0.0:5000 --vsock 16:5000 &
```

Open the port so Auditor and User can reach it:
```
aws ec2 authorize-security-group-ingress --group-id <sg> --protocol tcp --port 5000 --cidr 0.0.0.0/0
```

Check it answers. The RA address is now `<public-ip>:5000`:
```
tb-user ra-request --tee <public-ip>:5000 --format aws-nitro --out t.cbor
```

#### Case B — let tb do it

Same result, two commands, on the Nitro instance.

Build. Installs docker/nitro-cli, builds the image, layers the responder, normalizes the layers,
makes the EIF, and writes the measurements — `out/<name>.eif` and `out/measurements.json`:
```
tb-vendor build --app-dir <dir with Dockerfile> --out out
```

Launch. Starts the enclave, bridges TCP 5000 → vsock, requests an attestation from itself to
prove it answers (`out/selfcheck.cbor`), and opens the security-group port — or, without IAM
permission on the instance, prints the one `aws ec2` line to run from your account:
```
tb-vendor launch --eif out/<name>.eif
```

### 1.2 `run` — attest my enclave, register, self-check

In flow 1 (the Auditor audits and registers) the Vendor does **not** register — after 1.1 it is
done. `run` is for flow 2, where the Vendor registers its own enclave:
```
tb-vendor run --tee 127.0.0.1:5000 --rpc <url> --contract <0x…> --key-file <key>
```

What that does, one step at a time.

T1 — send a fresh nonce to the enclave, get back the signed attestation document:
```
tb-vendor ra-request --tee 127.0.0.1:5000 --out doc.cbor
```

T2 — verify the signature chain up to the platform root and the nonce, extract PCR0·1·2:
```
tb-vendor ra-parse --doc doc.cbor --out parsed.json
```

C1 — sign those measurements with my key and register them on the contract. The `key` it prints
is the lookup coordinate:
```
tb-vendor register --rpc <url> --contract <0x…> --key-file <key> --measurements parsed.json --service <name> --version <v>
```

C2 — read the registration back by measurement alone:
```
tb-vendor lookup --rpc <url> --contract <0x…> --measurements parsed.json
```

M2 — self-check: does what's on-chain match what the enclave attested, is it unexpired, is the
signer the contract's approver:
```
tb-vendor compare --a parsed.json --rpc <url> --contract <0x…> --key <key register printed>
```

Options. Add `--measurements out/measurements.json` to `run` and it compares the attestation
against your build value first and refuses to register on a mismatch (exit ≠ 0, the differing
PCRs are printed). `--service`/`--version` are asked for on the first registration; later ones
reuse them. Any flag you leave out is asked for interactively — except `--rpc`, which defaults to
Ethereum **mainnet** and requires typing `y` to accept; any other URL (a testnet) is taken as-is.

### 1.3 Hand-off with the Auditor (M1)

Simplest: tell the Auditor the repository and the commit. That is all it needs to rebuild.

Or hand it over as a file. `send` writes `build-config.json` (source repo/commit/path plus the
instance's id/region/AMI) to the root of the repository and pushes it:
```
tb-vendor send --repo <git url> --config build-config.json
```

Later, when the Auditor has published its result, fetch it — `measurement-record.json` and the
EIF, with their hashes verified:
```
tb-vendor receive --repo <git url> --into from-auditor
```

## 2. Auditor

### 2.1 Get the source

Simplest: `git clone` the repository and `git checkout <commit>` the Vendor named.

Or, if the Vendor used `send`, this clones the repository, reads `build-config.json`, and checks
that the commit it names really is in the history:
```
tb-auditor receive --repo <git url> --into work/src
```

### 2.2 Build and launch — Case A or Case B

Identical to the Vendor's 1.1 with `tb-auditor` in place of `tb-vendor`. Reproducibility is the
whole point here: same source + same `tb-ra-responder` + normalized layers ⇒ the same PCR0·1·2 the
Vendor got. If they differ, that is the finding — do not register.

```
tb-auditor build --app-dir work/src/<path> --out out
```
```
tb-auditor launch --eif out/<name>.eif
```

### 2.3 Deploy the registry — once, first time only

One contract = one approver: only registrations signed by that address are accepted, forever.
The deployer key only pays gas; it can be anyone's. It prints the contract address — everyone
uses it:
```
tb-auditor registry-deploy --rpc <url> --key-file <deployer.key> --approver <my 0x…>
```

### 2.4 `run` — attest my enclave, check it against my build, register, self-check

```
tb-auditor run --tee <my-instance>:5000 --measurements out/measurements.json --rpc <url> --contract <0x…> --key-file <key>
```
`--tee` is **my own** enclave, running **my own** build of the Vendor's source. That is the
audit: what goes on-chain is what I built from the source and ran myself, checked against the
build value, not what the Vendor told me.

What that does, one step at a time.

T1 — attestation document from my enclave:
```
tb-auditor ra-request --tee <my-instance>:5000 --out doc.cbor
```

T2 — verify it, extract PCR0·1·2:
```
tb-auditor ra-parse --doc doc.cbor --out parsed.json
```

M2 — the audit gate: the enclave's attested value must equal my build value. FAIL means the
enclave is not running what I built; nothing gets registered:
```
tb-auditor compare --a parsed.json --b out/measurements.json
```

C1 — register the **build value** with my key:
```
tb-auditor register --rpc <url> --contract <0x…> --key-file <key> --measurements out/measurements.json --service <name> --version <v>
```

C2 — read it back by measurement:
```
tb-auditor lookup --rpc <url> --contract <0x…> --measurements out/measurements.json
```

M2 — self-check against the chain:
```
tb-auditor compare --a parsed.json --rpc <url> --contract <0x…> --key <key>
```

Pointing `--tee` at the **Vendor's** enclave instead is flow 2's "something looks off" check
(attest what the Vendor is actually running and compare it to my build) — fine as an extra
step, but flow 1 registers from my own enclave. Same optional flags and prompts as 1.2.

### 2.5 Hand the result to the Vendor (M1)

`measurement-record.json` carries the service/version, the PCRs and the build tool version;
`send` fills in the EIF's sha256 and pushes both to the repository:
```
tb-auditor send --repo <git url> --record measurement-record.json --image out/<name>.eif
```

## 3. User

```
tb-user run --tee <vendor-public-ip>:5000 --rpc <url> --contract <0x…>
```
Ask the Vendor's enclave to attest → verify the document → find who registered that measurement
→ check each registration. Exit 0 if at least one registration matches (value, expiry, signer);
exit 1 with the reason if nobody registered it or none check out. No keys, no ETH.

What that does, one step at a time.

T1 — attestation document from the Vendor's enclave:
```
tb-user ra-request --tee <vendor-public-ip>:5000 --out doc.cbor --format aws-nitro
```

T2 — verify it, extract PCR0·1·2:
```
tb-user ra-parse --doc doc.cbor --out parsed.json
```

C2 — find the registration by measurement alone; no key from anyone is needed:
```
tb-user lookup --rpc <url> --contract <0x…> --measurements parsed.json
```

M2 — the trust decision, for each key `lookup` found:
```
tb-user compare --a parsed.json --rpc <url> --contract <0x…> --key <a key lookup found>
```


- `setup/vendor/`, `setup/auditor/` — build / launch / chain / transfer notes per role, including
  what exactly `build` normalizes and what it cannot fix (unpinned package versions, random state)
- `docs/` — design notes
