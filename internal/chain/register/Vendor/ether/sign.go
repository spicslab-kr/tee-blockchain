// Package ether 는 이더리움(EVM) 체인에 등록한다. internal/chain/ether/Registry.sol 과 짝이다.
// registry.go 는 그 컨트랙트에서 abigen 으로 만든 바인딩이다 (generate.go).
package ether

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	reg "tb/internal/chain/register/Vendor"
	p "tb/internal/tee/parse/Vendor"
)

// Registrar 는 RPC 로 Registry 컨트랙트 하나를 바라본다.
type Registrar struct {
	RPC      string         // 예: https://ethereum-sepolia-rpc.publicnode.com
	Contract common.Address // 배포된 Registry
	// SenderKey 는 트랜잭션을 보내고 가스를 내는 키. 서명자와 같아도, 달라도 된다.
	SenderKey []byte
	Timeout   time.Duration // 채굴 대기. 0 이면 2 분
	// Backend 가 있으면 RPC 대신 쓴다. 테스트에서 simulated backend 를 넣는다.
	Backend Backend
}

var _ reg.Registrar = (*Registrar)(nil)

// toEntry — 우리 타입을 컨트랙트의 Entry 로. 측정값은 32 바이트 해시 하나로 줄어든다.
func toEntry(values p.Measurements, meta reg.Meta) (RegistryEntry, error) {
	if err := meta.Validate(); err != nil {
		return RegistryEntry{}, err
	}
	h, err := reg.MeasurementHash(values)
	if err != nil {
		return RegistryEntry{}, err
	}
	return RegistryEntry{Platform: meta.Platform, Service: meta.Service, Version: meta.Version, Measurement: h, Expires: uint64(meta.Expires.Unix())}, nil
}

// Digest 는 서명 대상 해시 — 컨트랙트의 digest(e) 와 같은 값. 체인 ID 와 컨트랙트 주소가 들어가 다른 배포에는 못 쓴다.
func Digest(chainID *big.Int, contract common.Address, e RegistryEntry) ([]byte, error) {
	td := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"}, {Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"},
			},
			"Entry": {
				{Name: "platform", Type: "string"}, {Name: "service", Type: "string"}, {Name: "version", Type: "string"},
				{Name: "measurement", Type: "bytes32"}, {Name: "expires", Type: "uint64"},
			},
		},
		PrimaryType: "Entry",
		Domain: apitypes.TypedDataDomain{
			Name: "Registry", Version: "1",
			ChainId: (*math.HexOrDecimal256)(chainID), VerifyingContract: contract.Hex(),
		},
		Message: apitypes.TypedDataMessage{
			"platform": e.Platform, "service": e.Service, "version": e.Version,
			"measurement": e.Measurement[:], "expires": new(big.Int).SetUint64(e.Expires),
		},
	}
	digest, _, err := apitypes.TypedDataAndHash(td)
	return digest, err
}

// Sign 은 체인 ID 를 읽어 서명한다. 키는 secp256k1 개인키 32 바이트.
func (r Registrar) Sign(values p.Measurements, meta reg.Meta, key []byte) (reg.Signed, error) {
	priv, err := crypto.ToECDSA(key)
	if err != nil {
		return reg.Signed{}, fmt.Errorf("signing key: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	backend, done, err := r.backend(ctx)
	if err != nil {
		return reg.Signed{}, err
	}
	defer done()
	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return reg.Signed{}, err
	}
	return r.signWith(chainID, values, meta, priv)
}

func (r Registrar) signWith(chainID *big.Int, values p.Measurements, meta reg.Meta, priv *ecdsa.PrivateKey) (reg.Signed, error) {
	e, err := toEntry(values, meta)
	if err != nil {
		return reg.Signed{}, err
	}
	digest, err := Digest(chainID, r.Contract, e)
	if err != nil {
		return reg.Signed{}, err
	}
	sig, err := crypto.Sign(digest, priv)
	if err != nil {
		return reg.Signed{}, err
	}
	sig[64] += 27 // 컨트랙트(ECDSA.recover)는 v 를 27/28 로 받는다
	return reg.Signed{
		Values: values, Meta: meta,
		Signer:    crypto.PubkeyToAddress(priv.PublicKey).Hex(),
		Digest:    digest,
		Signature: sig,
	}, nil
}

// backend — Backend 가 있으면 그것, 없으면 RPC 에 연결한다. done 은 연결을 닫는다.
func (r Registrar) backend(ctx context.Context) (Backend, func(), error) {
	if r.Backend != nil {
		return r.Backend, func() {}, nil
	}
	client, err := ethclient.DialContext(ctx, r.RPC)
	if err != nil {
		return nil, nil, err
	}
	return client, client.Close, nil
}
