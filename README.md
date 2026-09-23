# tee-blockchain

Remote attestation for software running in a TEE (currently AWS Nitro Enclaves), with the
attested measurements registered on an Ethereum-compatible chain so anyone can look them up
and verify them later.

**Vendor** runs the software in an enclave. **Auditor** independently confirms what's
actually running — either by building the same source itself, or simply requesting
attestation directly from the running enclave — and registers the measurements it vouches
for on-chain. **User** asks a running enclave to attest itself, then checks that result
against what's on-chain before trusting it.

Each role gets its own binary (`tb-vendor`, `tb-auditor`, `tb-user`), built from
`cmd/vendor`, `cmd/auditor`, `cmd/user`. A role's binary only contains that role's commands
— User's binary, for example, has no `register` command at all.

## Build

```
go build -o tb-vendor   ./cmd/vendor
go build -o tb-auditor  ./cmd/auditor
go build -o tb-user     ./cmd/user
```

Requires Go 1.26+. Building the *enclave image itself* (`build`/`launch` below)
additionally requires `docker` and `nitro-cli`, and only runs on a Nitro-enabled EC2
instance (Linux) — and is itself optional, see below.

## Quick start — one command per role

Each role's whole job collapses to one `run` command. This is what you actually want most
of the time; the individual steps below (`ra-request`, `register`, `lookup`, `compare`,
...) are there for finer control or scripting.

```
tb-vendor run --tee <host:port> --rpc <url> --contract <0x…> --key-file <key> --service <name> --version <v>
```
```
tb-auditor run --tee <host:port> --rpc <url> --contract <0x…> --key-file <key> --service <name> --version <v>
```
```
tb-user run --tee <host:port> --rpc <url> --contract <0x…>
```

- Vendor/Auditor `run` = request attestation → register it → self-check, in one shot.
  Already have the measurements some other way (a build, or a received file)? Skip `--tee`
  and pass `--measurements <file>` instead. `--service`/`--version` are optional — if
  someone already registered this same measurement, `run` reuses whatever they used; you
  only need to type them for the very first registration.
- User `run` = request attestation → look up who's registered it → check it, in one shot.
  Exits 0 if at least one registered entry matches; exits 1 (with a reason) if nobody
  registered this measurement or none of them check out.
- Any flag you leave out gets asked for interactively (Enter accepts the shown default) —
  except `--rpc`: this talks to Ethereum **mainnet** by default, so accepting that default
  requires typing `y`, not just Enter. Typing any other URL (a testnet, say) is accepted
  as-is.

## Vendor — the individual steps

1. **Get the enclave image.** Either build it yourself on a Nitro-enabled EC2 instance:
   ```
   tb-vendor build --app-dir ./my-app --out out
   ```
   or just receive an already-built EIF file from whoever built it (currently via
   `tb-vendor receive` over git — see Auditor step 1; a plainer file-transfer path, FTP or
   a shared bucket, would work just as well and is worth adding). Building it yourself is
   optional — a received file works too.
2. **Launch** it — starts the enclave, bridges TCP to its vsock, self-checks with a real RA
   request, and opens the inbound port:
   ```
   tb-vendor launch --eif out/my-app.eif
   ```
3. **Deploy the registry** once (first time only) — fixes the Auditor and Vendor addresses
   for this contract permanently:
   ```
   tb-vendor registry-deploy --rpc <url> --key-file <deployer.key> --auditor <0x…> --vendor <0x…>
   ```
4. **Register + self-check** — one shot:
   ```
   tb-vendor run --tee <this enclave's address> --rpc <url> --contract <0x…> --key-file <vendor.key> --service <name> --version <v>
   ```
   Separately:
   ```
   tb-vendor register --rpc <url> --contract <0x…> --key-file <vendor.key> --measurements <file> --service <name> --version <v>
   ```
   ```
   tb-vendor compare --a <file> --rpc <url> --contract <0x…> --key <the key register printed>
   ```
   This is a signed transaction — the contract only accepts entries signed by the Auditor
   or Vendor key — but nobody looking it up needs your signature or key ahead of time; they
   only need the measurement value itself.
5. Anyone can now reach your enclave:
   ```
   tb-user run --tee <public-ip>:5000 --rpc <url> --contract <0x…>
   ```

## Auditor — the individual steps

1. **Get the measurements to verify.** Building the same source independently is one way:
   ```
   tb-auditor build --app-dir ./my-app --out out
   ```
   but it's optional — just as valid is requesting attestation directly from the running
   enclave, which `tb-auditor run --tee ...` does for you (or do it by hand with
   `ra-request` + `ra-parse`).
2. **Register + self-check** — one shot:
   ```
   tb-auditor run --tee <host:port> --rpc <url> --contract <0x…> --key-file <auditor.key> --service <name> --version <v>
   ```
   Separately:
   ```
   tb-auditor register --rpc <url> --contract <0x…> --key-file <auditor.key> --measurements <file> --service <name> --version <v>
   ```
   ```
   tb-auditor compare --a <file> --rpc <url> --contract <0x…> --key <the key register printed>
   ```

## User — the individual steps

One shot:
```
tb-user run --tee <host:port> --rpc <url> --contract <0x…>
```

Separately, that's these four steps:

1. **Request attestation** from the enclave
   ```
   tb-user ra-request --tee <host:port> --out doc.cbor
   ```
2. **Parse it** to get the measurements out
   ```
   tb-user ra-parse --doc doc.cbor --out measurements.json
   ```
3. **Look up** by measurement value alone — no key or signature needed from anyone
   ```
   tb-user lookup --rpc <url> --contract <0x…> --measurements measurements.json
   ```
4. **Compare** — the actual trust decision, against every registered entry found. Fails if
   none of them check out on value, expiry, or signer
   ```
   tb-user compare --a measurements.json --rpc <url> --contract <0x…> --key <a key from lookup>
   ```

## Documentation

- `setup/vendor/`, `setup/auditor/` — per-role build/launch/chain notes
- `docs/` — design notes
