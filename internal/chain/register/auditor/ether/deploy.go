package ether

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Deploy deploys Registry. The approver address cannot be changed after deployment; to change it, deploy anew.
// One contract, one approver: each role deploys its own contract with its own address.
// If backend is nil it connects to rpc.
func Deploy(ctx context.Context, rpc string, backend Backend, deployerKey []byte, approver common.Address) (common.Address, string, error) {
	deployer, err := crypto.ToECDSA(deployerKey)
	if err != nil {
		return common.Address{}, "", fmt.Errorf("deployer key: %w", err)
	}
	if backend == nil {
		client, err := ethclient.DialContext(ctx, rpc)
		if err != nil {
			return common.Address{}, "", err
		}
		defer client.Close()
		backend = client
	}
	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return common.Address{}, "", err
	}
	opts, err := bind.NewKeyedTransactorWithChainID(deployer, chainID)
	if err != nil {
		return common.Address{}, "", err
	}
	opts.Context = ctx
	addr, tx, _, err := DeployRegistry(opts, backend, approver)
	if err != nil {
		return common.Address{}, "", err
	}
	if _, err := bind.WaitDeployed(ctx, backend, tx); err != nil {
		return common.Address{}, "", err
	}
	return addr, tx.Hash().Hex(), nil
}
