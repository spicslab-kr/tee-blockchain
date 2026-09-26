package ether

// registry.go 는 contracts/evm/Registry.sol 을 forge(solc 0.8.28) 로 컴파일해 abigen 으로 만든 것이다.
// 컨트랙트를 고치면 다시 만든다. forge 는 foundryup 기본 위치(~/.foundry/bin)에 있고, abigen 은 go-ethereum 이
// 이미 의존성이라 go run 으로 부른다 (solc · abigen 을 따로 설치할 필요 없다):
//
//go:generate sh -c "cd ../../../../../contracts/evm && printf '@openzeppelin/contracts/=lib/openzeppelin-contracts/contracts/\n' > remappings.txt && $HOME/.foundry/bin/forge build --root . --contracts Registry.sol --optimize --optimizer-runs 200 --evm-version cancun --use 0.8.28 && rm remappings.txt"
//go:generate sh -c "cd ../../../../../contracts/evm && python -c \"import json,io;j=json.load(io.open('out/Registry.sol/Registry.json',encoding='utf-8'));io.open('out/Registry.abi','w').write(json.dumps(j['abi']));io.open('out/Registry.bin','w').write(j['bytecode']['object'][2:])\""
//go:generate go run github.com/ethereum/go-ethereum/cmd/abigen --abi ../../../../../contracts/evm/out/Registry.abi --bin ../../../../../contracts/evm/out/Registry.bin --pkg ether --type Registry --out registry.go
