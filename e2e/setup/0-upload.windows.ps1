# 준비 — 바이너리를 리눅스용으로 빌드해 두 인스턴스에 올린다 (누가 하든 한 번). 응답기는 같은 파일 하나를 양쪽에.
#    실행: TB-live 에서  .\e2e\setup\0-upload.windows.ps1
. "$PSScriptRoot\..\setup\env.windows.ps1"
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
Invoke-Expression "$($E2E.Scp) tb-vendor tb-ra-responder e2e/vendor/1-pack-image.nitro.sh e2e/vendor/1-build-source.nitro.sh e2e/vendor/9b-demo-swap-enclave.nitro.sh ${v}:~/"
Invoke-Expression "$($E2E.Scp) tb-auditor tb-ra-responder e2e/auditor/3-check-received-image.nitro.sh e2e/auditor/3-rebuild-source.nitro.sh ${a}:~/"
V 'chmod +x tb-vendor tb-ra-responder *.sh; sed -i "s/\r$//" *.sh; sha256sum tb-ra-responder'
A 'chmod +x tb-auditor tb-ra-responder *.sh; sed -i "s/\r$//" *.sh; sha256sum tb-ra-responder'
"uploaded. next: on the Vendor instance  ./1-pack-image.nitro.sh  (your own docker image)  or  ./1-build-source.nitro.sh  (tb builds the source)"
