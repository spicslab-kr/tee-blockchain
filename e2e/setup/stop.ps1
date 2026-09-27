# 정리 — 두 인스턴스의 엔클레이브와 TCP 다리를 내린다. (인스턴스 자체는 콘솔에서 Stop)
. "$PSScriptRoot\..\setup\env.ps1"
V 'sudo nitro-cli terminate-enclave --all; pkill -x tb-vendor; echo vendor: enclaves $(nitro-cli describe-enclaves | tr -d "\n ")'
A 'sudo nitro-cli terminate-enclave --all; pkill -x tb-auditor; echo auditor: enclaves $(nitro-cli describe-enclaves | tr -d "\n ")'
