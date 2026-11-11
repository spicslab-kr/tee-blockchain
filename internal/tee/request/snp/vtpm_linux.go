package snp

import (
	"fmt"
	"net/http"

	"github.com/google/go-tpm-tools/client"
	"github.com/google/go-tpm/legacy/tpm2"
	"google.golang.org/protobuf/proto"
)

// GCEAttester attests with the GCE vTPM's RSA attestation key, the key whose certificate chains to Google's
// "EK/AK CA Root" (internal/tee/parse/snp/root.pem). The AK's intermediate certificates are fetched over HTTP from the
// URLs in the AK certificate, so the guest needs outbound access to Google's certificate host while attesting.
type GCEAttester struct {
	Device string // default /dev/tpmrm0
}

func (g GCEAttester) Attest(nonce []byte) ([]byte, error) {
	dev := g.Device
	if dev == "" {
		dev = "/dev/tpmrm0"
	}
	rwc, err := tpm2.OpenTPM(dev)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dev, err)
	}
	defer rwc.Close()
	ak, err := client.GceAttestationKeyRSA(rwc)
	if err != nil {
		return nil, fmt.Errorf("GCE attestation key: %w", err)
	}
	defer ak.Close()
	att, err := ak.Attest(client.AttestOpts{Nonce: nonce, CertChainFetcher: http.DefaultClient})
	if err != nil {
		return nil, err
	}
	return proto.Marshal(att)
}
