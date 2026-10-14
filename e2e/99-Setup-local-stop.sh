#!/usr/bin/env bash
# Cleanup - stops the enclaves and the TCP bridge processes (att-vendor, att-auditor) on both instances.
#    The instances themselves are stopped from the AWS console.
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"
step "STOPPING ENCLAVES"
V 'sudo nitro-cli terminate-enclave --all; pkill -x att-vendor; echo vendor: enclaves $(nitro-cli describe-enclaves | tr -d "\n ")'
A 'sudo nitro-cli terminate-enclave --all; pkill -x att-auditor; echo auditor: enclaves $(nitro-cli describe-enclaves | tr -d "\n ")'
