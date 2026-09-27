# e2e/env.ps1 — 노트북 쪽 스크립트가 읽는 값. 자기 환경에 맞게 여기만 고친다.
# 인스턴스 쪽 스크립트(vendor/*.sh, auditor/*.sh)는 IP 를 모르며 소스·이름만 안다.

$E2E = @{
  # 두 Nitro 인스턴스 (eu-north-1). Vendor 는 빌드·띄우기, Auditor 는 검증·띄우기
  VendorHost  = "13.61.175.104"      # i-01f1ef6b04c10f37f
  AuditorHost = "13.60.50.242"       # i-0200ce262d24549ea
  SshUser     = "ec2-user"
  Pem         = "pem2.pem"           # TB-live 기준 상대경로

  # 체인 (Sepolia)
  Rpc         = "https://ethereum-sepolia-rpc.publicnode.com"
  AuditorKey  = "auditor.key"        # 0xD2082a2513781b29beeF5DB847b2c85Cbc50ECeE — 흐름 1 의 approver
  VendorKey   = "deployer.key"       # 0xAf48305C64683eB72C28be61dD0E41478A2a0Df5 — 가스 지불 · 흐름 2 의 approver
  AuditorAddr = "0xD2082a2513781b29beeF5DB847b2c85Cbc50ECeE"
  VendorAddr  = "0xAf48305C64683eB72C28be61dD0E41478A2a0Df5"

  # 등록에 적을 것
  Service     = "nginx"
  Version     = "1.31.6"

  # 인스턴스 쪽 EIF 이름 (vendor/pack-image.sh 는 out/nginx.eif, build-source.sh 는 out/alpine-slim.eif)
  EifPacked   = "out/nginx.eif"
  EifBuilt    = "out/alpine-slim.eif"
}

$E2E.Ssh = "ssh -i $($E2E.Pem) -o StrictHostKeyChecking=no"
$E2E.Scp = "scp -q -i $($E2E.Pem) -o StrictHostKeyChecking=no"
function V { param($cmd) Invoke-Expression "$($E2E.Ssh) $($E2E.SshUser)@$($E2E.VendorHost) '$cmd'" }
function A { param($cmd) Invoke-Expression "$($E2E.Ssh) $($E2E.SshUser)@$($E2E.AuditorHost) '$cmd'" }
