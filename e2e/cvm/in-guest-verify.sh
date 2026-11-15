#!/bin/sh
# Phase 2.3 inside one confidential VM (docs/plan/cvm-port.md): att-ra-responder makes fresh evidence on the guest,
# and the whole chain runs against it on 127.0.0.1 - request, nonce check, replay refusal, registry deploy on a local
# anvil, audit (which registers), and verify. No firewall rule is needed; the only part it leaves out is a verifier on
# another machine.
#
# Usage, on the guest, from a folder holding att-ra-responder, att-auditor, att-user and anvil (all linux/amd64):
#   sh in-guest-verify.sh tdx        intel-tdx
#   sh in-guest-verify.sh snp-initrd amd-sev-snp-initrd
#   sh in-guest-verify.sh snp-verity amd-sev-snp-verity
# The responder runs as root (ConfigFS TSM and /dev/tpmrm0 need it). Prints PASS/FAIL per check and exits non-zero
# if any check failed. Everything it writes goes to a temporary folder it prints, including results.jsonl: one line per
# check in the results contract of e2e/env.sh (docs/plan/test-matrix.md), with the format string as the platform. Copy
# it back and append it to e2e-work/results/<format>.jsonl.
set -u
EX=$1
FMT=$2
W=$(mktemp -d)
echo "work folder: $W"
RPC=http://127.0.0.1:8545
TEE=127.0.0.1:5000
RUN=$(date -u +%Y%m%dT%H%M%SZ)
# anvil's well-known development keys (from its default mnemonic, printed by every anvil start). Not secrets.
printf '%s\n' ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 > "$W/deployer.key"
printf '%s\n' 59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d > "$W/auditor.key"
AUDITOR=0x70997970C51812dc3A010C7d01b50e0d17dc79C8

json_str() { printf '%s' "$1" | tr '\r\n\t' '   ' | tr -d '\000-\037' | sed -e 's/\/\\/g' -e 's/"/\\"/g'; }
# record <test> <check> <result> <expected> <reason>: one line of the results contract.
record() {
	printf '{"platform":"%s","chain":"local","test":"%s","check":"%s","script":"e2e/cvm/in-guest-verify.sh","result":"%s","expected":"%s","reason":"%s","detail":"extractor %s, on the guest over 127.0.0.1","run":"%s","at":"%s"}\n' \
		"$(json_str "$FMT")" "$1" "$2" "$3" "$4" "$(json_str "$5")" "$EX" "$RUN" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$W/results.jsonl"
}

pass=0
fail=0
# check <rc> <test> <check> <expected result> <text>: PASS when rc is 0, and a results line either way (a failed check
# is recorded as "error"; its reason says what was seen).
check() {
	if [ "$1" -eq 0 ]; then
		echo "PASS  $5"; pass=$((pass + 1)); record "$2" "$3" "$4" "$4" "$5"
	else
		echo "FAIL  $5"; fail=$((fail + 1)); record "$2" "$3" error "$4" "FAILED: $5"
	fi
}

./anvil --port 8545 --silent > "$W/anvil.log" 2>&1 &
ANVIL=$!
sudo ./att-ra-responder ra-serve --extractor "$EX" --listen "tcp:$TEE" > "$W/responder.log" 2>&1 &
trap 'sudo pkill -f "att-ra-responder ra-serve" 2>/dev/null; kill $ANVIL 2>/dev/null' EXIT
sleep 3

# 1. Two fresh requests. Each nonce is made by att-user and saved next to the document.
./att-user ra-request --tee-addr "$TEE" --format "$FMT" --out-file "$W/a.doc"
check $? - request-1-answered accepted "request 1 answered with a $FMT document"
./att-user ra-request --tee-addr "$TEE" --format "$FMT" --out-file "$W/b.doc"
check $? - request-2-answered accepted "request 2 answered"

# 2. Each document verifies and is bound to the nonce sent with it (the nonce check must run, not be skipped).
./att-user ra-parse --doc "$W/a.doc" --format "$FMT" --out-file "$W/a.json" > "$W/parse-a.txt" 2>&1
check $? - document-1-verifies accepted "document 1: signature, chain and nonce verify"
grep -q "nonce      ok" "$W/parse-a.txt"
check $? T9 document-1-nonce-checked accepted "document 1: the nonce check ran and passed"
./att-user ra-parse --doc "$W/b.doc" --format "$FMT" --out-file "$W/b.json" > "$W/parse-b.txt" 2>&1
check $? - document-2-verifies accepted "document 2: signature, chain and nonce verify"

# 3. Two requests, two different documents (fresh evidence, not a recording).
if cmp -s "$W/a.doc" "$W/b.doc"; then check 1 - two-documents-differ different "the two documents differ"; else check 0 - two-documents-differ different "the two documents differ"; fi

# 4. Replay (test-matrix T9): document 1 presented against request 2's nonce must be refused (exit code 6, nonce).
./att-user ra-parse --doc "$W/a.doc" --format "$FMT" --nonce "$W/b.doc.nonce" --out-file "$W/replay.json" > "$W/replay.txt" 2>&1
rc=$?
if [ "$rc" -eq 6 ]; then check 0 T9 replay-refused refused "replayed document refused (exit 6)"; else check 1 T9 replay-refused refused "replayed document refused (exit 6, got $rc)"; fi

# 5. The chain: a registry on the local anvil, the Auditor audits the live guest and registers it, the User verifies.
./att-auditor registry-deploy --rpc "$RPC" --deployer-key-file "$W/deployer.key" --approver "$AUDITOR" > "$W/deploy.txt" 2>&1
check $? - registry-deployed accepted "registry deployed on anvil"
REG=$(sed -n 's/.*Registry deployed at \(0x[0-9a-fA-F]\{40\}\).*/\1/p' "$W/deploy.txt" | head -1)
echo "registry: $REG"
# The Auditor's own build would produce these values; here they are document 1's (no build exists on the guest yet).
./att-auditor audit --tee-addr "$TEE" --format "$FMT" --build-measurements "$W/a.json" --rpc "$RPC" --registry "$REG" \
	--signing-key-file "$W/auditor.key" --service in-guest --version 1.0.0 --out-dir "$W/audit" > "$W/audit.txt" 2>&1
check $? T4 auditor-audit-registers accepted "audit: a fresh document from the guest matched and was registered (values from the guest, not a rebuild)"
./att-user verify --tee-addr "$TEE" --format "$FMT" --rpc "$RPC" --registry "$REG" --out-dir "$W/verify" > "$W/verify.txt" 2>&1
rc=$?
check $rc T4 user-verify-exit accepted "verify exits 0"
grep -q "✓ matches" "$W/verify.txt"
check $? T4 user-verify-matches accepted "verify prints a '✓ matches' line"

echo "---- verify output"
cat "$W/verify.txt"
echo "---- results: $W/results.jsonl"
echo "---- $pass passed, $fail failed"
[ "$fail" -eq 0 ]
