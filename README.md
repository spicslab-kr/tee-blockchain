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

Software runs in a TEE (AWS Nitro Enclaves, or an Intel TDX or AMD SEV-SNP confidential VM on GCP). An independent Auditor rebuilds it, confirms the
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

### Get the source

```
git clone https://github.com/spicslab-kr/tee-blockchain
cd tee-blockchain
git submodule update --init contracts/evm/lib/openzeppelin-contracts
```

The last line fetches OpenZeppelin Contracts v5.7.0, which `contracts/evm/Registry.sol` is compiled against
([contracts/README.md](contracts/README.md)). The Go code does not need it: the contract's Go binding is committed. You
need it to compile or change the contract. `git clone --recursive` fetches it too, but also OpenZeppelin's own test
submodules, which are not needed and can fail on Windows when the folder path is long.

There are no release binaries yet; build them from source as below.

### The binaries

You need Go **go1.26.1** exactly (`go version` must print it): `build` refuses any other version, because a different
patch version produces a different `att-ra-responder` and therefore different measurements. If another Go is
installed, set `GOTOOLCHAIN=go1.26.1` (for example `export GOTOOLCHAIN=go1.26.1`) and Go fetches and uses exactly that
one. How to install it, and everything else each platform needs: [docs/build/toolchain.md](docs/build/toolchain.md).

For your own machine (the User's `verify`, and every chain command). The `NAME=value command` form is POSIX shell;
on Windows run these in Git Bash. `CGO_ENABLED=0` matters for `att-ra-responder`: with gcc installed, a native Linux
build otherwise links cgo and gives a different file, and with it different measurements, than a build anywhere else.
`build` refuses such a responder. It is on every line so the binaries are the same however they are built:

```
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-vendor       ./cmd/vendor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-auditor      ./cmd/auditor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-user         ./cmd/user
```

For a TEE instance or guest, which is Linux on x86_64 (add `.exe` nowhere; on Windows run this in Git Bash):

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o linux/att-vendor       ./cmd/vendor
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o linux/att-auditor      ./cmd/auditor
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o linux/att-ra-responder ./cmd/ra-responder
```

`att-ra-responder` goes next to `att-vendor` or `att-auditor` on the instance: on Nitro, `build` layers that file
into the enclave image; on TDX and SEV-SNP it runs on the guest and answers attestation requests.
`go test ./...` checks everything without any TEE (see [Testing](#testing)).

### The TEE image, with the same measurements every time

The Vendor and the Auditor each build the same source on their own instance. Whoever registers the
value, it can only be trusted if both builds give the same measurements.

**What to watch when building.** Anything that differs between the two builds changes the
measurements. **The full checklist, per platform, with the values to use, how to check each, and which measurement
each one moves: [docs/build/reproduce.md](docs/build/reproduce.md).** The most important:

1. The same CPU architecture (`amd64` or `arm64`) on both sides.
2. The same Go toolchain (pinned, go1.26.1) with `-trimpath -buildvcs=false`, and one and the same
   `att-ra-responder` file on both sides.
3. A base image pinned by digest (`FROM nginx@sha256:…`), and every package, dependency and
   `git clone` pinned to an exact version or commit.
4. No time, host name or random value written into a file at build time.
5. On Nitro, the same AMI and the same `docker` and `nitro-cli` versions. `build` does not record
   them for you, so write yours down; the versions our runs used are in
   [docs/build/toolchain.md](docs/build/toolchain.md).
6. Docker itself: `docker build` decides file order and timestamps in each layer anew on every
   run, so two builds of the same files still differ.
7. On TDX and SEV-SNP, the VM as well as the build: the same exact image version and the same
   **machine type** on both sides. Measured on 2026-11-16 by changing only the machine type of one
   guest ([session 4](docs/results/sessions-2026-11-16/session4-t1-t3-t7-t8.md#t1-machine-type-and-zone)):
   - SEV-SNP: the vCPU count changes MEASUREMENT (`n2d-standard-4` against `n2d-standard-2`); more
     memory with the same vCPUs (`n2d-highmem-2`) and another zone (`us-central1-b`) did not.
   - TDX: the machine type changes RTMR0 (`c3-standard-8` against `c3-standard-4`). Whether it is the
     vCPUs or the memory could not be told apart: GCP refused the shapes that would separate them
     (`c3-highmem-4`, `c3-highcpu-8`) for a TDX VM. Another zone (`us-central1-b`) did not change
     anything, measured on a TDX guest of its own.

   So the Auditor makes its guest with the Vendor's machine type, as the guides' "The same inputs"
   asks.

**What we provide.** `att-vendor build` and `att-auditor build` take care of part of this:

- They repack the image files in a fixed order with fixed times and owners, and remove what only
  records when it was built (package-manager logs, apk install times, build caches, `/tmp`).
- They refuse an `att-ra-responder` built with another Go version, without `-trimpath`, with the
  git commit stamped in (no `-buildvcs=false`), or for another OS or architecture.
- They warn about each Dockerfile line that breaks items 3 and 4. Fixing those lines is up to you.
- `--verify` builds a second time from scratch and fails unless the measurements match.
- `att-auditor receive` stops if the OS, architecture or Go version differs from the Vendor's.

```
att-vendor build --app-dir <dir with Dockerfile> --out-dir out --verify
```

All the cautions, per TEE: [AWS Nitro Enclaves](docs/build/aws-nitro.md), [Intel TDX](docs/build/tdx.md),
[AMD SEV-SNP](docs/build/sev-snp.md).

**Step by step, from nothing:** Nitro, the two flows in [e2e/README.md](e2e/README.md) (instances:
[e2e/cloud/README.md](e2e/cloud/README.md)); Intel TDX, [Getting started](docs/build/tdx.md#getting-started-from-nothing-to-a-verified-tdx-guest); AMD SEV-SNP,
[Getting started](docs/build/sev-snp.md#getting-started-from-nothing-to-a-verified-sev-snp-guest). On TDX and SEV-SNP
steps 1 to 13 use one guest, and their Auditor registers the Vendor's guest's values without rebuilding them; the
independent Auditor of the trust model above is each page's "The Auditor's half", which needs two guests and **passed
on both platforms on 2026-11-16** ([TDX](docs/results/sessions-2026-11-16/intel-tdx.md),
[SEV-SNP](docs/results/sessions-2026-11-16/amd-sev-snp.md)). What each platform's evidence gives the verifier, side
by side, and which one to use: [RA comparison](docs/build/ra-comparison.md#which-one-to-use).

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
att-vendor launch --eif out/<name>.eif           # run it; add --open-port-to <verifier-ip>/32 to open the port
att-vendor send   --repo <git url> --config build-config.json
```

You write `build-config.json` yourself; its fields and an example are in the Vendor guide,
[Hand-off with the Auditor](cmd/vendor/README.md#hand-off-with-the-auditor).

As an **Auditor**, on a Nitro instance, with the Vendor's repository. A registry is deployed once, before the first
`audit`; it prints the address that `--registry` takes:

```
att-auditor registry-deploy --rpc <url> --deployer-key-file <key> --approver <0x… address of your signing key>
att-auditor receive --repo <git url> --into work/src
att-auditor build   --app-dir work/src/<path> --out-dir out
att-auditor launch  --eif out/<name>.eif
att-auditor audit   --config work/src/build-config.json --tee-addr <my-instance>:5000 \
                   --build-measurements out/measurements.json --rpc <url> --registry <0x…> --signing-key-file <key>
```

A key file is one line, a 32-byte Ethereum private key in hex (with or without `0x`); how to make one and get its
address: [e2e/README.md, Key files](e2e/README.md#key-files). On a public network both keys need a balance: test ETH
from a Sepolia faucet. To try it all without any ETH or key files, run the registry on a local `anvil` chain:
[Running without a public testnet](e2e/README.md#running-without-a-public-testnet). On TDX and SEV-SNP the same
commands run on a guest; the getting-started procedures linked above go through them step by step.

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
- **Launch and self-check.** `launch` starts the enclave, bridges the port, and attests itself. It
  never opens the security group on its own: it prints the command, and opens the port only to the CIDR
  given with `--open-port-to` (`0.0.0.0/0` only when written), logging what it opened to whom
  ([details](docs/build/aws-nitro.md#opening-the-ra-port)).
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

- **AWS Nitro Enclaves end to end.** Request, document verification against the AWS Nitro root,
  measurement extraction, image build and launch.
- **Intel TDX and AMD SEV-SNP on GCP end to end.** `ra-parse`,
  `audit` and `verify` check a TDX quote (`intel-tdx`: MRTD, RTMR0–2) or SEV-SNP evidence
  (`amd-sev-snp-initrd`, `amd-sev-snp-verity`), tested against evidence recorded on GCP, including
  recordings with an application in both options. `att-ra-responder ra-serve --extractor tdx | snp-initrd |
  snp-verity` produces the evidence inside the guest; tested against the recordings, and on 2026-11-12 run on
  live TDX and SEV-SNP guests with the whole chain on the guest (`e2e/cvm/in-guest-verify.sh`). For TDX,
  `build` and `launch --platform intel-tdx` build the application initrd (both options, with `--verify`) and
  install and check it under GRUB; for SEV-SNP the same with `--platform amd-sev-snp-initrd` or
  `amd-sev-snp-verity`. Run on real guests, both options, on 2026-11-15 and 2026-11-16, with `audit` and `verify`
  from another machine. **The independent Auditor ran on both CVMs on 2026-11-16**: on two guests of one image, the
  Auditor rebuilt the Vendor's application on its own guest, got the same values, registered them, and a User's
  `verify` of the Vendor's guest passed; one byte changed on the Vendor's side was refused
  ([TDX](docs/results/sessions-2026-11-16/intel-tdx.md), [SEV-SNP](docs/results/sessions-2026-11-16/amd-sev-snp.md)).
  SEV-SNP evidence is two documents from two signers, and nothing binds them to one VM: a relay that hashes one VM's
  vTPM evidence into another VM's report verifies (reproduced 2026-11-16; see
  [RA comparison](docs/build/ra-comparison.md) and [AMD SEV-SNP](docs/build/sev-snp.md#same-vm)).
  What a measurement says about a running guest: [runtime immutability](docs/build/runtime-immutability.md).
- **GCP Confidential Space and Azure** have request-side scaffolding only.
- **The measurement is the identity.** The image hash (PCR0·1·2 on Nitro, MRTD and RTMR0–2 on TDX, MEASUREMENT,
  PCR4, KERNEL, CMDLINE, INITRD and for verity PCR12 on SEV-SNP) is what gets registered and looked up, so no name, version or key has to be known in advance.
- **One architecture, one registration.** x86_64 and arm64 builds measure differently, so each
  registration says which architecture it is for.
- **A deliberately simple contract.** `Registry.sol` never overwrites and never revokes. Expiry is
  the only way a registration ends.

→ [Registry contract](contracts/README.md)

## Project resources

- [Project structure](STRUCTURE.md): every folder and module, and how the roles fit together
- [End-to-end scripts](e2e/README.md): our runs against real Nitro instances
- [Running without a public testnet](e2e/README.md#running-without-a-public-testnet): both flows on real enclaves
  with a local chain (`anvil`) and `NET=local`, so you can run the whole thing without any ETH

## Testing

```
go test ./...
```

Unit tests run anywhere. `cmd/*_e2e_test.go` build the real binaries and drive them against a local
[anvil](https://getfoundry.sh) chain and recorded attestation evidence (Nitro, TDX and SEV-SNP). No TEE
hardware is needed for any test. The tests look for anvil on `PATH` or in `~/.foundry/bin` and skip the
chain tests without it; they were last run with anvil 1.8.0 (Foundry v1.8.0, commit `61ae26af`). To install exactly
that version: `foundryup --install v1.8.0` (commands per system in [e2e/README.md](e2e/README.md#running-without-a-public-testnet)).

## License

The code is licensed under the [MIT License](LICENSE), the same license `contracts/evm/Registry.sol` declares. The
documentation and images (`*.md` files and `docs/`) are licensed under
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
