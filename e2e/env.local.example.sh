# Copy to e2e/env.local.sh (gitignored) and fill in your instances' public IPs or DNS names.
VENDOR_HOST=203.0.113.10      # Nitro-enabled instance that runs the Vendor role
AUDITOR_HOST=203.0.113.20     # instance that runs the Auditor role
ARM64_HOST=                   # optional arm64 instance for the architecture checks; when empty those scripts skip the arm64 side
# The Ethereum addresses of your auditor.key and deployer.key (both need Sepolia test ETH). Required by every script
# that deploys or registers; without them those scripts stop with an error. The build-only checks do not need them.
AUDITOR_ADDR=
VENDOR_ADDR=
