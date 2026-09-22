package ether

// registry.go 는 contracts/evm/Registry.sol 을 solc 0.8.28 로 컴파일해 abigen 으로 만든 것이다.
// 컨트랙트를 고치면 다시 만든다 (solc, abigen 이 PATH 에 있어야 한다):
//
//go:generate sh -c "cd ../../../../../contracts/evm && solc --base-path . --include-path lib/openzeppelin-contracts/contracts @openzeppelin/contracts/=lib/openzeppelin-contracts/contracts/ --optimize --optimize-runs 200 --evm-version cancun --abi --bin --overwrite -o /tmp/tb-registry Registry.sol"
//go:generate abigen --abi /tmp/tb-registry/Registry.abi --bin /tmp/tb-registry/Registry.bin --pkg ether --type Registry --out registry.go
