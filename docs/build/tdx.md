# Intel TDX: what is measured, what changes it, and what is not done yet

**Status.** The verifying side works on a recorded quote: `ra-parse`, `audit` and `verify` accept `intel-tdx`
(`internal/tee/parse/tdx`). Requesting a quote from a running guest works (`att-ra-responder ra-serve --extractor
tdx`; the whole chain passed on a live guest on 2026-11-12). Building the initrd and launching it are in the tool since
2026-11-15 (`build` and `launch --platform intel-tdx`, both options, `--verify`), and **ran on real guests on
2026-11-15**: install, reboot and `launch --check` passed for both options, and `audit` and `verify` passed from
another machine (`docs/results/trial-2026-11-16.md`). In those runs the Auditor registered the Vendor's guest's own values.
**The independent Auditor ran on 2026-11-16** ("The Auditor's half", two guests, both options): it rebuilt on its own
guest, got the Vendor's values, registered them, and a User's `verify` of the Vendor's guest passed; a one-byte change
on the Vendor's side was refused (`docs/results/sessions-2026-11-16/intel-tdx.md`).
The audit-and-verify chain test (`TestRun_E2E_TDX_AuditorUser`) passes against a local anvil chain (first run
2026-11-11, anvil 1.8.0). Tools and versions: [toolchain.md](toolchain.md).

Statements are marked **confirmed** (read from our recorded quote, a spec, a library's source or an official page),
**observed** (seen in our runs) or **not confirmed**.

## Getting started: from nothing to a verified TDX guest

This takes one TDX guest on Google Cloud and your own computer, and goes all the way: build the application's
initrd on the guest, boot it, read the measurements, register them on a chain, and have a User verify the guest. Type
the commands in order. `<guest-ip>` and `<0x…>` are values an earlier step prints; in "The Auditor's half" also `<auditor-guest-ip>`, the image name and the commit the Vendor hands over.

**What steps 1 to 13 show, and what they do not.** They run against **one** guest, and in step 11 the Auditor
registers the values the **Vendor's** guest measured, without rebuilding anything. That is **not** the trust model
README describes ("an independent Auditor rebuilds it, confirms the enclave really runs those bytes"): a registration
made that way says no more than the Vendor's own word. It shows that the chain works end to end. The Auditor's
independent rebuild is "The Auditor's half" below, steps A1 to A9, which replace step 11; it needs two guests at once
and **ran and passed on 2026-11-16**, both options. The procedure was followed end to end in a clean environment on 2026-11-15
(`docs/results/trial-2026-11-16.md`), and again from the documents alone, both options, on 2026-11-16
(`docs/results/trial-2026-11-16-clean.md`); the places where either needed knowledge from outside this page have been
filled in since. If a step stops with an error, that is a finding; note it.

**Names.** The commands use the guest name `attestamp-tdx-vendor` and the zone `us-central1-a`. If your guest has
another name (you made `create auditor`, or gave `PREFIX=…` to `gcp-cvm.sh`), replace the name in every `gcloud`
command; `bash e2e/cloud/gcp-cvm.sh tdx status` lists the names. If somebody else in the same project already has an
`attestamp-tdx-vendor`, `create` says `already exists, reusing it (not created)` and makes nothing: choose your own
names with `PREFIX=<you>-tdx` (the guest is then `<you>-tdx-vendor`), and pass the same `PREFIX=` to `status` and
`stop`, which act only on VMs whose name starts with it.

**Cost.** The guest is a `c3-standard-4` and is billed by the hour while it runs. Step 13 stops it.

### Which option

| | Option 1: files in the initrd | Option 2: a dm-verity image in the initrd |
|---|---|---|
| what the boot measures | the initrd, which holds your files (RTMR2) | the initrd, which holds a read-only image of your files and its root hash (RTMR2) |
| where the files are after the boot | they are unpacked into the initramfs; the boot then switches to the disk's root and the initramfs is emptied (that is what initramfs-tools does; not checked on our guest), so **nothing runs from them afterwards** | mounted read-only on `/opt/app-verity`; every block is checked against the root hash when it is read |
| extra work | none | a base initrd built with our two hook scripts (step 4) |
| choose it | to try the whole flow, or for small data only the boot needs | for an application that has to be there while the guest runs |

Both register under the same format, `intel-tdx`, with the same four values (MRTD, RTMR0, RTMR1, RTMR2).

### 1. On your computer: the tools and the source

Everything below runs in `bash`: a terminal on Linux or macOS; on Windows, Git Bash (part of Git for Windows,
https://git-scm.com/download/win) or a WSL distribution. `git`, Go, `gcloud` and the clone must be in the **same**
environment, so that `gcloud compute scp` finds the files the build made.

First `git` (Git for Windows and macOS already have it; on Ubuntu or Debian, also under WSL:
`sudo apt-get install -y git`), then the source, then Go **go1.26.1** exactly, which the repository's install script
fetches:

```
git clone https://github.com/spicslab-kr/tee-blockchain
cd tee-blockchain
bash e2e/0-Setup-local-install.sh
```

The script ends with `Ready - Go 1.26.1 confirmed exactly`. On Linux it puts Go in `/usr/local/go` and adds it to
`PATH` in `~/.bashrc`, which only a **new interactive** shell reads: open a new terminal, or run `source ~/.bashrc`,
and check `go version`. A script or `bash -c` does not read `~/.bashrc`; there, run
`export PATH=$PATH:/usr/local/go/bin` first. On Windows, installing Go needs Git Bash opened **as
administrator** (the script stops and says so otherwise); afterwards open a new Git Bash window, so that `go` is on
`PATH`, and check `go version`. On Ubuntu the script also
installs `build-essential` (gcc); that is why every build in step 2 sets `CGO_ENABLED=0` (a native Linux build with gcc
otherwise links cgo and gives another `att-ra-responder`).

`gcloud`, the Google Cloud CLI. Install it, one way per system:

| system | command |
|---|---|
| Ubuntu or Debian, also under WSL | `sudo apt-get install -y apt-transport-https ca-certificates gnupg curl`, then `curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg \| sudo gpg --dearmor -o /usr/share/keyrings/cloud.google.gpg`, then `echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" \| sudo tee /etc/apt/sources.list.d/google-cloud-sdk.list`, then `sudo apt-get update` and `sudo apt-get install -y google-cloud-cli` |
| Windows (Git Bash) | `winget install --id Google.CloudSDK` in PowerShell, then a new Git Bash window |
| macOS | the archive and `./google-cloud-sdk/install.sh` from https://cloud.google.com/sdk/docs/install |

Under WSL, Windows' own `PATH` is appended, so a `gcloud` installed on the Windows side is found too
(`command -v gcloud` prints a `/mnt/c/…` path). That one works from WSL too (2026-11-16), but slowly: `gcp-cvm.sh
status` took over two minutes. Install the Linux one and check that `command -v gcloud` prints `/usr/bin/gcloud`.
Then sign in, with a project that has billing and C3 quota in `us-central1-a`:

```
gcloud auth login
gcloud config set project <your project id>
```

`anvil` from Foundry, version 1.8.0, for a local chain (no ETH needed; used from step 11). On Linux, macOS or WSL:

```
curl -fsSL https://foundry.paradigm.xyz | bash
source ~/.bashrc
foundryup --install v1.8.0
anvil --version
```

Or, without running an installer script, the release archive itself (Linux x86_64; the release page has the other
systems under the same pattern), checked against the release's checksum file:

```
curl -fsSLO https://github.com/foundry-rs/foundry/releases/download/v1.8.0/foundry_v1.8.0_linux_amd64.tar.gz
curl -fsSLO https://github.com/foundry-rs/foundry/releases/download/v1.8.0/foundry_v1.8.0_linux_amd64.sha256
cat foundry_v1.8.0_linux_amd64.sha256
sha256sum foundry_v1.8.0_linux_amd64.tar.gz
mkdir -p ~/.foundry/bin
tar -xzf foundry_v1.8.0_linux_amd64.tar.gz -C ~/.foundry/bin
export PATH=$PATH:$HOME/.foundry/bin
```

The two hashes must be equal, and equal to the one in [toolchain.md](toolchain.md) (`8c8560de…ac57`). The checksum
file is `…_linux_amd64.sha256`, not `….tar.gz.sha256`.

On Windows without WSL, download `foundry_v1.8.0_win32_amd64.zip` from
https://github.com/foundry-rs/foundry/releases/tag/v1.8.0 and put `anvil.exe` on `PATH`. The `curl` line needs
`curl` (`sudo apt-get install -y curl` on a minimal Ubuntu). Versions we used: [toolchain.md](toolchain.md).

### 2. On your computer: the binaries

In the `tee-blockchain` folder of step 1:

```
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-auditor ./cmd/auditor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-user    ./cmd/user
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o linux/att-vendor       ./cmd/vendor
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o linux/att-ra-responder ./cmd/ra-responder
```

The first two run on your computer; the `linux/` two go to the guest.

### 3. On your computer: the guest

```
bash e2e/cloud/gcp-cvm.sh tdx create vendor --yes
```

This makes **one** VM, `attestamp-tdx-vendor` (`c3-standard-4`, Ubuntu 24.04, TDX, zone `us-central1-a`), and a
firewall rule `attestamp-ra` that lets **your computer's current address** reach port 5000 on it (if the project has no such rule yet; if it has one, made from another computer, it is kept as it is and your address must be added to it: `e2e/cloud/README.md`, "From another computer, or after your address changed"). It ends by printing
the VM's `EXTERNAL_IP`: that is `<guest-ip>` below (`bash e2e/cloud/gcp-cvm.sh tdx status` prints it again).

It also prints the exact image it used, for example `image ubuntu-os-cloud/ubuntu-2404-noble-amd64-v20261105`.
**Write that name down**: whoever rebuilds and checks your values needs a guest from the same image (see "Use the
same image as the other side" below).

The VM takes a minute to boot after `create` returns; if the first `gcloud compute ssh` or `scp` fails to connect, try
again after half a minute. On Windows `gcloud` uses PuTTY, which asks once per new guest whether to store its host key:
answer `y`.

The first `gcloud compute ssh` or `scp` on a computer that has no `~/.ssh/google_compute_engine` yet makes that key
and prints `Updating project ssh metadata`: it adds the public key to the **project**, so every VM in the project,
other people's included, accepts it from then on. That is how `gcloud` works unless the project uses OS Login; if that
is not acceptable in a shared project, ask its owner before this step.

Port 22: a new project opens it to everyone (`default-allow-ssh`). To close that, see `e2e/cloud/README.md`, "SSH to
the guests"; with IAP, add `--tunnel-through-iap` to every `gcloud compute ssh` and `scp` below (checked on
2026-11-16 from Git Bash, both ways, with the `0.0.0.0/0` rule deleted). If plain `gcloud compute ssh` times out, the
project has probably been closed that way: add the flag.

A stopped VM can get a **new** external address when it is started again (seen on 2026-11-15 and 16 on both
platforms). After every `start`, take `<guest-ip>` again from `status`.

Copy the binaries to it, one file per command:

```
gcloud compute scp linux/att-vendor       attestamp-tdx-vendor:att-vendor       --zone us-central1-a
gcloud compute scp linux/att-ra-responder attestamp-tdx-vendor:att-ra-responder --zone us-central1-a
```

For Option 2 also the two hook scripts:

```
gcloud compute scp internal/tee/parse/tdx/testdata/record/initramfs-hook.sh         attestamp-tdx-vendor:initramfs-hook.sh         --zone us-central1-a
gcloud compute scp internal/tee/parse/tdx/testdata/record/initramfs-local-bottom.sh attestamp-tdx-vendor:initramfs-local-bottom.sh --zone us-central1-a
```

Log in:

```
gcloud compute ssh attestamp-tdx-vendor --zone us-central1-a
```

### 4. On the guest: prepare it

Everything in steps 4 to 9 is typed on the guest.

```
chmod +x att-vendor att-ra-responder
sudo shutdown -h +240              # a safety stop in 4 hours; a reboot cancels it, so run it again after each one
```

Make the stock boot load an initrd. Google's Ubuntu images boot without one (`GRUB_FORCE_PARTUUID`, explained below
in "`GRUB_FORCE_PARTUUID`"), and the application is in the initrd. The original file is kept for step 13:

```
grep -n GRUB_FORCE_PARTUUID /etc/default/grub.d/40-force-partuuid.cfg
sudo cp -p /etc/default/grub.d/40-force-partuuid.cfg /root/40-force-partuuid.cfg.orig
sudo sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' /etc/default/grub.d/40-force-partuuid.cfg
sudo update-grub
```

The base initrd. `mkinitramfs` writes a random seed, so two runs never give the same file; the Vendor makes it once
and publishes it, and an Auditor uses this file rather than its own.

Option 1, the distribution's own:

```
sudo cp /boot/initrd.img-$(uname -r) base-initrd.img
sudo chown $USER base-initrd.img
```

Option 2, one with the hook that opens and mounts the image (the scripts are removed again right after, so later
kernel updates do not carry them). It goes into its own file, `base-initrd-v.img`, so that Option 1's
`base-initrd.img` stays as it is and both options can be tried on one guest:

```
sudo apt-get update
sudo apt-get install -y squashfs-tools cryptsetup-bin
sudo install -m 755 initramfs-hook.sh         /etc/initramfs-tools/hooks/attestamp-verity
sudo install -m 755 initramfs-local-bottom.sh /etc/initramfs-tools/scripts/local-bottom/attestamp-verity
sudo mkinitramfs -o base-initrd-v.img $(uname -r)
sudo rm /etc/initramfs-tools/hooks/attestamp-verity /etc/initramfs-tools/scripts/local-bottom/attestamp-verity
sudo chown $USER base-initrd-v.img
sudo mkdir -p /opt/app-verity      # where the image is mounted; the disk's root is read-only when the hook runs
```

### 5. On the guest: the application

The application is a folder of ordinary files; a link or a device file is refused. For a first run:

```
mkdir app
printf 'hello from attestamp\n' > app/hello.txt
```

### 6. On the guest: build

Option 1:

```
./att-vendor build --platform intel-tdx --app-dir app --base-initrd base-initrd.img --option 1 --verify
```

Option 2 (the same with `--option 2`). It makes the image with `mksquashfs` and `veritysetup`, installed in step 4,
and runs as root, as the image of our recordings was made (`internal/setup/build/cpio/testdata/make-verity-image.sh`):

```
sudo ./att-vendor build --platform intel-tdx --app-dir app --base-initrd base-initrd-v.img --option 2 --verify
```

It ends with `✓ reproducible on this host` and `✓ initrd ready: out/initrd-app.img   build value:
out/build-value.json`. `build-value.json` holds the initrd's SHA-384, the digest GRUB will measure.

### 7. On the guest: install and boot

```
cat /proc/sys/kernel/random/boot_id     # note it: it is new on every boot, and tells the new boot from the old below
sudo ./att-vendor launch --platform intel-tdx --initrd out/initrd-app.img --reboot
```

It copies the initrd to `/boot/initrd-attestamp.img`, writes the GRUB entry `attestamp` (the stock entry with only its
title and initrd changed) to `/etc/grub.d/41_attestamp`, makes it the default in
`/etc/default/grub.d/99-attestamp.cfg`, runs `update-grub` and reboots. The connection drops.

The reboot can take a few minutes, and logging in too early reaches the **old** boot, which is still shutting down
(seen on 2026-11-15: a minute was not enough; on 2026-11-16 the new boot answered 55 to 166 seconds after the reboot
command). The surest test is the boot ID noted above. Log in (the `gcloud compute ssh` command of step 3) and check:

```
cat /proc/sys/kernel/random/boot_id   # must differ from the one noted; the same one is the old boot: log out and wait
uptime                                # "up 1 min" or so
cat /proc/cmdline                     # our entry: root=UUID=… ; the stock boot: root=PARTUUID=…
```

Do not rely on the serial console (`gcloud compute instances get-serial-port-output`) for this: it returns only the
newest part of the log, and the old boot's `login:` line looks the same as the new one's (2026-11-16: one wait ended too
early, another never ended).

Then `sudo shutdown -h +240` again.

### 8. On the guest: check the boot and read the values

```
sudo ./att-vendor launch --platform intel-tdx --check
```

It asks the guest for a quote, verifies it, replays the boot's event log against it, requires the initrd GRUB measured
to be the one built, and requires the application to have been unpacked. It ends with `✓ the boot measured the built
initrd` and writes MRTD and RTMR0–2 to `out/measurements.json`. If it says "RTMR2 has no GRUB initrd command, so no
initrd was loaded", you are on the old boot: wait and log in again (step 7). For Option 2, also:

```
cat /run/attestamp-status          # ok <root hash>
ls /opt/app-verity/opt/app         # hello.txt
```

### 9. On the guest: answer attestation requests

```
sudo nohup ./att-ra-responder ra-serve --extractor tdx --listen tcp:0.0.0.0:5000 > ra.log 2>&1 < /dev/null &
```

It is not a service: after every reboot, start it again. Log out (`exit`). The `< /dev/null` matters only when the
line is sent through `gcloud compute ssh --command '…'` instead of typed: without it the background process holds the
session's input and the command does not return.

### 10. On your computer: get the evidence

```
./att-user ra-request --tee-addr <guest-ip>:5000 --format intel-tdx --out-file quote.bin
./att-user ra-parse   --doc quote.bin --format intel-tdx
```

`ra-parse` prints `signature ok`, `nonce ok` and the four values; they are the ones in `out/measurements.json` on the
guest. If `ra-request` times out, your address is not the one the firewall rule allows (it was set when the rule was
first made): `gcloud compute firewall-rules describe attestamp-ra` shows it.

### 11. On your computer: register with audit

> **This step is not an audit.** It registers what the Vendor's guest measured, so the Auditor vouches for values it
> never reproduced. It is here to show the chain working with one guest. An Auditor that means its signature does
> "The Auditor's half" (A1 to A9; run and passed on 2026-11-16) instead of this step.

A local chain, in a second terminal, left running until step 12 is done:

```
anvil --port 8545
```

Two keys. These are anvil's development accounts 1 and 0, which every anvil start funds with test ETH; they are public
knowledge, never use them on a public network:

```
printf '%s\n' 59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d > auditor.key
printf '%s\n' ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 > deployer.key
```

A registry whose approver is the Auditor (account 1, `0x70997970C51812dc3A010C7d01b50e0d17dc79C8`). It prints
`Registry deployed at 0x…`; that address is `<0x…>` below (on a fresh anvil it is
`0x5FbDB2315678afecb367f032d93F642f64180aa3`):

```
./att-auditor registry-deploy --rpc http://127.0.0.1:8545 --deployer-key-file deployer.key --approver 0x70997970C51812dc3A010C7d01b50e0d17dc79C8
```

The values the guest measured, then the audit: it asks the guest for a fresh quote, checks it equals those values, and
registers them.

```
gcloud compute scp attestamp-tdx-vendor:out/measurements.json measurements.json --zone us-central1-a
./att-auditor audit --tee-addr <guest-ip>:5000 --format intel-tdx --build-measurements measurements.json \
  --rpc http://127.0.0.1:8545 --registry <0x…> --signing-key-file auditor.key \
  --service hello --version 1.0.0 --arch amd64
```

It ends with `PASS: audited and registered`.

### 12. On your computer: verify, as any User

```
./att-user verify --tee-addr <guest-ip>:5000 --format intel-tdx --rpc http://127.0.0.1:8545 --registry <0x…>
```

Exit 0 and a `✓ matches  hello 1.0.0 (amd64) by 0x7099…` line: the guest runs what was registered. To see a refusal,
rebuild with one byte changed in `app/hello.txt`, launch it (steps 6 to 9) and verify again: exit 1, "nobody has
registered these measurements".

### 13. Put everything back, and stop

On the guest (`gcloud compute ssh …` again), one line at a time. Stop the responder first, on its own line:

```
sudo pkill -f "[a]tt-ra-responder ra-serve"
```

`pkill -f` matches whole command lines, its own shell's included. With the plain pattern
`"att-ra-responder ra-serve"`, sent together with other commands in one `gcloud compute ssh --command "…"`, it also
matched, and killed, the remote shell running them, so nothing after it ran (2026-11-15). `[a]tt` matches the text
`att` but not the text `[a]tt` in the command line that carries it, so the line above stops only the responder.

Then:

```
sudo rm -f /etc/grub.d/41_attestamp /etc/default/grub.d/99-attestamp.cfg /boot/initrd-attestamp.img
sudo cp -p /root/40-force-partuuid.cfg.orig /etc/default/grub.d/40-force-partuuid.cfg
sudo update-grub
cat /proc/sys/kernel/random/boot_id     # note it, as in step 7
sudo systemctl reboot
```

After that boot (log in again as in step 7), for Option 2 only: the image was still mounted on `/opt/app-verity`
before the reboot, so the folder can be removed only now:

```
sudo rmdir /opt/app-verity
```

`cat /proc/cmdline` shows the stock command line (`root=PARTUUID=… panic=-1`) and `ls /boot/initrd-attestamp.img`
finds nothing.
`base-initrd.img`, `base-initrd-v.img`, `app` and `out` in the home folder are yours to keep or delete. On your computer, stop anvil
(Ctrl-C) and the VM:

```
bash e2e/cloud/gcp-cvm.sh tdx stop        # the disk still costs a little; "delete" removes the VM and its disk
```

With `PREFIX=…` in step 3, give it here too (`PREFIX=<you>-tdx bash e2e/cloud/gcp-cvm.sh tdx stop`): `stop` acts only
on the VMs whose name starts with the prefix, default `attestamp-tdx`. To stop one VM by name:
`gcloud compute instances stop <name> --zone us-central1-a`.

The firewall rule `attestamp-ra` is left in place; `gcloud compute firewall-rules delete attestamp-ra` removes it.

### The Auditor's half: rebuild, boot, compare, register

> **Ran on 2026-11-16, both options, and passed** (`docs/results/sessions-2026-11-16/intel-tdx.md`). Two guests
> made in one call from `ubuntu-2404-noble-amd64-v20261105`; the Auditor's own build equalled the Vendor's, both guests
> measured the same four values, `audit --vendor-tee` registered the Auditor's own value, and the User's `verify` of
> the Vendor's guest against it gave exit 0. With one byte changed on the Vendor's side only, `audit` said `the
> Vendor's guest runs something else` and `verify` gave exit 1. One step was wrong and is fixed (A2's copy of `app`).
> Measured afterwards (T1, same day): another machine type changes RTMR0; another zone did not change anything (see "The
> same inputs").

This is what README's trust model asks of the Auditor: rebuild the Vendor's application independently, confirm that a
TEE really runs those bytes, and only then register. Step 11 above does not do that. It is the same flow as Nitro's,
moved to a confidential VM:

| | Nitro (`e2e/flow1-auditor-registers`) | TDX, below |
|---|---|---|
| rebuild | the Auditor builds the EIF on its own instance; the build gives all three PCRs | the Auditor builds the initrd on its own guest (A4); the build gives the initrd's hash, which GRUB measures into RTMR2 |
| boot it | the Auditor launches the EIF in its own enclave | the Auditor installs the initrd and reboots its own guest (A5) |
| compare | the attested PCRs must equal the built ones | the boot's event log must show GRUB loading exactly the built initrd (A6, `launch --check`), and its four values are recorded |
| register | `audit`: a fresh attestation of its own enclave equals its build, then register | `audit`: a fresh quote from its own guest equals A6's values, then register; `--vendor-tee` also asks the Vendor's guest (A8) |
| the Vendor | launches the Auditor's EIF | builds from the same inputs and boots (steps 4 to 9), or boots the Auditor's `out/initrd-app.img`, the same bytes |
| the User | verifies the Vendor's enclave against the Auditor's registry | the same, step 12 against the Auditor's registry (A9) |

**Why the Auditor needs a guest of its own.** The build predicts only the initrd. MRTD (the firmware), RTMR0 (its
configuration) and RTMR1 (the kernel) come from the machine, and RTMR2 also holds the kernel command line, which comes
from the image. The Auditor learns those by booting the same inputs on a guest it controls, and they equal the
Vendor's only when the inputs below are the same.

#### The same inputs

The Auditor's values equal the Vendor's only if all of these are the same on both guests (the same rows with a check
command for each: [reproduce.md](reproduce.md), "Intel TDX"). Measured on 2026-11-15
(`docs/results/trial-2026-11-16.md`): two guests made five days apart from `ubuntu-2404-noble-amd64-v20261105`,
`c3-standard-4`, `us-central1-a`, built the same `app/hello.txt` into the same initrd and measured the same MRTD,
RTMR0, RTMR1 and RTMR2; on 2026-11-16 a third guest of the same image gave the same four values again. That is the
condition the whole half rests on.

| input | how to make it the same | what changes if it differs |
|---|---|---|
| the image, **exact version** (`ubuntu-2404-noble-amd64-v20261105`), not the family `ubuntu-2404-lts-amd64` | the Vendor records the name step 3 printed; the Auditor creates with `IMAGE=<that name>` (A2). A family moves every few weeks | the kernel (RTMR1), the root file system UUID on the command line (RTMR2), the stock initrd |
| the kernel | do not `apt upgrade` either guest; `uname -r` must print the same on both | RTMR1, and the Option 1 base initrd |
| the machine type (`c3-standard-4`) | `MACHINE=` the Vendor's. **Required**: the Auditor's and the Vendor's guest must have the same machine type | **RTMR0** (measured 2026-11-16, T1). One guest, one boot entry, only the type changed: `c3-standard-4` RTMR0 `8847ec5d…`, `c3-standard-8` **`eba48352…`**; MRTD, RTMR1 and RTMR2 did not change. Whether the vCPU count or the memory moves it could not be separated: GCP refused `c3-highmem-4` and `c3-highcpu-8` for TDX, and in `c3-standard` both grow together. RTMR0 is where the firmware measures the VM's configuration (its ACPI tables and memory layout) |
| the zone (`us-central1-a`) | `ZONE=` the Vendor's | nothing, where measured: a TDX guest made in `us-central1-b` from the same image and type gave the same MRTD, RTMR0, RTMR1 and RTMR2 as `us-central1-a` (2026-11-16, T1; measured on TDX itself, not taken from SEV-SNP). That is one other zone on one day, not every zone for good, so keep the Vendor's zone; it costs nothing |
| the base initrd | Option 1: the image's own `/boot/initrd.img-<kernel>`, the same file on every guest of one image (`4703ec07…` on all three). Option 2: **the Vendor's** `base-initrd-v.img`; `mkinitramfs` never makes the same file twice, so the Auditor must not make its own | the initrd hash, so RTMR2 |
| the application folder | the same files byte for byte (and the same modes; `build` normalises them) | the initrd hash, so RTMR2 |
| the option (1 or 2) | the Vendor says which | the initrd, and for Option 2 the image |
| this repository's commit | the same commit, built as in step 2 | the archive `build` writes (the cpio format is pinned by tests, but only the same code is sure to give the same bytes) |

What the Vendor hands the Auditor, by any channel: the image name, the machine type and zone, the option, this repository's commit (`git rev-parse HEAD` on the Vendor's computer), the `app` folder (or the commit of the source it comes from), its `out/build-value.json`, and for Option 2 its
`base-initrd-v.img`. From the Vendor's guest to the Vendor's computer:

```
gcloud compute scp --recurse attestamp-tdx-vendor:app      vendor-app        --zone us-central1-a
gcloud compute scp attestamp-tdx-vendor:out/build-value.json vendor-build-value.json --zone us-central1-a
gcloud compute scp attestamp-tdx-vendor:base-initrd-v.img vendor-base-initrd-v.img --zone us-central1-a   # Option 2
```

#### The steps

The order with both halves: the Vendor's steps 1 to 10, then A1 to A9 instead of step 11, then step 12, then step 13 on
both guests. A1 to A9 are the Auditor's, on the Auditor's computer and its own guest.

**A1. On the Auditor's computer: tools and binaries.** Step 1 as written, then `git checkout <the Vendor's commit>` in the clone, so that both sides build the same code. Step 2, with
`att-auditor` for the guest instead of `att-vendor`:

```
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-auditor ./cmd/auditor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-user    ./cmd/user
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o linux/att-auditor      ./cmd/auditor
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o linux/att-ra-responder ./cmd/ra-responder
```

**A2. On the Auditor's computer: its own guest, from the Vendor's exact image.**

```
IMAGE=<the Vendor's image name> MACHINE=c3-standard-4 ZONE=us-central1-a bash e2e/cloud/gcp-cvm.sh tdx create auditor --yes
bash e2e/cloud/gcp-cvm.sh tdx status
```

`IMAGE`, `MACHINE` and `ZONE` are the Vendor's ("The same inputs"); the guest is `attestamp-tdx-auditor` (with `PREFIX=` as in "Names", its own prefix). `IMAGE` is the exact name the Vendor's step 3 printed (`ubuntu-2404-noble-amd64-v20261105` is ours, an example). If you give the Auditor's guest its own `PREFIX=`, run `status`, `stop` and step 13's commands once per prefix: each acts only on the VMs of its own prefix, so one call shows or stops one guest. And if the Auditor's computer is not the one that first ran `gcp-cvm.sh … create` in this project, `attestamp-ra` does not let it reach port 5000 yet: add its address as `e2e/cloud/README.md`, "From another computer, or after your address changed", shows (one `/32`), or A8 times out.
`status` lists each disk's image: both must name the same version. Its address is `<auditor-guest-ip>` below. Then
copy what the build needs, one item per command:

```
gcloud compute scp linux/att-auditor       attestamp-tdx-auditor:att-auditor       --zone us-central1-a
gcloud compute scp linux/att-ra-responder  attestamp-tdx-auditor:att-ra-responder  --zone us-central1-a
gcloud compute scp --recurse vendor-app    attestamp-tdx-auditor:              --zone us-central1-a
gcloud compute scp vendor-base-initrd-v.img attestamp-tdx-auditor:base-initrd-v.img --zone us-central1-a   # Option 2
```

and log in: `gcloud compute ssh attestamp-tdx-auditor --zone us-central1-a`. The folder arrives as `vendor-app` in
the home folder; on the guest, `rm -rf app && mv vendor-app app`. (Copying straight to `…:app` fails with Windows'
`pscp` when `app` does not exist yet, `unable to open app`, and leaves an **empty** `app` behind; a build of it gives
another initrd, which A4's comparison catches. Seen 2026-11-16.)

**A3. On the Auditor's guest: prepare it.** Step 4 as written, with `chmod +x att-auditor att-ra-responder`, and with
two differences. Option 1: `base-initrd.img` is copied from `/boot` as written; its SHA-256 must equal
`base_initrd_sha256` in the Vendor's `build-value.json` (`sha256sum base-initrd.img`). Option 2: `sudo apt-get update`, install the packages
(`squashfs-tools cryptsetup-bin`; their versions must equal the Vendor's, `dpkg-query -W squashfs-tools cryptsetup-bin`
on both, see [reproduce.md](reproduce.md)) and make `/opt/app-verity`, but **do not** run the hook and `mkinitramfs` lines: use
the Vendor's `base-initrd-v.img`, copied in A2. Check `uname -r` against the Vendor's.

**A4. On the Auditor's guest: build.** The Auditor's own build of the Vendor's application:

```
./att-auditor build --platform intel-tdx --app-dir app --base-initrd base-initrd.img --option 1 --verify
```

(Option 2: `sudo ./att-auditor build --platform intel-tdx --app-dir app --base-initrd base-initrd-v.img --option 2
--verify`.) Its `out/build-value.json` is the Auditor's expected value. `initrd_sha384` must equal the Vendor's (and
for Option 2 `root_hash` too): the same files on the same base give the same initrd. If they differ, the inputs differ;
stop and find which.

**A5. On the Auditor's guest: install and boot.** Step 7 with `att-auditor`:

```
cat /proc/sys/kernel/random/boot_id
sudo ./att-auditor launch --platform intel-tdx --initrd out/initrd-app.img --reboot
```

Wait for the new boot as step 7 says, log in again, and `sudo shutdown -h +240`.

**A6. On the Auditor's guest: compare the boot with the build.**

```
sudo ./att-auditor launch --platform intel-tdx --check
```

This is the comparison: it verifies a fresh quote, replays the event log against it and requires GRUB to have loaded
exactly the initrd A4 built. It ends with `✓ the boot measured the built initrd` and writes the four values to
`out/measurements.json`. If it refuses, the Auditor registers nothing. Those four values should equal the Vendor's
step 8 values; whether they do is checked against the Vendor's running guest in A8, not by trusting a file the Vendor
sent.

**A7. On the Auditor's guest: answer requests.** Step 9 as written. Log out.

**A8. On the Auditor's computer: register only if they match.** anvil, the keys and `registry-deploy` as in step 11.
Then, with the Auditor's own values:

```
gcloud compute scp attestamp-tdx-auditor:out/measurements.json auditor-measurements.json --zone us-central1-a
./att-auditor audit --tee-addr <auditor-guest-ip>:5000 --format intel-tdx --build-measurements auditor-measurements.json \
  --vendor-tee <guest-ip>:5000 \
  --rpc http://127.0.0.1:8545 --registry <0x…> --signing-key-file auditor.key \
  --service hello --version 1.0.0 --arch amd64
```

`audit` asks the Auditor's own guest for a fresh quote and registers only if it equals `auditor-measurements.json`
(`identical: the guest runs exactly what I built`); otherwise it stops with `not registering anything`. Without
`--build-measurements` it refuses to start at all, on every platform (since 2026-11-16): it would register whatever the guest reports,
an initrdless boot included (`docs/results/sessions-2026-11-16/tdx-initrdless.md`). That refusal does not stop an
Auditor who registers such values by hand with `register`; the trust model has to assume the Auditor does not. With
`--vendor-tee` it also asks the Vendor's guest and prints `same: the Vendor's guest runs exactly what I built` or
`the Vendor's guest runs something else`. In the second case it **still registers the Auditor's own value and exits
12**, saying that the registration succeeded and only the Vendor comparison failed; do not run it again (that ends
with exit 5, already registered). A User verifying the Vendor's guest will then be refused (A9), which is
correct.

**A9. On the Auditor's computer (or any computer that `attestamp-ra` lets in and that reaches the chain; with the local anvil of step 11 that is the Auditor's): the User verifies the Vendor's guest.** Step 12 against the registry of A8:

```
./att-user verify --tee-addr <guest-ip>:5000 --format intel-tdx --rpc http://127.0.0.1:8545 --registry <0x…>
```

Exit 0 and `✓ matches  hello 1.0.0 (amd64) by 0x7099…` now mean what README promises: the Vendor's guest runs what an
Auditor rebuilt and booted itself. Then step 13 on both guests (`attestamp-tdx-auditor` too) and
`bash e2e/cloud/gcp-cvm.sh tdx stop`, which stops both if they share a prefix (once per prefix otherwise).

### Use the same image as the other side

The Vendor's and the Auditor's guests must be made from the **same exact image version**, for example
`ubuntu-2404-noble-amd64-v20261105`, not merely the same family (`ubuntu-2404-lts-amd64`). Why: the kernel command line
GRUB measures into RTMR2 names the root file system by its UUID (`root=UUID=9d4d258d-…`), and the Option 1 base initrd
is the image's own file. Both come **with the image**, not with the disk: every disk made from one image version
carries the same file system UUID, the same partition UUID and the same `/boot` files. Measured on 2026-11-15
(`docs/results/trial-2026-11-16.md`): `attestamp-tdx-vendor`, made 2026-11-10, and `attestamp-tdx-auditor`, made
2026-11-15, both from `v20261105`, had the same UUIDs, the same base initrd `4703ec07…`, built the same initrd
`bc024887…` from `app/hello.txt` and measured the same MRTD, RTMR0, RTMR1 and RTMR2; MRTD, RTMR0 and RTMR1 also equal the
2026-11-11 recordings.

A family is a pointer that Canonical moves to a new version every few weeks. `gcp-cvm.sh` resolves it once, prints the
exact name and takes `IMAGE=<name>` to make the other side from the same one.

If the two images differ, expect:

| what differs | what changes |
|---|---|
| another version of the same family | a new root file system UUID, so the command line and **RTMR2**; a new stock initrd, so the Option 1 base and the initrd hash; often a new kernel, so **RTMR1** too |
| another kernel on the same image (an `apt upgrade` on one side) | **RTMR1** (the kernel), the base initrd (`/boot/initrd.img-<version>`), so **RTMR2** |
| another machine type (`c3-standard-4` vs `c3-standard-8`) | **RTMR0**, measured 2026-11-16 (`8847ec5d…` vs `eba48352…`); MRTD, RTMR1 and RTMR2 unchanged |
| another zone (`us-central1-a` vs `us-central1-b`) | nothing, measured 2026-11-16 with one TDX guest in each |
| another firmware (GCP updates TDVF) | **MRTD** and RTMR0 |

The values then differ and `audit` refuses, correctly: it is another boot. Both sides move to a new image together
and register again. An old image version can also be deleted by Canonical some time after it is deprecated;
`gcp-cvm.sh` then says it was not found.

## The quote

One request returns one signed document, like Nitro: a TDX quote (version 4 on GCP, confirmed). It is the TD report
signed by an attestation key; Intel's Quoting Enclave vouches for that key with a report signed by the platform's PCK
certificate, and the PCK certificate chains to the Intel SGX Root CA. The PCK chain is inside the quote.

`ra-parse` checks, in order:

1. The quote decodes as version 4 with TEE type `0x81` (TDX).
2. The PCK certificate chains, through the intermediate carried in the quote, to the Intel SGX Root CA embedded as
   `internal/tee/parse/tdx/root.pem`. The root at the end of the quote's own chain is never trusted.
3. go-tdx-guest checks the quote signature, the Quoting Enclave report signature and the hash that binds the
   attestation key to that report.
4. REPORTDATA carries the nonce sent.
5. TDATTRIBUTES.DEBUG (bit 0) is clear. A debug TD is refused.

Intel's collateral (TCB info, QE identity) and revocation lists come from Intel's PCS over the network and are **not**
fetched, so the check runs offline like the Nitro one. The cost: the platform's TCB level is not judged, and a quote
from a machine with an out-of-date TDX module still verifies.

The root certificate is the same one go-tdx-guest embeds and the one inside our real quote (sha256 fingerprint
`44:A0:19:6B:…:74:D3`, valid 2018-05-21 → 2049-12-31, confirmed). **It is also the one Intel publishes** (checked
2026-11-12): `https://certificates.trustedservices.intel.com/Intel_SGX_Provisioning_Certification_RootCA.cer` gives the
same subject, serial `22650CD65A9D3489F383B49552BF501B392706AC`, validity and full sha256 fingerprint
`44:A0:19:6B:2B:99:F8:89:B8:E1:49:E9:5B:80:7A:35:0E:74:24:96:43:99:E8:85:A7:CB:B8:CC:FA:B6:74:D3`
(`openssl x509 -inform DER -noout -fingerprint -sha256` on the download, and on `root.pem` without `-inform DER`).

## The values

| name | what it covers |
|---|---|
| MRTD | the TD's initial contents: the virtual firmware (TDVF) GCP launches |
| RTMR0 | the firmware's configuration |
| RTMR1 | the boot loader and kernel |
| RTMR2 | the kernel command line and the initrd, where the application archive is |

The firmware extends what a TPM would put in PCR1 and 7 into RTMR0, PCR2–6 into RTMR1 and PCR8–15 into RTMR2. That
mapping is the TDVF design.

Measured on four recordings from `attestamp-tdx-vendor` (Option 1 v1/v2 and Option 2 v1/v2, 2026-11-11): MRTD,
RTMR0 and RTMR1 were identical across all four, and only RTMR2 changed with the application. This confirms the
assumed mapping of GRUB's measurements into RTMR2, and it is also why TDX needs no second format for Option 2: both
options move the same single register. (`internal/tee/parse/tdx/app_fixture_test.go`. The rest of the mapping, which
of RTMR0 and RTMR1 holds what, was not tested: nothing in those registers was changed.)

Why all four, not RTMR2 alone: the firmware, boot loader and kernel extend RTMR2 themselves, so a changed kernel could
extend the expected RTMR2 while running something else, and only MRTD and RTMR0–1 would show it. Nitro registers PCR0,
1 and 2 together for the same reason.

**"RTMR[2] alone" in the 2026-10-23 slides is a contrast, not a registration rule** (decided in the project,
2026-11-11). The slides say one value moves with the application on TDX (RTMR2) where SEV-SNP Option 2 moves two
(CMDLINE and PCR12). That is what the recordings show. It does not mean registering RTMR2 alone: that would open the
attack in the previous paragraph, because whoever changes the kernel can make RTMR2 come out as expected while running
something else, and only MRTD and RTMR0–1 would reveal it. `intel-tdx` registers all four values, MRTD, RTMR0, RTMR1
and RTMR2, and the name list does not change.

### Option 1 and Option 2

TDX, like SEV-SNP, has two ways to carry the application (the 2026-10-23 slides, slide 16):

- **Option 1:** the application is a cpio archive in the initrd (`internal/setup/build/cpio`).
- **Option 2:** the application is a read-only squashfs image with a dm-verity hash tree, and that image is placed
  inside the initrd rather than on a separate disk. RTMR2 already covers the whole initrd, so the image is measured
  with it and nothing is extended at run time.

Consequence: TDX needs no second format. `intel-tdx` carries both options with the same measurement names (MRTD,
RTMR0, RTMR1, RTMR2). SEV-SNP splits into `amd-sev-snp-initrd` and `amd-sev-snp-verity` because its image sits on
disk, so a boot-time hook extends vTPM PCR12 and a sixth value appears (`docs/build/sev-snp.md`, "Decision: one CBOR
container, two format strings"; `docs/build/ra-comparison.md`).

ext4 is not reproducible even with a fixed UUID and `hash_seed`, so the image is squashfs with `-mkfs-time 0
-all-time 0`, file times set to 0, and dm-verity with a fixed `--salt` and `--uuid` (`docs/build/sev-snp.md`,
"Variables").

Option 2 on TDX was booted on 2026-11-12. A script in the initrd
(`internal/tee/parse/tdx/testdata/record/initramfs-local-bottom.sh`) verifies the image against the root hash that
travels in the initrd. It then opens the image with dm-verity and mounts it read-only on `/opt/app-verity`.
- Results for v1 and v2: `dmsetup status` `V`, and the application could be read.
- Only RTMR2 differed between the two boots.
- An image that did not match its root hash was not opened. The boot still continued, because the script only records
  `fail`.

The fixtures are `gcp-c3-opt2m-*`, recorded with the procedure in `docs/build/app-fixture-recording.md`, "TDX Option 2
mounted at boot", which also has the results of the four boots, the tampered one included.
To make the Option 2 base initrd with the hooks for a real build, follow "Getting started", step 4; the hook scripts
are in `internal/tee/parse/tdx/testdata/record/`, and `build --option 2` makes the image itself.

Left out, because they describe the machine or whoever launched it, not the image:

| field | why |
|---|---|
| MROWNER, MROWNERCONFIG, MRCONFIGID | set by whoever launches the TD; MROWNER is non-zero in our GCP quote (confirmed). That it differs per instance is the project's working assumption, not measured |
| RTMR3 | extended at run time by the guest; zero in our quote (confirmed) |
| MRSEAM, MRSIGNERSEAM | the TDX module, i.e. the platform |

## Nonce and time

REPORTDATA is 64 bytes and a `tee/request` nonce is 32. `CheckNonce` requires REPORTDATA to be the nonce followed by
zeros; a 64-byte nonce fills all of it. The TDX request side, when written, must put the nonce first and zeros after.

A TDX quote has **no timestamp** (confirmed: the quote format has no time field). The PCK chain is therefore judged at
verification time. That costs little: the PCK leaf in our quote is valid 2026-10-15 → 2033-10-15 (confirmed), not the
three hours of a Nitro leaf. Freshness comes from the nonce.

## Variables

Two rows below were measured on TDX by changing only that variable, both on 2026-11-16: the VM shape (T1) and the
application archive's Go version (T2, `docs/results/sessions-2026-11-16/intel-tdx.md`). The other rows are what the
structure above implies, or what the SEV-SNP runs measured for the same files. Also measured is the opposite case: two
guests of one image, made five days apart, gave the same four values (see "Use the same image as the other side").

| variable | which value | note |
|---|---|---|
| GCP's TDVF version | MRTD | GCP may update it; every registration would change |
| VM shape (vCPUs, memory) | RTMR0 | **measured 2026-11-16**: `c3-standard-4` `8847ec5d…`, `c3-standard-8` `eba48352…`, nothing else moved. Which of vCPUs and memory does it was not separable (GCP allows TDX on `c3-standard` shapes only, where both grow together) |
| shim, GRUB, kernel | RTMR1 | OS image updates |
| kernel command line | RTMR2 | it carries the root file system's UUID (`root=UUID=…` with our entry). That UUID comes with the image version, not the disk: two guests of one image had the same (measured 2026-11-15), so it changes only with another image |
| base initrd | RTMR2 | `mkinitramfs` writes a random seed, so the Vendor's base initrd is used as published (as on SEV-SNP) |
| application archive | RTMR2 | Go version, `-trimpath`, the cpio writer: the same as on SEV-SNP (`docs/build/sev-snp.md`). The archive is written by `internal/setup/build/cpio`; its output is pinned by the golden files in `internal/setup/build/cpio/testdata` |
| archive placement | RTMR2 | the application archive goes **before** the base initrd (`cpio.AppFirst`), the placement the TDX fixtures were recorded with. An unpadded append failed in 2026-10 (the 2026-10 record, not repeated since); the cause is 4-byte alignment, measured on SEV-SNP on 2026-11-11. See "Before or after the base initrd" below |
| `GRUB_FORCE_PARTUUID` | RTMR2 | GCP's Ubuntu 24.04 image boots its stock entry without an initrd when this is set (confirmed on the SEV-SNP image of the same family). An application in an initrd is then never loaded. See "`GRUB_FORCE_PARTUUID`" below |

## Byte order

Every integer in the quote is read little endian, as Intel's quote specification requires; the signatures and the
attestation key are big endian. Sources: Intel "TDX DCAP Quote Generation Library and Quote Verification Library"
(`Intel_TDX_DCAP_Quoting_Library_API.pdf`, rev. 0.9x), Appendix A.3. Offsets are into the raw quote (header 0x00–0x2F,
TD quote body from 0x30). "Pinned" means a test fails if the order were the other one.

| field | offset, size | order | read by | spec | pinned by |
|---|---|---|---|---|---|
| header version | 0x000, u16 | LE | go-tdx-guest `abi` | A.3 "Endianess: Little Endian (applies to all integer fields)"; A.3.1 "Value: 4" | `TestEndian_HeaderIsLittleEndian`; every fixture test (the library refuses other versions) |
| attestation key type | 0x002, u16 | LE | `abi` | A.3.1: 2 = ECDSA-256-with-P-256 | same |
| TEE type | 0x004, u32 | LE | `abi`, compared in `tdx/doc.go` | A.3.1: `0x00000081` = TDX | same |
| TDATTRIBUTES | 0x0A8, 8 bytes | bit 0 in the first byte (LE numbering) | `tdx/measure.go` (`attr[0]`) | A.3.2 types it a byte array; A.3.4 numbers its bits (0 DEBUG, 28 SEPT_VE_DISABLE, 62:32 reserved) but **does not say which byte holds bit 0** | `TestEndian_TDAttributesBitNumbering` (read LE the fixture sets only bit 28; read BE it would set reserved bit 36), `TestEndian_DebugBitIsNotInLastByte`, `TestParse_DebugTDIsRefused` |
| signed data size | 0x278, u32 | LE | `abi` | A.3 rule | every fixture test |
| quote signature r‖s | 0x27C, 2×32 | BE | `abi` (`SignatureToDER`) | A.3.5 "r component followed by the s component"; byte order inside r and s not stated | `TestVerifySig_RealQuoteWithEmbeddedRoot` (BE is what verifies) |
| attestation key x‖y | 0x2BC, 2×32 | BE | go-tdx-guest `verify` | A.3.6; order inside not stated | same |
| certification data type, size | 0x2FC u16, 0x2FE u32 | LE | `abi` | A.3.8–A.3.9 | every fixture test |
| QE report MISCSELECT, ISVPRODID, ISVSVN | 0x312 u32, 0x402 u16, 0x404 u16 | LE | `abi` | A.3.10 | **not pinned**: no check judges these values |
| QE report signature | 0x482, 2×32 | BE | `abi` | A.3.5 | signature test |
| QE auth data size; PCK chain type, size | 0x4C2 u16; 0x4E4 u16, 0x4E6 u32 | LE | `abi` | A.3.7, A.3.9 | every fixture test |

Two things to know:

- go-tdx-guest verifies the signatures over bytes **rebuilt from the parsed fields**, not over the bytes received. A
  field read in the wrong order and written back the same way would still verify, so a passing signature says nothing
  about byte order. Only fields whose values are checked (version, key type, TEE type, the sizes) are pinned by the
  library; TDATTRIBUTES is pinned by our own tests.
- The TDATTRIBUTES bit numbering is pinned by the fixture, not by the DCAP document. Intel's TDX Module ABI
  specification would be the authority; it was not retrieved.

## `GRUB_FORCE_PARTUUID`

(Every setting this project changes, with what breaks if it differs, is collected in
[settings-and-hashes.md](settings-and-hashes.md); this section is the detail for this one.)

Researched 2026-11-11. What is **confirmed** has a source; the rest is marked.

**What it is (confirmed).**

- Ubuntu's cloud-image build (`livecd-rootfs`, `live-build/functions`) sets it for images with a non-generic kernel,
  such as `linux-gcp`.
- At image build time the function `force_boot_without_initramfs` writes the root partition's PARTUUID, as a literal
  value, into `/etc/default/grub.d/40-force-partuuid.cfg`. Our instance has exactly that file (2026-11-11).
- The GRUB side is Ubuntu's own patches to `grub2`: `ubuntu-support-initrd-less-boot.patch` and
  `ubuntu-add-initrd-less-boot-fallback.patch`.
- Purpose: boot time ("no IO involved to lift the initrd… particularly relevant to cloud images", Launchpad bug
  1754586).
- The kernel can find `root=PARTUUID=` without an initramfs, but not a file-system `UUID=`
  (`block/early-lookup.c`). That is why turning the setting off switches the command line to `root=UUID=`, as we saw.

**The fallback (confirmed, Launchpad bug 1870189).**

- Each stock entry records `initrdfail=1` in `grubenv` before booting.
- `grub-initrd-fallback.service` clears it once local file systems are up.
- If the initrdless boot panics first (`panic=-1` reboots at once), the next boot uses the initrd.
- Every later boot tries initrdless again: "on each fresh reboot it again attempts an initramfsless boot before
  falling back".

**The two concerns raised.**

- **Migration: partly valid, for a narrower reason (inferred).** GCP Confidential VMs cannot live-migrate; every
  maintenance event is a stop and start (GCP's Confidential VM docs, confirmed). Cloning a disk copies both the
  partition GUID and the file-system UUID, so neither is more unique.
  - The real risk is that the PARTUUID in the config file is a fixed value that `update-grub` never refreshes. If the
    root file system lands on a partition with a new GUID (re-partitioning, file-level migration), both the
    initrdless entry and its fallback point at a partition that no longer exists, and the VM does not boot.
  - Moving to a machine type whose disk driver is a module rather than built in costs one panic and reboot, then the
    fallback recovers.
- **"Docker might not build": not supported by what we found (inferred).** Docker's kernel needs (overlay,
  br_netfilter, nf_tables, veth, cgroup v2) are loaded from `/lib/modules` on the root file system after boot,
  whether or not an initramfs was used. It would matter only if the application ran from the initramfs and never
  switched to the real root; that is a design choice, not this setting. Not tested.

**For us: off.** An application placed in the initrd needs the initrd loaded on every boot.

| | leave it ON | turn it OFF (what we did on `attestamp-snp-vendor`, see `docs/build/sev-snp.md`) |
|---|---|---|
| initrd loaded | never by the stock entries, so the application is absent and no initrd is measured | by every entry |
| boot | faster (amount not found) | slightly slower |
| root device | a fixed PARTUUID written at image build time | `root=UUID=`, regenerated by `update-grub` |
| missing disk driver | one panic and reboot, then the fallback | handled by the initramfs |
| custom one-shot entry (`grub-reboot`) | a stale `initrdfail=1` can redirect it (Ubuntu's `00_header` code, inferred from the patch) | no fallback in the way |
| still needed | a custom entry made the default | a custom entry and initrd file of our own; the stock initrd is the distribution's, regenerated on kernel updates |

Not found: a measured boot-time saving; the list of drivers built into `linux-gcp`.

## Before or after the base initrd

Researched 2026-11-11. The question: is "appending the application archive fails" a kernel limitation, or
something about how the file was made?

**What the kernel does (confirmed).** Sources: mainline `init/initramfs.c`, read on GitHub master on 2026-11-11 and at
commit `067c5c24cef2`, and `Documentation/driver-api/early-userspace/buffer-format.rst`.

- `unpack_to_rootfs` walks the initrd as a sequence of segments.
- An uncompressed cpio segment is recognised only when `*buf == '0' && !(this_header & 3)`: its first byte is `0` and
  its offset from the start of the whole initrd is a multiple of 4. The buffer-format document states the same rule
  (`cpio_file := ALGN(4) + …`).
- NUL bytes between segments are skipped one at a time.
- After a compressed segment the offset moves to `saved_offset + my_inptr`, the number of bytes the decompressor
  consumed. zstd reports one frame's length, so data after a zstd segment is supported.
- A segment that is neither an aligned `0` nor a known compression magic ends with `invalid magic at start of
  compressed archive`.
- The kernel logs `Initramfs unpacking failed: <reason>` and does not stop. Files already unpacked, the whole base
  initramfs included, stay in place.

**What the files look like (confirmed).**

- Ubuntu 24.04's `mkinitramfs` writes the early cpios, then `cpio | zstd`, with no padding at the end, so the base
  initrd's length mod 4 is arbitrary.
- The scripts' `cat base app.cpio` adds no padding either, and neither did `cpio.Join` until 2026-11-11 (it now pads
  the base for `AppLast`, commit `7e47556`).
- GRUB zero-pads each file to 4 bytes only when it loads several (`initrd a b`, `grub-core/loader/linux.c`). One
  combined file gets no padding.

**Measured 2026-11-11.** The same base initrd file (50,100,283 bytes, size mod 4 = 3, sha256 `4703ec07…b0a2`) was
used on both guests:

| guest, placement | kernel | marker `/dev/attestamp-app-marker` |
|---|---|---|
| SEV-SNP, `AppLast`, unpadded | logged `Initramfs unpacking failed: invalid magic at start of compressed archive` (both boots) | absent: the application never ran |
| SEV-SNP, `AppLast`, padded (base zero-padded to 50,100,284 bytes) | no error (both boots) | present, with the application's text |
| Intel TDX, `AppFirst` | no error on any of four boots | present on all four |

Placement, not platform, decides the outcome: `AppFirst` works because the application archive is padded to 512
bytes, so the base initrd that follows starts on a 4-byte boundary.

After the padding fix in `cpio.Join` (commit `7e47556`), placement is a convention rather than a correctness
requirement: both orders work. The order still changes the measured bytes, so each platform keeps the placement its
fixtures were recorded with: `AppFirst` on TDX, `AppLast` on SEV-SNP.

In the unpadded case the guest booted and INITRD still changed with the application (`docs/build/sev-snp.md`,
"Correction: SEV-SNP Option 1 as appended in 2026-11"). **A changed or matching measurement does not by itself prove
the application ran**: only the marker, or the boot log, does. The boot logs are committed next to the fixtures
(`*-boot.txt`).

**TDX with `AppLast` was not tested on 2026-11-11.** The statement that an append failed to decompress on TDX comes
from the record of a 2026-10 session, not from this one. That base initrd and its serial
log were not kept. If its length was not a multiple of 4, the measurement above explains it; and an unpadded append
lets the guest boot without the application, so "failed" there may mean the application was missing rather than that
the boot stopped.

## The instance

`e2e/cloud/gcp-cvm.sh tdx create <roles…> --yes` creates it.

| setting | value | why |
|---|---|---|
| machine type | `c3-standard-4` | TDX on GCP runs on C3 (Sapphire Rapids) or C4 (script header, checked against Google's docs 2026-11) |
| confidential type | `--confidential-compute-type=TDX` | |
| CPU platform | none | the script header says TDX must be created **without** `--min-cpu-platform` |
| maintenance | `--maintenance-policy=TERMINATE` | these VMs cannot live-migrate |
| image | `ubuntu-os-cloud/ubuntu-2404-lts-amd64` | must carry `TDX_CAPABLE`; the script checks it |
| zone | `us-central1-a` (default) | |

Our recorded guest, `attestamp-tdx-vendor` (read with `gcloud compute instances describe` and `gcloud compute disks
describe` on 2026-11-11):

| | |
|---|---|
| image | `ubuntu-os-cloud/global/images/ubuntu-2404-noble-amd64-v20261105` (the family moves; this is the exact image) |
| boot disk | 64 GB, `pd-balanced` |
| Shielded VM | Secure Boot **off**, vTPM on, integrity monitoring on |
| maintenance | `TERMINATE` |
| OS, kernel | Ubuntu 24.04.5, `7.0.0-1017-gcp` |
| quote interface | ConfigFS TSM, provider `tdx_guest` |

## From scratch: record and check one quote

To build and run an application, follow [Getting started](#getting-started-from-nothing-to-a-verified-tdx-guest)
instead. This section only records a quote from a stock guest, as the first fixture was made.

1. `bash e2e/cloud/gcp-cvm.sh tdx create vendor --yes`, then on the instance `sudo shutdown -h +240` as a safety stop.
2. Record a quote with ConfigFS TSM, as in `internal/tee/parse/tdx/testdata/README.md`.
3. Copy `quote.bin` back (`gcloud compute scp`, one file per call) and stop the instance.
4. `att-user ra-parse --doc quote.bin --format intel-tdx --nonce <128 hex chars>` prints MRTD and RTMR0–2.

## Not done yet

| what | why |
|---|---|
| a failed dm-verity check at boot stopping the boot | Option 2 builds and boots with the real tools (2026-11-15), but a root hash that does not match still lets the boot go on; `launch --check` refuses it (`/run/attestamp-status`) |
| TCB level, CRLs | collateral is not fetched (see above) |
| quote version 5 | the go-tdx-guest revision in `go.mod` decodes version 4 only; version 5 needs a newer one and its own fixture |
| the RTMR0/RTMR1 part of the mapping, and the VM-shape dependence | not tested; only RTMR2 was (it moves with the initrd) |
| `AppLast` on TDX | not tested on 2026-11-11; the 2026-10 failure is the only record (`docs/status/build-info-gaps.md`, Still missing) |
