# Security

## Reporting a vulnerability

Please report it privately, not in a public issue:

**Contact: _[to be filled in by the maintainers]_**

Say what you found, how to reproduce it, and which commit you used. There are no releases yet, so only the current
`main` is supported.

## In scope

Attestamp's whole job is a trust decision, so anything that makes it decide wrongly:

- `verify`, `audit`, `self-register` or `ra-parse` accepting evidence they should refuse: a forged or altered
  document, a broken certificate chain, a wrong or reused nonce, a TEE in debug mode, evidence from another TEE.
- A registration being found, or trusted, for measurements, a platform or an approver it was not made for. This
  includes the four copies of the measurement hash disagreeing (`CONTRIBUTING.md`, "The measurement hash").
- `contracts/evm/Registry.sol` accepting a registration not signed by its approver.
- `build` or `launch` accepting an input it is meant to refuse (a responder built with the wrong toolchain or
  settings, a build environment that does not match, an image that measures differently), so that two parties
  register values that do not describe the same code.
- A secret, instance address or account identifier committed to this repository.

## Known and documented

These are already written down; a report that goes beyond them is welcome.

- On SEV-SNP on GCP, a report from one VM requested over another VM's vTPM attestation verifies (T10):
  [docs/build/ra-comparison.md](docs/build/ra-comparison.md#the-sev-snp-relay-measured-t10-2026-11-16).
- TCB levels and CRLs are not checked on TDX and SEV-SNP: [tdx.md](docs/build/tdx.md#not-done-yet),
  [sev-snp.md](docs/build/sev-snp.md#not-done-yet).
- On TDX and SEV-SNP the responder that hands out the evidence is not itself measured
  ([docs/build/ra-comparison.md](docs/build/ra-comparison.md)).

## Out of scope

- Weaknesses in the TEE hardware, firmware or cloud services themselves (AWS Nitro, Intel TDX, AMD SEV-SNP, Google
  Cloud): report them to those vendors.
- Attacks that need an approver's signing key.
- Vulnerabilities in a dependency that do not affect Attestamp: report them upstream. If one does affect Attestamp,
  tell us too.
