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

	reg "attestamp/internal/chain/register/Vendor"
)

// Backend is what Registrar and Deploy need from the chain. Both *ethclient.Client and simulated.Client satisfy it.
type Backend interface {
	bind.ContractBackend
	bind.DeployBackend
	ChainID(ctx context.Context) (*big.Int, error)
}

// Register sends the register(registration, signature) transaction and waits for it to be mined. A contract rejection is returned as the matching error.
func (r Registrar) Register(ctx context.Context, s reg.Signed) (reg.Receipt, error) {
	if len(r.SenderKey) == 0 {
		return reg.Receipt{}, errors.New("sender key is required to pay for the transaction")
	}
	sender, err := crypto.ToECDSA(r.SenderKey)
	if err != nil {
		return reg.Receipt{}, fmt.Errorf("sender key: %w", err)
	}
	e, err := toRegistration(s.Values, s.Meta)
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
	registry, err := NewRegistry(r.RegistryAddr, backend)
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
			return reg.Receipt{RecordKey: common.Hash(ev.RecordKey).Hex(), TxHash: tx.Hash().Hex(), Block: receipt.BlockNumber.Uint64()}, nil
		}
	}
	return reg.Receipt{}, errors.New("Registered event not found in receipt")
}

// classify maps the contract's revert data to the errors in the register package.
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
			return fmt.Errorf("%w: approver %v", reg.ErrNotAnApprover, args[0])
		case "AlreadyRegistered":
			recordKey, _ := args[0].([32]byte)
			return fmt.Errorf("%w: record key %s, expires %s", reg.ErrAlreadyRegistered, common.Hash(recordKey).Hex(), unixTime(args[1]))
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
