# Copy to e2e/env.local.sh (gitignored) and fill in your instances' public IPs or DNS names.
VENDOR_HOST=203.0.113.10      # Nitro-enabled instance that runs the vendor role
AUDITOR_HOST=203.0.113.20     # instance that runs the auditor role
ARM64_HOST=                   # optional arm64 instance for the architecture checks; when empty those scripts skip the arm64 side
