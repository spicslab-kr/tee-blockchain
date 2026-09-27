# 조작 · Vendor — 바꿔 띄운 이미지(2-swap-enclave)의 값을 자기 등록부에 등록한다 (설계 흐름 2 ②의 "A 여야 하는데 B 를 등록").
#    정상 흐름의 8-self-register 를 out-tampered 값으로 돌리는 것. 컨트랙트는 e2e-work\contract-flow2.txt 에 남는다.
#    실행: .\e2e\tamper\vendor\(local)\windows\3-self-register-swapped.ps1
& "$PSScriptRoot\..\..\..\..\normal\vendor\(local)\windows\8-self-register.ps1" -RemoteOut out-tampered
