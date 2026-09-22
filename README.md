# tee-blockchain

Remote attestation for software running in a TEE (currently AWS Nitro Enclaves), with the
attested measurements registered on an Ethereum-compatible chain so anyone can look them up
and verify them later.

**Vendor** builds and runs the software in an enclave. **Auditor** independently builds the
same source, or fetches a running enclave's attestation, and registers the measurements it
trusts on-chain. **User** asks a running enclave to attest itself, then checks that result
against what's on-chain before trusting it.

Each role gets its own binary (`tb-vendor`, `tb-auditor`, `tb-user`), built from `cmd/vendor`,
`cmd/auditor`, `cmd/user`. A role's binary only contains that role's commands — User's binary,
for example, has no `register` or `build` command at all.

## Build

```
go build -o tb-vendor   ./cmd/vendor
go build -o tb-auditor  ./cmd/auditor
go build -o tb-user     ./cmd/user
```

Requires Go 1.26+. The `build`/`launch` commands additionally require `docker` and
`nitro-cli`, and only run on a Nitro-enabled EC2 instance (Linux).

## Vendor

1. **Build** the enclave image on a Nitro-enabled EC2 instance — this layers the RA responder
   onto your Dockerfile and produces an EIF plus PCR0/1/2:
   ```
   tb-vendor build --app-dir ./my-app --out out
   ```
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
4. **Register** your own measurements so others can look them up under your signature:
   ```
   tb-vendor register --rpc <url> --contract <0x…> --key-file <vendor.key> \
     --measurements out/measurements.json --service <name> --version <v>
   ```
5. Anyone can now reach your enclave: `tb-user ra-request --tee <public-ip>:5000`.

## Auditor

1. **Get the measurements to verify.** Either build the same source independently:
   ```
   tb-auditor build --app-dir ./my-app --out out
   ```
   or request attestation directly from a running enclave and parse it:
   ```
   tb-auditor ra-request --tee <host>:<port> --out doc.cbor
   tb-auditor ra-parse --doc doc.cbor --nonce doc.cbor.nonce --out measurements.json
   ```
2. **Register** what you found, under your own signature:
   ```
   tb-auditor register --rpc <url> --contract <0x…> --key-file <auditor.key> \
     --measurements measurements.json --service <name> --version <v>
   ```
3. **Check it against the chain** — confirms the value, expiry, and signer all match:
   ```
   tb-auditor compare --a measurements.json --rpc <url> --contract <0x…> --key <registered-key>
   ```

## User

1. **Request attestation** from the enclave you want to trust:
   ```
   tb-user ra-request --tee <host>:<port> --out doc.cbor --format aws-nitro
   ```
2. **Parse it** to get the measurements out:
   ```
   tb-user ra-parse --doc doc.cbor --nonce doc.cbor.nonce --out measurements.json
   ```
3. **Look up** who has registered a matching measurement on-chain:
   ```
   tb-user lookup --rpc <url> --contract <0x…> --measurements measurements.json
   ```
4. **Compare** — this is the actual trust decision. It fails (exit 1) if the value, expiry, or
   signer don't check out:
   ```
   tb-user compare --a measurements.json --rpc <url> --contract <0x…> --key <a-key-from-lookup>
   ```

## Documentation

- `setup/vendor/`, `setup/auditor/` — per-role build/launch/chain notes
- `docs/` — design notes
