package ether

// registry.go is generated from contracts/evm/Registry.sol: forge compiles it (solc 0.8.28) and abigen produces the binding.
// Regenerate it whenever the contract changes. forge is at the default foundryup location (~/.foundry/bin). abigen is run
// with go run because go-ethereum is already a dependency, so neither solc nor abigen needs a separate install:
//
//go:generate sh -c "cd ../../../../../contracts/evm && printf '@openzeppelin/contracts/=lib/openzeppelin-contracts/contracts/\n' > remappings.txt && $HOME/.foundry/bin/forge build --root . --contracts Registry.sol --optimize --optimizer-runs 200 --evm-version cancun --use 0.8.28 && rm remappings.txt"
//go:generate sh -c "cd ../../../../../contracts/evm && python -c \"import json,io;j=json.load(io.open('out/Registry.sol/Registry.json',encoding='utf-8'));io.open('out/Registry.abi','w').write(json.dumps(j['abi']));io.open('out/Registry.bin','w').write(j['bytecode']['object'][2:])\""
//go:generate go run github.com/ethereum/go-ethereum/cmd/abigen --abi ../../../../../contracts/evm/out/Registry.abi --bin ../../../../../contracts/evm/out/Registry.bin --pkg ether --type Registry --out registry.go
