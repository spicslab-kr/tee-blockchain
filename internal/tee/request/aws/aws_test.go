package aws

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestNSM_RequestEncoding(t *testing.T) {
	n := bytes.Repeat([]byte{7}, 32)
	b, err := encodeAttestationRequest(n, []byte("svc"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]map[string][]byte
	if err := cbor.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	a, ok := m["Attestation"]
	if !ok || !bytes.Equal(a["nonce"], n) || string(a["user_data"]) != "svc" || a["public_key"] != nil {
		t.Fatalf("request shape: %v", m)
	}
	// The nonce's size is checked by each role's adapter (internal/tee/request/<role>/aws), before Attest.
}

func TestNSM_ResponseDecoding(t *testing.T) {
	ok, _ := cbor.Marshal(map[string]any{"Attestation": map[string]any{"document": []byte("cose-sign1")}})
	doc, err := decodeAttestationResponse(ok)
	if err != nil || string(doc) != "cose-sign1" {
		t.Fatalf("ok response: %v %q", err, doc)
	}
	bad, _ := cbor.Marshal(map[string]any{"Error": "InvalidArgument"})
	if _, err := decodeAttestationResponse(bad); err == nil || !strings.Contains(err.Error(), "InvalidArgument") {
		t.Fatalf("error response: %v", err)
	}
	empty, _ := cbor.Marshal(map[string]any{"Attestation": map[string]any{}})
	if _, err := decodeAttestationResponse(empty); err == nil {
		t.Fatal("empty document must be rejected")
	}
	if _, err := decodeAttestationResponse([]byte{0xff}); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
