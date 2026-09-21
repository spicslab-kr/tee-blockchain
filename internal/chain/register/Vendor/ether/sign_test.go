package ether

import (
	"bytes"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	l "tb/internal/chain/lookup/Vendor"
	reg "tb/internal/chain/register/Vendor"
	p "tb/internal/tee/parse/Vendor"
)

func sample() (p.Measurements, reg.Meta) {
	return p.Measurements{
			{Name: "PCR1", Value: bytes.Repeat([]byte{1}, 48)},
			{Name: "PCR0", Value: bytes.Repeat([]byte{0}, 48)},
			{Name: "PCR2", Value: bytes.Repeat([]byte{2}, 48)},
		}, reg.Meta{
			Meta:    l.Meta{Service: "my-app", Version: "1.0.0", Platform: "aws-nitro"},
			Expires: time.Unix(1_800_000_000, 0),
		}
}

func TestMeasurementHash_OrderIndependent(t *testing.T) {
	v, _ := sample()
	a, err := reg.MeasurementHash(v)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := reg.MeasurementHash(p.Measurements{v[2], v[0], v[1]})
	if a != b {
		t.Fatal("order must not matter")
	}
	c, _ := reg.MeasurementHash(p.Measurements{v[0], v[1]})
	if a == c {
		t.Fatal("dropping a measurement must change the hash")
	}
	if _, err := reg.MeasurementHash(p.Measurements{v[0], v[0]}); err == nil {
		t.Fatal("duplicate names must be rejected")
	}
	if _, err := reg.MeasurementHash(nil); err == nil {
		t.Fatal("empty must be rejected")
	}
}

func TestDigest_DependsOnChainAndContract(t *testing.T) {
	v, m := sample()
	e, err := toEntry(v, m)
	if err != nil {
		t.Fatal(err)
	}
	c1 := common.HexToAddress("0x1000000000000000000000000000000000000001")
	d1, _ := Digest(big.NewInt(31337), c1, e)
	d2, _ := Digest(big.NewInt(11155111), c1, e)
	d3, _ := Digest(big.NewInt(31337), common.HexToAddress("0x2000000000000000000000000000000000000002"), e)
	if bytes.Equal(d1, d2) || bytes.Equal(d1, d3) {
		t.Fatal("a signature must be bound to one chain and one contract")
	}
	if len(d1) != 32 {
		t.Fatalf("digest length %d", len(d1))
	}
}

func TestSign_RecoversToSigner(t *testing.T) {
	v, m := sample()
	priv, _ := crypto.GenerateKey()
	r := Registrar{Contract: common.HexToAddress("0x1000000000000000000000000000000000000001")}
	s, err := r.signWith(big.NewInt(31337), v, m, priv)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Signature) != 65 || (s.Signature[64] != 27 && s.Signature[64] != 28) {
		t.Fatalf("signature shape: len %d v %d", len(s.Signature), s.Signature[64])
	}
	sig := append([]byte(nil), s.Signature...)
	sig[64] -= 27
	pub, err := crypto.SigToPub(s.Digest, sig)
	if err != nil {
		t.Fatal(err)
	}
	if crypto.PubkeyToAddress(*pub).Hex() != s.Signer {
		t.Fatal("signature does not recover to the signer address")
	}
}

func TestMeta_Validate(t *testing.T) {
	_, m := sample()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := m
	bad.Version = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("empty version must be rejected")
	}
	bad = m
	bad.Expires = time.Time{}
	if err := bad.Validate(); err == nil {
		t.Fatal("empty expiry must be rejected")
	}
}
