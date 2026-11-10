# Recorded SEV-SNP evidence

One evidence pair from a real AMD SEV-SNP guest on GCP, used by the tests in `internal/tee/parse/snp`. It lets the
parser be written and tested with no instance running.

| File | Content |
|---|---|
| `gcp-n2d-att.pb` | vTPM attestation: go-tpm-tools `attest.Attestation`, binary protobuf, 63,957 bytes. Quotes over every PCR bank signed by the GCE attestation key (RSA), with the AK certificate, its intermediates and the TCG event log |
| `gcp-n2d-report.bin` | SNP attestation report, 1,184 bytes, raw from ConfigFS TSM `outblob` |
| `gcp-n2d-auxblob.bin` | The certificate table the host returned with the report (ConfigFS TSM `auxblob`), 4,763 bytes |
| `gcp-n2d-record.txt` | What `record` printed: nonce, REPORT_DATA, sizes |
| `record/main.go` | The program that recorded them |

## Where it came from

| | |
|---|---|
| Cloud | Google Cloud, zone us-central1-a, Confidential VM type SEV_SNP |
| Machine type | `n2d-standard-2` (AMD Milan) |
| OS | Ubuntu 24.04.5 LTS |
| Kernel | `7.0.0-1017-gcp` |
| Recorded | 2026-11-10, 17:23 UTC |
| Interfaces | `/dev/tpmrm0` for the vTPM; ConfigFS TSM, `/sys/kernel/config/tsm/report`, `provider` = `sev_guest`, for the report |

Read from the report: VERSION 5, POLICY `0x30000` (DEBUG, bit 19, is clear), SIGNING_KEY 0 (signed by the VCEK).

## The nonce and the binding

- vTPM nonce = SHA-256 of the ASCII string `attestamp-snp-fixture-0001` =
  `d529879465187801d4b9ffc1c7d0848a2f028e5df9ab46d3eb7a9d5cdb7f925f` (32 bytes, the `tee/request` nonce size).
- REPORT_DATA = SHA-512 of the whole `gcp-n2d-att.pb` file =
  `29377722…4fb7da` (full value in `gcp-n2d-record.txt`).

The SHA-512 binding is the one the 2026-11 SEV-SNP test scripts used (`vendor/vendor_evidence.sh`), so these files
have the same shape as the evidence those runs produced.

## Identifying information in these files

These files identify the GCP project and the instance they came from. They are committed anyway, for the same
reason `internal/tee/parse/aws/testdata/fixture-my-web.cbor` carries an EC2 instance ID: without a committed fixture
the parser tests cannot run in CI or for anyone else, and testing without hardware is what this project relies on.
The values cannot be removed without breaking the signatures over them.

| File | Contains |
|---|---|
| `gcp-n2d-att.pb` | In the Google-signed AK certificate: the GCP **project ID**, the **zone**, the **instance name** (`attestamp-snp-vendor`) and the numeric **instance ID**, all found as text. The project number was not found as text; whether the certificate carries it in binary form was not checked. The event log describes the boot of that instance. |
| `gcp-n2d-report.bin` | `CHIP_ID`, which identifies the physical AMD processor of the Google host, and that host's TCB versions. No project or instance identifiers (searched for). |
| `gcp-n2d-auxblob.bin` | The VCEK certificate, which carries the same chip ID and TCB. No project or instance identifiers (searched for). |

None of them contains a private key, a credential or an IP address.

## Recording another one

`record/main.go` is outside the module build (Go ignores `testdata`). To build it:

```
mkdir /tmp/record && cp record/main.go /tmp/record && cd /tmp/record
go mod init record
go get github.com/google/go-tpm-tools@v0.4.11 github.com/google/go-tpm@v0.9.8
go mod tidy
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o record .
```

Then on an SEV-SNP guest: `sudo ./record -out fx -nonce-text attestamp-snp-fixture-0001`.
