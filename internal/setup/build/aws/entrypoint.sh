#!/bin/sh
# When the enclave boots, this file runs first, in place of the original image's command.
# It brings up loopback, starts the RA responder in the background, then runs the original command exactly as written in /att-app-cmd, one argument per line.
# (The original command is passed as a file because nitro-cli cannot handle a Docker CMD together with a script entrypoint.)
# Docker sets HOME at run time but the enclave's init does not, so the same default is given here.
export HOME="${HOME:-/root}"
export PATH="${PATH:-/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin}"
/att-ra lo-up || echo "att: loopback up failed" >&2
/att-ra ra-serve --listen "vsock:${ATT_RA_PORT:-5000}" --extractor nsm &
set --
while IFS= read -r arg; do set -- "$@" "$arg"; done < /att-app-cmd
exec "$@"
