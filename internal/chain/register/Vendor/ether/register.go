package ether

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	reg "tb/internal/chain/register/Vendor"
)

// Backend is what is needed to talk to the chain. Both *ethclient.Client and simulated.Client satisfy it.
type Backend interface {
	bind.ContractBackend
	bind.DeployBackend
	ChainID(ctx context.Context) (*big.Int, error)
}

// Register sends the register(e, signature) transaction and waits for it to be mined. If the contract rejects it, the reason is returned as an error.
func (r Registrar) Register(ctx context.Context, s reg.Signed) (reg.Receipt, error) {
	if len(r.SenderKey) == 0 {
		return reg.Receipt{}, errors.New("sender key is required to pay for the transaction")
	}
	sender, err := crypto.ToECDSA(r.SenderKey)
	if err != nil {
		return reg.Receipt{}, fmt.Errorf("sender key: %w", err)
	}
	e, err := toEntry(s.Values, s.Meta)
	if err != nil {
		return reg.Receipt{}, err
	}
	backend, done, err := r.backend(ctx)
	if err != nil {
		return reg.Receipt{}, err
	}
	defer done()
	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return reg.Receipt{}, err
	}
	opts, err := bind.NewKeyedTransactorWithChainID(sender, chainID)
	if err != nil {
		return reg.Receipt{}, err
	}
	opts.Context = ctx
	registry, err := NewRegistry(r.Contract, backend)
	if err != nil {
		return reg.Receipt{}, err
	}
	tx, err := registry.Register(opts, e, s.Signature)
	if err != nil {
		return reg.Receipt{}, classify(err)
	}
	to := r.Timeout
	if to == 0 {
		to = 2 * time.Minute
	}
	wctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	receipt, err := bind.WaitMined(wctx, backend, tx)
	if err != nil {
		return reg.Receipt{}, fmt.Errorf("waiting for tx %s: %w", tx.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return reg.Receipt{}, fmt.Errorf("tx %s reverted", tx.Hash().Hex())
	}
	for _, lg := range receipt.Logs {
		if ev, err := registry.ParseRegistered(*lg); err == nil {
			return reg.Receipt{Key: common.Hash(ev.Key).Hex(), TxHash: tx.Hash().Hex(), Block: receipt.BlockNumber.Uint64()}, nil
		}
	}
	return reg.Receipt{}, errors.New("Registered event not found in receipt")
}

// classify turns the revert data returned by the contract into our errors.
func classify(err error) error {
	var de interface{ ErrorData() interface{} }
	if !errors.As(err, &de) {
		return err
	}
	hexData, ok := de.ErrorData().(string)
	if !ok {
		return err
	}
	data := common.FromHex(hexData)
	if len(data) < 4 {
		return err
	}
	parsed, aerr := RegistryMetaData.GetAbi()
	if aerr != nil {
		return err
	}
	for name, e := range parsed.Errors {
		if [4]byte(e.ID[:4]) != [4]byte(data[:4]) {
			continue
		}
		unpacked, _ := e.Unpack(data)
		args, _ := unpacked.([]any)
		for len(args) < 2 {
			args = append(args, nil)
		}
		switch name {
		case "NotAnApprover":
			return fmt.Errorf("%w: signer %v", reg.ErrNotAnApprover, args[0])
		case "AlreadyRegistered":
			key, _ := args[0].([32]byte)
			return fmt.Errorf("%w: key %s, expires %s", reg.ErrAlreadyRegistered, common.Hash(key).Hex(), unixTime(args[1]))
		case "ExpiryAlreadyPassed":
			return fmt.Errorf("%w: expires %s, chain time %s", reg.ErrExpiryPassed, unixTime(args[0]), unixTime(args[1]))
		default:
			return fmt.Errorf("contract rejected: %s%v", name, args)
		}
	}
	return err
}

func unixTime(v any) string {
	if u, ok := v.(uint64); ok {
		return time.Unix(int64(u), 0).UTC().Format(time.RFC3339)
	}
	return fmt.Sprint(v)
}
