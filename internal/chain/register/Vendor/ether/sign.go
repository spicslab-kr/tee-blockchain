// Package ether registers on an Ethereum (EVM) chain. It pairs with contracts/evm/Registry.sol.
// registry.go is the abigen binding generated from that contract (see generate.go).
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

	reg "attestamp/internal/chain/register/Vendor"
	p "attestamp/internal/tee/parse/Vendor"
)

// Registrar talks to a single Registry contract over RPC.
type Registrar struct {
	RPC      string         // e.g. https://ethereum-sepolia-rpc.publicnode.com
	Contract common.Address // the deployed Registry
	// SenderKey is the key that sends the transaction and pays gas. It may or may not be the signer.
	SenderKey []byte
	Timeout   time.Duration // how long to wait for mining; 0 means 2 minutes
	// If Backend is set it is used instead of RPC. Tests inject a simulated backend.
	Backend Backend
}

var _ reg.Registrar = (*Registrar)(nil)

// toEntry converts our types into the contract's Entry. The measurements are reduced to a single 32-byte hash.
func toEntry(values p.Measurements, meta reg.Meta) (RegistryEntry, error) {
	if err := meta.Validate(); err != nil {
		return RegistryEntry{}, err
	}
	h, err := reg.MeasurementHash(values)
	if err != nil {
		return RegistryEntry{}, err
	}
	return RegistryEntry{Platform: meta.Platform, Arch: meta.Arch, Service: meta.Service, Version: meta.Version, Measurement: h, Expires: uint64(meta.Expires.Unix())}, nil
}

// Digest is the hash to sign, the same value as the contract's digest(e). It includes the chain ID and contract address,
// so it cannot be reused on another deployment.
func Digest(chainID *big.Int, contract common.Address, e RegistryEntry) ([]byte, error) {
	td := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"}, {Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"},
			},
			"Entry": {
				{Name: "platform", Type: "string"}, {Name: "arch", Type: "string"}, {Name: "service", Type: "string"},
				{Name: "version", Type: "string"}, {Name: "measurement", Type: "bytes32"}, {Name: "expires", Type: "uint64"},
			},
		},
		PrimaryType: "Entry",
		Domain: apitypes.TypedDataDomain{
			Name: "Registry", Version: "1",
			ChainId: (*math.HexOrDecimal256)(chainID), VerifyingContract: contract.Hex(),
		},
		Message: apitypes.TypedDataMessage{
			"platform": e.Platform, "arch": e.Arch, "service": e.Service, "version": e.Version,
			"measurement": e.Measurement[:], "expires": new(big.Int).SetUint64(e.Expires),
		},
	}
	digest, _, err := apitypes.TypedDataAndHash(td)
	return digest, err
}

// Sign reads the chain ID and signs. The key is a 32-byte secp256k1 private key.
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
	sig[64] += 27 // the contract (ECDSA.recover) expects v as 27/28
	return reg.Signed{
		Values: values, Meta: meta,
		Signer:    crypto.PubkeyToAddress(priv.PublicKey).Hex(),
		Digest:    digest,
		Signature: sig,
	}, nil
}

// backend returns Backend if set, otherwise connects to RPC. done closes the connection.
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
