# Copy to e2e/env.local.sh (gitignored) and fill in your instances' public IPs or DNS names.
VENDOR_HOST=203.0.113.10      # Nitro-enabled instance that runs the vendor role
AUDITOR_HOST=203.0.113.20     # instance that runs the auditor role
ARM64_HOST=                   # optional arm64 instance for the architecture checks; when empty those scripts skip the arm64 side
# The Ethereum addresses of your auditor.key and deployer.key (both need Sepolia test ETH). Without these lines
# env.sh uses the original author's addresses, and a registry deployed with your keys would name the wrong approver.
#AUDITOR_ADDR=0x...
#VENDOR_ADDR=0x...
