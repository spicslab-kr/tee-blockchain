# 준비 — 바이너리를 리눅스용으로 빌드해 두 인스턴스에 올린다 (누가 하든 한 번). 응답기는 같은 파일 하나를 양쪽에.
#    실행: TB-live 에서  .\e2e\setup\(local)\windows\0-upload.ps1
. "$PSScriptRoot\env.ps1"
$ErrorActionPreference = "Stop"

$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o tb-vendor ./cmd/vendor
go build -o tb-auditor ./cmd/auditor
go build -o tb-ra-responder ./cmd/ra-responder
Remove-Item Env:\GOOS, Env:\GOARCH
go build -o tb-auditor.exe ./cmd/auditor
go build -o tb-vendor.exe ./cmd/vendor
go build -o tb-user.exe ./cmd/user
"responder sha256: " + (Get-FileHash tb-ra-responder -Algorithm SHA256).Hash.ToLower()

$v = "$($E2E.SshUser)@$($E2E.VendorHost)"; $a = "$($E2E.SshUser)@$($E2E.AuditorHost)"
# a running ra-proxy holds the old binary open — stop it (and any enclave) before overwriting
V 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-vendor; true'
A 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-auditor; true'
Invoke-Expression "$($E2E.Scp) tb-vendor tb-ra-responder 'e2e/normal/vendor/(nitro)/4-launch-received.sh' 'e2e/flow2/vendor/(nitro)/1-build-source.sh' 'e2e/tamper/vendor/(nitro)/1-swap-enclave.sh' ${v}:~/"
Invoke-Expression "$($E2E.Scp) tb-auditor tb-ra-responder 'e2e/normal/auditor/(nitro)/1-build-source.sh' ${a}:~/"
V 'chmod +x tb-vendor tb-ra-responder *.sh; sed -i "s/\r$//" *.sh; sha256sum tb-ra-responder'
A 'chmod +x tb-auditor tb-ra-responder *.sh; sed -i "s/\r$//" *.sh; sha256sum tb-ra-responder'
"uploaded. next: on the Auditor instance  ./1-build-source.sh   (the Auditor builds; the Vendor deploys what it gets)"
