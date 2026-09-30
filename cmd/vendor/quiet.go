package main

import tr "tb/internal/core/transfer/Vendor"

// quiet — audit · verify 처럼 여러 단계를 한 명령으로 묶은 것이 raRequest · raParse · register · lookup 을 부를 때 켠다.
// 그러면 그 함수들은 자기 출력을 내지 않고, 부른 쪽이 "지금 무엇을 하는지" 한 줄씩 찍는다.
var quiet bool

// lastRegistered — register 가 마지막으로 올린 등록. audit / self-register 가 자기 문장으로 찍기 위해 남긴다.
var lastRegistered struct {
	Key, Tx, Expires, Signer string
	BuildEnv                 tr.BuildEnv // 어느 아키텍처·어느 Go 툴체인의 측정값인지 — 체인에는 안 올라가고, 화면과 measurement-record.json 에만 적힌다
	Block                    uint64
}
