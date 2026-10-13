#!/usr/bin/env bash
# Cleanup - stop the enclaves and TCP bridges on both instances. (Stop the instances themselves from the console.)
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"
step "STOPPING ENCLAVES"
V 'sudo nitro-cli terminate-enclave --all; pkill -x att-vendor; echo vendor: enclaves $(nitro-cli describe-enclaves | tr -d "\n ")'
A 'sudo nitro-cli terminate-enclave --all; pkill -x att-auditor; echo auditor: enclaves $(nitro-cli describe-enclaves | tr -d "\n ")'
