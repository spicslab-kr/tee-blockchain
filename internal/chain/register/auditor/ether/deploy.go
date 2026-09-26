package ether

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Deploy 는 Registry 를 배포한다. 서명자(approver) 주소는 배포 뒤 바꿀 수 없다. 바꾸려면 새로 배포한다.
// 컨트랙트 하나 = 서명자 하나 — 각 역할이 자기 주소로 자기 컨트랙트를 배포한다.
// backend 가 nil 이면 rpc 에 연결한다.
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
