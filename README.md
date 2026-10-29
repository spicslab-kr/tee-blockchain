<p align="center">
  <img src="docs/img/logo.svg" alt="Attestamp logo" width="160">
</p>

<h1 align="center">Attestamp</h1>

<p align="center"><b>Remote attestation that makes the hash of the code running in a TEE trustworthy.</b></p>

<p align="center">
  <sub>CREATED AND MAINTAINED BY</sub><br>
  SPICS Lab, Kyung Hee University<br>
  <a href="https://github.com/spicslab-kr/tee-blockchain/issues">GitHub issues</a> &nbsp;·&nbsp;
  <a href="#contributors">Contributors</a>
</p>

Software runs in a TEE (AWS Nitro Enclaves today). An independent Auditor rebuilds it, confirms the
enclave really runs those bytes, and registers the measurement on an Ethereum-compatible chain.
Anyone can then ask the enclave to attest itself and compare the answer with what is on-chain,
without a key from anyone.

<p align="center">
  <img src="https://img.shields.io/badge/go-1.26.1%20pinned-00ADD8?logo=go&logoColor=white" alt="Go 1.26.1 pinned">
  <img src="https://img.shields.io/badge/TEE-AWS%20Nitro%20Enclaves-FF9900?logo=amazonaws&logoColor=white" alt="TEE AWS Nitro Enclaves">
  <img src="https://img.shields.io/badge/chain-Ethereum%20%C2%B7%20Solidity-3C3C3D?logo=ethereum&logoColor=white" alt="Chain Ethereum and Solidity">
</p>

<p align="center">
  <a href="#build">Build</a> &nbsp;·&nbsp;
  <a href="#quick-start">Quick start</a> &nbsp;·&nbsp;
  <a href="#features">Features</a> &nbsp;·&nbsp;
  <a href="#project-resources">Project resources</a>
</p>

---

## Build

### The binaries

Download the Linux binaries for your architecture (`amd64` or `arm64`) from the
[latest release](../../releases/latest). They are built with the pinned toolchain and `-trimpath`,
and `SHA256SUMS` covers every file. Or build from source:

```
go build -trimpath -o att-vendor       ./cmd/vendor
go build -trimpath -o att-auditor      ./cmd/auditor
go build -trimpath -o att-user         ./cmd/user
go build -trimpath -o att-ra-responder ./cmd/ra-responder
```

The toolchain is pinned to exactly go1.26.1. `build` refuses any other version, because a
different patch version produces a different `att-ra-responder` and therefore different
measurements.

### The TEE image, with the same measurements every time

The Vendor and the Auditor each build the same source on their own instance. Whoever registers the
value, it can only be trusted if both builds give the same measurements.

**What to watch when building.** Anything that differs between the two builds changes the
measurements. The most important:

1. The same CPU architecture (`amd64` or `arm64`) on both sides.
2. The same Go toolchain (pinned, go1.26.1) with `-trimpath`, and one and the same
   `att-ra-responder` file on both sides.
3. A base image pinned by digest (`FROM nginx@sha256:…`), and every package, dependency and
   `git clone` pinned to an exact version or commit.
4. No time, host name or random value written into a file at build time.
5. The same AMI and the same `docker` and `nitro-cli` versions. Write them down: they are not
   recorded anywhere.
6. Docker itself: `docker build` decides file order and timestamps in each layer anew on every
   run, so two builds of the same files still differ.

**What we provide.** `att-vendor build` and `att-auditor build` take care of part of this:

- They repack the image files in a fixed order with fixed times and owners, and remove what only
  records when it was built (package-manager logs, apk install times, build caches, `/tmp`).
- They refuse an `att-ra-responder` built with another Go version, without `-trimpath`, or for
  another OS or architecture.
- They warn about each Dockerfile line that breaks items 3 and 4. Fixing those lines is up to you.
- `--verify` builds a second time from scratch and fails unless the measurements match.
- `att-auditor receive` stops if the OS, architecture or Go version differs from the Vendor's.

```
att-vendor build --app-dir <dir with Dockerfile> --out-dir out --verify
```

All the cautions, per TEE: [AWS Nitro Enclaves](docs/build/aws-nitro.md).

## Quick start

As a **User**, with an enclave address and a registry address. No keys, no ETH, no Nitro:

```
att-user verify --tee-addr <vendor-public-ip>:5000 --rpc <ethereum rpc url> --registry <0x…>
```

Exit 0 and a line like `✓ matches  nginx 1.31.6 (amd64) by 0x…, expires …` means a registration
matches. Exit 1 prints the reason. Measured from Korea against a Nitro enclave in eu-north-1 and
Ethereum mainnet: about 3 s and 21 MB peak.

As a **Vendor**, on a Nitro instance, with the directory that holds your Dockerfile:

```
att-vendor build  --app-dir <dir> --out-dir out  # reproducible enclave image + measurements
att-vendor launch --eif out/<name>.eif           # run it, open the attestation port
att-vendor send   --repo <git url> --config build-config.json
```

As an **Auditor**, on a Nitro instance, with the Vendor's repository:

```
att-auditor receive --repo <git url> --into work/src
att-auditor build   --app-dir work/src/<path> --out-dir out
att-auditor launch  --eif out/<name>.eif
att-auditor audit   --config work/src/build-config.json --tee-addr <my-instance>:5000 \
                   --build-measurements out/measurements.json --rpc <url> --registry <0x…> --signing-key-file <key>
```

## Features

### Two ways to register

**Flow 1: an Auditor registers.** The Auditor rebuilds the Vendor's source in its own TEE and
registers the hash it gets. The User trusts what the Auditor signed.

<p align="center">
  <img src="docs/img/flow-auditor.svg" alt="Flow 1: the Vendor sends source and build info, the Auditor rebuilds, signs and registers the hash on the blockchain, and the User compares the TEE's hash with the registry">
</p>

**Flow 2: the Vendor registers itself.** Without an Auditor, the Vendor signs and registers the hash
of its own enclave. An Auditor can check it later: rebuild the source, ask the Vendor's TEE for its
hash, and register its own signed hash. The User can then require both registries to agree.

<p align="center">
  <img src="docs/img/flow-vendor.svg" alt="Flow 2: the Vendor signs and registers the hash on the blockchain, an Auditor can check and register later, and the User compares the TEE's hash with the registry">
</p>

### Three roles, four binaries

- **One binary per role.** `att-vendor`, `att-auditor` and `att-user` each contain only their own
  role's commands. The User's binary has no `register` at all.
- **A shared in-enclave responder.** `att-ra-responder` answers attestation requests inside the
  enclave. `build` layers it into the image.
- **Two flows.** Flow 1: the Auditor rebuilds and registers, and the User trusts only what the
  Auditor signed. Flow 2: a Vendor without an Auditor registers to its own registry, and the User
  can require that both registries agree.

### Vendor

- **Reproducible enclave image.** `build` and `pack` layer the responder onto your Docker image and
  normalize everything Docker makes nondeterministic.
- **Reproducibility scan.** Unpinned packages, `date`, random keys and the like are reported line by
  line, and `--verify` builds twice and fails unless both measurements are identical.
- **Build-environment declaration.** `build-config.json` records source, OS, CPU architecture and
  toolchain, so the Auditor can rebuild in the same environment.
- **Launch and self-check.** `launch` starts the enclave, bridges the port, and attests itself.
- **Self-registration (flow 2).** `self-register` attests, registers and checks in one command.

→ [Vendor guide](cmd/vendor/README.md)

### Auditor

- **Environment check.** `receive` clones the Vendor's source and refuses to continue on a different
  OS, CPU architecture or Go version, because a mismatch would look like a finding.
- **Same bytes or no registration.** `audit` registers only if the enclave's attested value equals
  the Auditor's own build.
- **One registry, one approver.** `registry-deploy` creates a registry that accepts only that
  approver's EIP-712 signature.
- **Hand-back.** `send` publishes the measurement record and image to the Vendor.

→ [Auditor guide](cmd/auditor/README.md)

### User

- **One command.** `verify` asks the enclave to attest, verifies the document against the platform
  root, finds who registered that measurement, and checks every registration.
- **No secrets.** No key, no ETH, no Nitro, no Docker.
- **Trust decision.** A registration passes only if the measurement matches, it has not expired
  (judged on the chain's clock), and its approver is the registry's own approver or an address you name (`--trusted-approvers`).
- **Several registries.** Repeat `--registry` to require the Vendor's and the Auditor's registry to
  agree.

→ [User guide](cmd/user/README.md)

### Platform and registry

- **AWS Nitro Enclaves today.** Request, document verification against the AWS Nitro root, and
  measurement extraction are implemented end to end. GCP and Azure have request-side scaffolding
  only. Confidential VMs that measure a guest OS (AMD SEV-SNP, Intel TDX) are not covered.
- **The measurement is the identity.** The enclave image hash (PCR0·1·2) is what gets registered
  and looked up, so no name, version or key has to be known in advance.
- **One architecture, one registration.** x86_64 and arm64 builds measure differently, so each
  registration says which architecture it is for.
- **A deliberately simple contract.** `Registry.sol` never overwrites and never revokes. Expiry is
  the only way a registration ends.

→ [Registry contract](contracts/README.md)

## Project resources

- [Project structure](STRUCTURE.md): every folder and module, and how the roles fit together
- [End-to-end scripts](e2e/README.md): our runs against real Nitro instances
- [Releases](../../releases)

## Testing

```
go test ./...
```

Unit tests run anywhere. `cmd/*_e2e_test.go` build the real binaries and drive them against a local
[anvil](https://getfoundry.sh) chain and a recorded Nitro attestation document. No Nitro hardware is
needed for any test.

## License

The code is licensed under the [Apache License, Version 2.0](LICENSE). The documentation and
images (`*.md` files and `docs/`) are licensed under
[Creative Commons Attribution 4.0 International](LICENSE-CC-BY) (CC BY 4.0).

## Contributors

<table>
  <tr>
    <td align="center" width="25%">
      <a href="https://github.com/qixiangme"><img src="https://github.com/qixiangme.png" width="80" alt=""><br><b>Changmin Lee</b></a><br>
      <sub>Intern at <a href="https://spicslab.kr">SPICS Lab</a></sub>
    </td>
    <td align="center" width="25%">
      <a href="https://github.com/bagss3366"><img src="https://github.com/bagss3366.png" width="80" alt=""><br><b>Sunghyun Park</b></a><br>
      <sub>M.S. student at <a href="https://spicslab.kr">SPICS Lab</a></sub>
    </td>
    <td align="center" width="25%">
      <a href="https://github.com/rallykevin"><img src="https://github.com/rallykevin.png" width="80" alt=""><br><b>Kevin Nam</b></a><br>
      <sub>Principal Investigator of <a href="https://spicslab.kr">SPICS Lab</a></sub>
    </td>
    <td align="center" width="25%">
      <a href="https://github.com/junbumshin-armor"><img src="https://github.com/junbumshin-armor.png" width="80" alt=""><br><b>Junbum Shin</b></a><br>
      <sub>CEO of <a href="https://datarmor.ai">Datarmor</a></sub>
    </td>
  </tr>
</table>
