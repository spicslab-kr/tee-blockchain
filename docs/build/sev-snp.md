# AMD SEV-SNP: what is measured, what changes it, and what is not done yet

**Status.** The verifying side works on recorded evidence: `ra-parse` checks a GCP SEV-SNP evidence pair and
extracts its values (`internal/tee/parse/snp`, formats `amd-sev-snp-initrd` and `amd-sev-snp-verity`). Requesting
evidence from a running guest works (`att-ra-responder ra-serve --extractor snp-initrd | snp-verity`; the whole chain
passed on a live guest with `snp-initrd` on 2026-11-12). Building and launching (`build` and `launch --platform
amd-sev-snp-initrd | amd-sev-snp-verity`) **ran on real guests on 2026-11-15**: install, reboot and `launch --check`
passed for both options, Option 2's PCR12 was the predicted value, and `audit` and `verify` passed from another
machine (`docs/plan/trial-2026-11-16.md`). In those runs the Auditor registered the Vendor's guest's own values.
**The independent Auditor ran on 2026-11-16** ("The Auditor's half", two guests, both formats) and passed, and the
**relay (T10) was reproduced** the same day: see "Same VM" (`docs/results/sessions-2026-11-16/amd-sev-snp.md`). Tools and versions: [toolchain.md](toolchain.md). A known weakness is
written down below under "Same VM": SEV-SNP support is weaker here than Nitro or TDX, and that does not go away
with more code.

Every statement below is marked by where it comes from: **confirmed** (read from our recorded evidence, a spec, a
library's source or an official page, named next to it), **observed** (seen in our own runs, not proved in general)
or **not confirmed**.

## Getting started: from nothing to a verified SEV-SNP guest

This takes one SEV-SNP guest on Google Cloud and your own computer, and goes all the way: build what carries the
application on the guest, boot it, read the measurements, register them on a chain, and have a User verify the guest.
Type the commands in order. `<guest-ip>` and `<0x…>` are values an earlier step prints; nothing else needs filling in.

**What steps 1 to 13 show, and what they do not.** They run against **one** guest, and in step 11 the Auditor
registers the values the **Vendor's** guest measured, without rebuilding anything. That is **not** the trust model
README describes ("an independent Auditor rebuilds it, confirms the enclave really runs those bytes"): a registration
made that way says no more than the Vendor's own word. It shows that the chain works end to end. The Auditor's
independent rebuild is "The Auditor's half" below, steps A1 to A9, which replace step 11; it needs two guests at once
and **ran and passed on 2026-11-16**, both formats. The procedure was followed end to
end in a clean environment on 2026-11-15 (`docs/plan/trial-2026-11-16.md`), and again from the documents alone, both
options, on 2026-11-16 (`docs/plan/trial-2026-11-16-clean.md`); the places where either needed knowledge from outside
this page have been filled in since. If a step stops with an error, that is a finding; note it. And on
SEV-SNP the evidence cannot prove that its two documents come from the same VM ([Same VM](#same-vm)); the procedure
does not change that.

**Names.** The commands use the guest name `attestamp-snp-vendor` and the zone `us-central1-a`. If your guest has
another name (you made `create auditor`, or gave `PREFIX=…` to `gcp-cvm.sh`), replace the name in every `gcloud`
command; `bash e2e/cloud/gcp-cvm.sh snp status` lists the names. If somebody else in the same project already has an
`attestamp-snp-vendor`, `create` says `already exists, reusing it (not created)` and makes nothing: choose your own
names with `PREFIX=<you>-snp` (the guest is then `<you>-snp-vendor`), and pass the same `PREFIX=` to `status` and
`stop`, which act only on VMs whose name starts with it.

**Cost.** The guest is an `n2d-standard-2`, billed by the hour while it runs. Step 13 stops it.

### Which option

| | Option 1, `amd-sev-snp-initrd` | Option 2, `amd-sev-snp-verity` |
|---|---|---|
| where the application is | its files, as an archive appended to the initrd | a read-only squashfs image with a dm-verity hash tree, on the disk in `/var/lib/attestamp` |
| what proves it | INITRD (SHA-256 of the initrd GRUB loaded) | INITRD (a base initrd with our hook), CMDLINE (it carries the image's root hash) and PCR12 (the hook extends it with the result of checking the image) |
| after the boot | the files are unpacked into the initramfs; the boot then switches to the disk's root and the initramfs is emptied (that is what initramfs-tools does; not checked on our guest), so **nothing runs from them afterwards** | mounted read-only on `/opt/app-verity`; every block is checked against the root hash when it is read |
| values registered | 5: MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD | 6: the same and PCR12 |
| extra work | none | a base initrd built with our two hook scripts and `tpm2-tools` (step 4) |
| choose it | to try the whole flow, or for small data only the boot needs | for an application that has to be there while the guest runs |

The option is chosen by the format string; every command below takes it with `--platform` or `--format`. Write
`amd-sev-snp-initrd` for Option 1 and `amd-sev-snp-verity` for Option 2, the same one everywhere.

### 1. On your computer: the tools and the source

The same as [TDX step 1](tdx.md#1-on-your-computer-the-tools-and-the-source), which gives every command:

1. `bash`: a terminal, Git Bash on Windows, or WSL; `git`, Go, `gcloud` and the clone in the **same** environment.
2. `git`, then `git clone https://github.com/spicslab-kr/tee-blockchain`, `cd tee-blockchain`, then
   `bash e2e/0-Setup-local-install.sh` for Go **go1.26.1** exactly. On Linux open a new terminal (or
   `source ~/.bashrc`) before `go version`: the script adds Go to `PATH` in `~/.bashrc`, which only a new interactive
   shell reads.
3. `gcloud`, installed as in the TDX table (apt repository on Ubuntu/Debian and WSL, `winget install --id
   Google.CloudSDK` on Windows, the archive on macOS), then `gcloud auth login` and
   `gcloud config set project <your project id>`, with a project that has billing and **N2D** quota in
   `us-central1-a`. Under WSL a Windows `gcloud` is also found and works, slowly; the Linux one is meant.
4. `anvil` 1.8.0: `curl -fsSL https://foundry.paradigm.xyz | bash`, `source ~/.bashrc`, `foundryup --install v1.8.0`;
   or the release archive `foundry_v1.8.0_linux_amd64.tar.gz`, checked against `foundry_v1.8.0_linux_amd64.sha256`
   (commands in the TDX step); on Windows without WSL, `anvil.exe` from the v1.8.0 release zip.

Versions we used: [toolchain.md](toolchain.md).

### 2. On your computer: the binaries

In the `tee-blockchain` folder of step 1:

```
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-auditor ./cmd/auditor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-user    ./cmd/user
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o linux/att-vendor       ./cmd/vendor
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o linux/att-ra-responder ./cmd/ra-responder
```

The first two run on your computer; the `linux/` two go to the guest.

### 3. On your computer: the guest

```
bash e2e/cloud/gcp-cvm.sh snp create vendor
```

This makes **one** VM, `attestamp-snp-vendor` (`n2d-standard-2`, AMD Milan, Ubuntu 24.04, SEV-SNP, zone
`us-central1-a`), and a firewall rule `attestamp-ra` that lets **your computer's current address** reach port 5000 on
it. It ends by printing the VM's `EXTERNAL_IP`: that is `<guest-ip>` below (`bash e2e/cloud/gcp-cvm.sh snp status`
prints it again).

It also prints the exact image it used, for example `image ubuntu-os-cloud/ubuntu-2404-noble-amd64-v20261105`.
**Write that name down**: whoever rebuilds and checks your values needs a guest from the same image (see "Use the
same image as the other side" below).

The VM takes a minute to boot after `create` returns; if the first `gcloud compute ssh` or `scp` fails to connect, try
again after half a minute. On Windows `gcloud` uses PuTTY, which asks once per new guest whether to store its host key
(or to update it, when the new guest got an address an earlier VM had): answer `y`.

The first `gcloud compute ssh` or `scp` on a computer that has no `~/.ssh/google_compute_engine` yet makes that key
and prints `Updating project ssh metadata`: it adds the public key to the **project**, so every VM in the project,
other people's included, accepts it from then on. That is how `gcloud` works unless the project uses OS Login; if that
is not acceptable in a shared project, ask its owner before this step.

Port 22: a new project opens it to everyone (`default-allow-ssh`). To close that, see `e2e/cloud/README.md`, "SSH to
the guests"; with IAP, add `--tunnel-through-iap` to every `gcloud compute ssh` and `scp` below (checked on
2026-11-16 from Git Bash, both ways, with the `0.0.0.0/0` rule deleted). If plain `gcloud compute ssh` times out, the
project has probably been closed that way: add the flag.

A stopped VM can get a **new** external address when it is started again (seen twice on SEV-SNP on 2026-11-16). After
every `start`, take `<guest-ip>` again from `status`.

Copy the binaries to it, one file per command:

```
gcloud compute scp linux/att-vendor       attestamp-snp-vendor:att-vendor       --zone us-central1-a
gcloud compute scp linux/att-ra-responder attestamp-snp-vendor:att-ra-responder --zone us-central1-a
```

For Option 2 also the two hook scripts (SEV-SNP's own: they extend PCR12, TDX's do not):

```
gcloud compute scp internal/tee/parse/snp/testdata/record/initramfs-hook.sh         attestamp-snp-vendor:initramfs-hook.sh         --zone us-central1-a
gcloud compute scp internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh attestamp-snp-vendor:initramfs-local-bottom.sh --zone us-central1-a
```

Log in:

```
gcloud compute ssh attestamp-snp-vendor --zone us-central1-a
```

### 4. On the guest: prepare it

Everything in steps 4 to 9 is typed on the guest.

```
chmod +x att-vendor att-ra-responder
sudo shutdown -h +240              # a safety stop in 4 hours; a reboot cancels it, so run it again after each one
```

Make the stock boot load an initrd. Google's Ubuntu images boot without one (`GRUB_FORCE_PARTUUID`; see "The boot
setting changed" below and `docs/build/tdx.md`, "`GRUB_FORCE_PARTUUID`"), and the application, or for Option 2 the
hook that checks it, is in the initrd. The original file is kept for step 13:

```
grep -n GRUB_FORCE_PARTUUID /etc/default/grub.d/40-force-partuuid.cfg
sudo cp -p /etc/default/grub.d/40-force-partuuid.cfg /root/40-force-partuuid.cfg.orig
sudo sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' /etc/default/grub.d/40-force-partuuid.cfg
sudo update-grub
```

The base initrd. `mkinitramfs` writes a random seed, so two runs never give the same file; the Vendor makes it once
and publishes it, and an Auditor uses this file rather than its own.

Option 1, the distribution's own (the build pads it to 4 bytes itself before appending the application):

```
sudo cp /boot/initrd.img-$(uname -r) base-initrd.img
sudo chown $USER base-initrd.img
```

Option 2, one with the hook that checks, mounts and reports the image. `tpm2-tools` must be installed **before**
`mkinitramfs`: the hook copies `tpm2_pcrextend` into the initrd. The scripts are removed again right after, so later
kernel updates do not carry them. It goes into its own file, `base-initrd-v.img`, so that Option 1's
`base-initrd.img` stays as it is and both options can be tried on one guest:

```
sudo apt-get update
sudo apt-get install -y squashfs-tools cryptsetup-bin tpm2-tools
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
./att-vendor build --platform amd-sev-snp-initrd --app-dir app --base-initrd base-initrd.img --verify
```

Option 2. It makes the image with `mksquashfs` and `veritysetup`, installed in step 4, and runs as root, as the image of
our recordings was made (`internal/setup/build/cpio/testdata/make-verity-image.sh`):

```
sudo ./att-vendor build --platform amd-sev-snp-verity --app-dir app --base-initrd base-initrd-v.img --verify
```

It ends with `✓ reproducible on this host` and `✓ ready: out/initrd-app.img   build value: out/build-value.json`.
`build-value.json` holds INITRD (the initrd's SHA-256); for Option 2 also the root hash, the PCR12 the hook will
extend for it, and the kernel argument `attestamp.roothash=<root hash>`. Option 2 also writes `out/app.img` and
`out/app.hash`.

### 7. On the guest: install and boot

```
cat /proc/sys/kernel/random/boot_id     # note it: it is new on every boot, and tells the new boot from the old below
sudo ./att-vendor launch --platform amd-sev-snp-initrd --build-value out/build-value.json --reboot
```

(`amd-sev-snp-verity` for Option 2.) It copies the initrd to `/boot/initrd-attestamp.img`, writes the GRUB entry
`attestamp` (the stock entry with only its title and initrd changed, and for Option 2 `attestamp.roothash=…` added to
the kernel command line) to `/etc/grub.d/41_attestamp`, makes it the default in
`/etc/default/grub.d/99-attestamp.cfg`, for Option 2 copies `app.img` and `app.hash` to `/var/lib/attestamp`, runs
`update-grub` and reboots. The connection drops.

The reboot can take a few minutes, and logging in too early reaches the **old** boot, which is still shutting down
(seen on TDX on 2026-11-15: a minute was not enough; on 2026-11-16 the new boot answered 55 to 166 seconds after the
reboot command). The surest test is the boot ID noted above. Log in (the `gcloud compute ssh` command of step 3) and
check:

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
sudo ./att-vendor launch --platform amd-sev-snp-initrd --check
```

(`amd-sev-snp-verity` for Option 2.) It asks the guest for evidence (the SNP report and the vTPM attestation),
verifies it as a User would, requires INITRD to be the built initrd's and, for Option 2, PCR12 to be what the built
image gives, and requires the application to have been unpacked or mounted. It ends with `✓ the boot measured the
build` and writes the values to `out/measurements.json`. If it refuses because INITRD is not the built initrd, or
because GRUB loaded no initrd, and the boot ID is the old one, you are on the old boot: wait and log in again (step 7).
For Option 2, also:

```
cat /run/attestamp-status          # ok
cat /run/attestamp-log             # the check's error output, if any: read it when the status is fail
ls /opt/app-verity/opt/app         # hello.txt
```

### 9. On the guest: answer attestation requests

```
sudo nohup ./att-ra-responder ra-serve --extractor snp-initrd --listen tcp:0.0.0.0:5000 > ra.log 2>&1 < /dev/null &
```

(`--extractor snp-verity` for Option 2. The extractor names are not the format names: `snp-initrd` answers for
`amd-sev-snp-initrd`, `snp-verity` for `amd-sev-snp-verity`.) It is not a service: after every reboot, start it again.
While it attests, the guest fetches Google's AK certificates over the network; a GCP VM can by default. Log out
(`exit`). The `< /dev/null` matters only when the line is sent through `gcloud compute ssh --command '…'` instead of
typed: without it the background process holds the session's input and the command does not return.

Evidence is usable only from our boot. A responder started on the **stock** boot (before step 7, or after step 13)
answers, but every format refuses its evidence with exit 7, `GRUB loaded no initrd, so the application archive was
never measured`, after `signature ok` and `nonce ok`: Google's stock boot loads no initrd (2026-11-16). TDX evidence
from the same state parses; there the refusal comes only at `verify`, when the values are not registered.

### 10. On your computer: get the evidence

```
./att-user ra-request --tee-addr <guest-ip>:5000 --format amd-sev-snp-initrd --out-file evidence.cbor
./att-user ra-parse   --doc evidence.cbor --format amd-sev-snp-initrd
```

`ra-parse` prints `signature ok`, `nonce ok` and the values; they are the ones in `out/measurements.json` on the guest.
If `ra-request` times out, your address is not the one the firewall rule allows (it was set when the rule was first
made): `gcloud compute firewall-rules describe attestamp-ra` shows it.

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

The values the guest measured, then the audit: it asks the guest for fresh evidence, checks it equals those values,
and registers them.

```
gcloud compute scp attestamp-snp-vendor:out/measurements.json measurements.json --zone us-central1-a
./att-auditor audit --tee-addr <guest-ip>:5000 --format amd-sev-snp-initrd --build-measurements measurements.json \
  --rpc http://127.0.0.1:8545 --registry <0x…> --signing-key-file auditor.key \
  --service hello --version 1.0.0 --arch amd64
```

It ends with `PASS: audited and registered`.

### 12. On your computer: verify, as any User

```
./att-user verify --tee-addr <guest-ip>:5000 --format amd-sev-snp-initrd --rpc http://127.0.0.1:8545 --registry <0x…>
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
sudo rm -rf /var/lib/attestamp                  # Option 2 only
sudo cp -p /root/40-force-partuuid.cfg.orig /etc/default/grub.d/40-force-partuuid.cfg
sudo update-grub
cat /proc/sys/kernel/random/boot_id     # note it, as in step 7
sudo systemctl reboot
```

After that boot (log in again as in step 7), `cat /proc/cmdline` shows the stock command line
(`root=PARTUUID=… panic=-1`), and for Option 2 the image is no longer mounted, so its folder can go:

```
sudo rmdir /opt/app-verity         # Option 2 only
```

`base-initrd.img`, `base-initrd-v.img`, `app` and `out` in the home folder are yours to keep or delete.
On your computer, stop anvil (Ctrl-C) and the VM:

```
bash e2e/cloud/gcp-cvm.sh snp stop        # the disk still costs a little; "delete" removes the VM and its disk
```

With `PREFIX=…` in step 3, give it here too (`PREFIX=<you>-snp bash e2e/cloud/gcp-cvm.sh snp stop`): `stop` acts only
on the VMs whose name starts with the prefix, default `attestamp-snp`. To stop one VM by name:
`gcloud compute instances stop <name> --zone us-central1-a`.

The firewall rule `attestamp-ra` is left in place; `gcloud compute firewall-rules delete attestamp-ra` removes it.

### The Auditor's half: rebuild, boot, compare, register

> **Ran on 2026-11-16, both formats, and passed** (`docs/results/sessions-2026-11-16/amd-sev-snp.md`). Two guests
> made in one call from `ubuntu-2404-noble-amd64-v20261105`; the Auditor's own build equalled the Vendor's, both guests
> measured the same values, `audit --vendor-tee` registered the Auditor's own value, and the User's `verify` of the
> Vendor's guest against it gave exit 0. With one byte changed on the Vendor's side only, `audit` said `the Vendor's
> guest runs something else` and `verify` gave exit 1. A4's comparison also caught a wrong input (an extra file in the
> copied `app`) before anything was registered. Measured afterwards (T1, same day): another machine type changes
> MEASUREMENT, another zone did not ("The same inputs"). And the relay of "Same VM"
> still passes: an independent Auditor makes the registered values trustworthy, not the pairing of the two documents.

This is what README's trust model asks of the Auditor: rebuild the Vendor's application independently, confirm that a
TEE really runs those bytes, and only then register. Step 11 above does not do that. It is the same flow as Nitro's,
moved to a confidential VM:

| | Nitro (`e2e/flow1-auditor-registers`) | SEV-SNP, below |
|---|---|---|
| rebuild | the Auditor builds the EIF on its own instance; the build gives all three PCRs | the Auditor builds on its own guest (A4); the build gives INITRD, and for Option 2 the root hash and the PCR12 the hook will extend |
| boot it | the Auditor launches the EIF in its own enclave | the Auditor installs the build and reboots its own guest (A5) |
| compare | the attested PCRs must equal the built ones | verified evidence from the boot must show the built INITRD (and for Option 2 the predicted PCR12) (A6, `launch --check`), and its values are recorded |
| register | `audit`: a fresh attestation of its own enclave equals its build, then register | `audit`: fresh evidence from its own guest equals A6's values, then register; `--vendor-tee` also asks the Vendor's guest (A8) |
| the Vendor | launches the Auditor's EIF | builds from the same inputs and boots (steps 4 to 9), or boots the Auditor's `out/` files, the same bytes |
| the User | verifies the Vendor's enclave against the Auditor's registry | the same, step 12 against the Auditor's registry (A9) |

**Why the Auditor needs a guest of its own.** The build predicts only INITRD (and PCR12). MEASUREMENT (the firmware),
PCR4 (the boot applications), KERNEL and CMDLINE come from the machine and the image. The Auditor learns them by
booting the same inputs on a guest it controls, and they equal the Vendor's only when the inputs below are the same.

What this half does **not** change: SEV-SNP evidence still cannot prove that its two documents come from one VM
([Same VM](#same-vm), T10). The Auditor's rebuild makes the registered values trustworthy; it does not close that gap.

#### The same inputs

The Auditor's values equal the Vendor's only if all of these are the same on both guests. Measured on 2026-11-15
(`docs/plan/trial-2026-11-16.md`): two guests made five days apart from `ubuntu-2404-noble-amd64-v20261105`,
`n2d-standard-2`, `us-central1-a`, built the same `app/hello.txt` into the same initrd and measured the same
MEASUREMENT, PCR4, KERNEL, CMDLINE and INITRD; on 2026-11-16 a third guest of the same image gave the same five values
again. That is the condition the whole half rests on.

| input | how to make it the same | what changes if it differs |
|---|---|---|
| the image, **exact version** (`ubuntu-2404-noble-amd64-v20261105`), not the family `ubuntu-2404-lts-amd64` | the Vendor records the name step 3 printed; the Auditor creates with `IMAGE=<that name>` (A2). A family moves every few weeks | the root file system UUID in CMDLINE, the stock initrd, often KERNEL and PCR4 |
| the kernel | do not `apt upgrade` either guest; `uname -r` must print the same on both | KERNEL, and the Option 1 base initrd |
| the machine type (`n2d-standard-2`) | `MACHINE=` the Vendor's. **Required**: the Auditor's and the Vendor's guest must have the same machine type, or at least the same number of vCPUs | **MEASUREMENT** (measured 2026-11-16, T1). One guest, one boot entry, only the type changed: `n2d-standard-2` (2 vCPUs, 8 GB) `f6790085…`, `n2d-standard-4` (4 vCPUs, 16 GB) **`2279d213…`**, `n2d-highmem-2` (2 vCPUs, 16 GB) `f6790085…` again. So it is the vCPU count, not the memory: the SNP launch digest covers each vCPU's initial register state (VMSA). PCR4, KERNEL, CMDLINE and INITRD did not change |
| the zone (`us-central1-a`) | `ZONE=` the Vendor's | nothing, where measured: a guest made in `us-central1-b` from the same image gave the same five values as `us-central1-a` (2026-11-16, T1). That is one other zone on one day, not every zone for good, so keep the Vendor's zone; it costs nothing |
| the base initrd | Option 1: the image's own `/boot/initrd.img-<kernel>`, the same file on every guest of one image (`4703ec07…` on all three). Option 2: **the Vendor's** `base-initrd-v.img`; `mkinitramfs` never makes the same file twice, so the Auditor must not make its own | INITRD |
| the application folder | the same files byte for byte (and the same modes; `build` normalises them) | INITRD (Option 1); the root hash, so CMDLINE and PCR12 (Option 2) |
| the format: `amd-sev-snp-initrd` or `amd-sev-snp-verity` | the Vendor says which | everything after the firmware |
| this repository's commit | the same commit, built as in step 2 | the archive or image `build` writes (pinned by tests, but only the same code is sure to give the same bytes) |

What the Vendor hands the Auditor, by any channel: the image name, the machine type and zone, the format, the `app`
folder (or the commit of the source it comes from), its `out/build-value.json`, and for Option 2 its
`base-initrd-v.img`. From the Vendor's guest to the Vendor's computer:

```
gcloud compute scp --recurse attestamp-snp-vendor:app      vendor-app        --zone us-central1-a
gcloud compute scp attestamp-snp-vendor:out/build-value.json vendor-build-value.json --zone us-central1-a
gcloud compute scp attestamp-snp-vendor:base-initrd-v.img vendor-base-initrd-v.img --zone us-central1-a   # Option 2
```

#### The steps

The order with both halves: the Vendor's steps 1 to 10, then A1 to A9 instead of step 11, then step 12, then step 13 on
both guests. A1 to A9 are the Auditor's, on the Auditor's computer and its own guest. The commands show Option 1;
for Option 2 write `amd-sev-snp-verity` wherever they say `amd-sev-snp-initrd`, and `snp-verity` for the extractor.

**A1. On the Auditor's computer: tools and binaries.** Step 1 as written, at the Vendor's commit. Step 2, with
`att-auditor` for the guest instead of `att-vendor`:

```
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-auditor ./cmd/auditor
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o att-user    ./cmd/user
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o linux/att-auditor      ./cmd/auditor
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o linux/att-ra-responder ./cmd/ra-responder
```

**A2. On the Auditor's computer: its own guest, from the Vendor's exact image.**

```
IMAGE=ubuntu-2404-noble-amd64-v20261105 MACHINE=n2d-standard-2 ZONE=us-central1-a bash e2e/cloud/gcp-cvm.sh snp create auditor
bash e2e/cloud/gcp-cvm.sh snp status
```

`IMAGE`, `MACHINE` and `ZONE` are the Vendor's ("The same inputs"); the guest is `attestamp-snp-auditor` (with
`PREFIX=` as in "Names", its own prefix). `status` lists each disk's image: both must name the same version. Its
address is `<auditor-guest-ip>` below. Then copy what the build needs, one item per command:

```
gcloud compute scp linux/att-auditor       attestamp-snp-auditor:att-auditor       --zone us-central1-a
gcloud compute scp linux/att-ra-responder  attestamp-snp-auditor:att-ra-responder  --zone us-central1-a
gcloud compute scp --recurse vendor-app    attestamp-snp-auditor:              --zone us-central1-a
gcloud compute scp vendor-base-initrd-v.img attestamp-snp-auditor:base-initrd-v.img --zone us-central1-a   # Option 2
```

and log in: `gcloud compute ssh attestamp-snp-auditor --zone us-central1-a`. The folder arrives as `vendor-app` in
the home folder; on the guest, `rm -rf app && mv vendor-app app`. (Copying straight to `…:app` fails with Windows'
`pscp` when `app` does not exist yet, and leaves an **empty** `app` behind; A4's comparison catches the build of it.
Seen on TDX, 2026-11-16.)

**A3. On the Auditor's guest: prepare it.** Step 4 as written, with `chmod +x att-auditor att-ra-responder`, and with
two differences. Option 1: `base-initrd.img` is copied from `/boot` as written; its SHA-256 must equal
`base_initrd_sha256` in the Vendor's `build-value.json` (`sha256sum base-initrd.img`). Option 2: install
`squashfs-tools cryptsetup-bin` and make `/opt/app-verity`, but **do not** run the hook and `mkinitramfs` lines (nor
install `tpm2-tools`, which only `mkinitramfs` needs): use the Vendor's `base-initrd-v.img`, copied in A2. Check
`uname -r` against the Vendor's.

**A4. On the Auditor's guest: build.** The Auditor's own build of the Vendor's application:

```
./att-auditor build --platform amd-sev-snp-initrd --app-dir app --base-initrd base-initrd.img --verify
```

(Option 2: `sudo ./att-auditor build --platform amd-sev-snp-verity --app-dir app --base-initrd base-initrd-v.img
--verify`.) Its `out/build-value.json` is the Auditor's expected value. `initrd_sha256` must equal the Vendor's, and
for Option 2 `root_hash` and `pcr12` too: the same files on the same base give the same build. If they differ, the
inputs differ; stop and find which.

**A5. On the Auditor's guest: install and boot.** Step 7 with `att-auditor`:

```
cat /proc/sys/kernel/random/boot_id
sudo ./att-auditor launch --platform amd-sev-snp-initrd --build-value out/build-value.json --reboot
```

Wait for the new boot as step 7 says, log in again, and `sudo shutdown -h +240`.

**A6. On the Auditor's guest: compare the boot with the build.**

```
sudo ./att-auditor launch --platform amd-sev-snp-initrd --check
```

This is the comparison: it verifies fresh evidence and requires INITRD to be the one A4 built, and for Option 2 PCR12
to be the value A4 predicted. It ends with `✓ the boot measured the build` and writes the values to
`out/measurements.json`. If it refuses, the Auditor registers nothing. Those values should equal the Vendor's step 8
values; whether they do is checked against the Vendor's running guest in A8, not by trusting a file the Vendor sent.

**A7. On the Auditor's guest: answer requests.** Step 9 as written (`--extractor snp-initrd`, or `snp-verity`). Log
out. Evidence is usable only from this boot: on the stock boot every format refuses it (step 9).

**A8. On the Auditor's computer: register only if they match.** anvil, the keys and `registry-deploy` as in step 11.
Then, with the Auditor's own values:

```
gcloud compute scp attestamp-snp-auditor:out/measurements.json auditor-measurements.json --zone us-central1-a
./att-auditor audit --tee-addr <auditor-guest-ip>:5000 --format amd-sev-snp-initrd --build-measurements auditor-measurements.json \
  --vendor-tee <guest-ip>:5000 \
  --rpc http://127.0.0.1:8545 --registry <0x…> --signing-key-file auditor.key \
  --service hello --version 1.0.0 --arch amd64
```

`audit` asks the Auditor's own guest for fresh evidence and registers only if it equals `auditor-measurements.json`
(`identical: the guest runs exactly what I built`); otherwise it stops with `not registering anything`. Without
`--build-measurements` it refuses to start at all, on every platform (since 2026-11-16): it would register whatever the guest reports,
an initrdless boot included (`docs/results/sessions-2026-11-16/tdx-initrdless.md`). That refusal does not stop an
Auditor who registers such values by hand with `register`; the trust model has to assume the Auditor does not. With
`--vendor-tee` it also asks the Vendor's guest and prints `same: the Vendor's guest runs exactly what I built` or
`the Vendor's guest runs something else`. In the second case it **still registers the Auditor's own value and exits
12**, saying that the registration succeeded and only the Vendor comparison failed; do not run it again (that ends
with exit 5, already registered). A User verifying the Vendor's guest will then be refused (A9), which is
correct.

**A9. On any computer: the User verifies the Vendor's guest.** Step 12 against the registry of A8:

```
./att-user verify --tee-addr <guest-ip>:5000 --format amd-sev-snp-initrd --rpc http://127.0.0.1:8545 --registry <0x…>
```

Exit 0 and `✓ matches  hello 1.0.0 (amd64) by 0x7099…` now mean what README promises, within the limit of
[Same VM](#same-vm): the Vendor's guest runs what an Auditor rebuilt and booted itself. Then step 13 on both guests
(`attestamp-snp-auditor` too) and `bash e2e/cloud/gcp-cvm.sh snp stop`, which stops both.

### Use the same image as the other side

The Vendor's and the Auditor's guests must be made from the **same exact image version**, for example
`ubuntu-2404-noble-amd64-v20261105`, not merely the same family (`ubuntu-2404-lts-amd64`). Why: CMDLINE, the kernel
command line the vTPM measures, names the root file system by its UUID (`root=UUID=9d4d258d-…`), and the Option 1
base initrd is the image's own file. Both come **with the image**, not with the disk: every disk made from one image
version carries the same file system UUID, the same partition UUID and the same `/boot` files. Measured on 2026-11-15
(`docs/plan/trial-2026-11-16.md`): `attestamp-snp-vendor`, made 2026-11-10, and `attestamp-snp-auditor`, made
2026-11-15, both from `v20261105`, used the same base initrd `4703ec07…` and measured the same MEASUREMENT, PCR4,
KERNEL, CMDLINE (`80dfd7d3…`) and INITRD (`11cd710e…`) for the same `app/hello.txt`.

A family is a pointer that Canonical moves to a new version every few weeks. `gcp-cvm.sh` resolves it once, prints the
exact name and takes `IMAGE=<name>` to make the other side from the same one.

If the two images differ, expect:

| what differs | what changes |
|---|---|
| another version of the same family | a new root file system UUID, so **CMDLINE**; a new stock initrd, so the Option 1 base and **INITRD**; often a new kernel, so **KERNEL**, and a new shim or GRUB, so **PCR4** |
| another kernel on the same image (an `apt upgrade` on one side) | **KERNEL** and the base initrd (`/boot/initrd.img-<version>`), so **INITRD** |
| another machine type with another vCPU count (`n2d-standard-2` vs `n2d-standard-4`) | **MEASUREMENT**, measured 2026-11-16 (`f6790085…` vs `2279d213…`); the same vCPU count with more memory (`n2d-highmem-2`) did not change it |
| another zone (`us-central1-a` vs `us-central1-b`) | nothing, measured 2026-11-16 with one guest in each |
| another firmware (GCP updates its OVMF) | **MEASUREMENT** |

The values then differ and `audit` refuses, correctly: it is another boot. Both sides move to a new image together
and register again. An old image version can also be deleted by Canonical some time after it is deprecated;
`gcp-cvm.sh` then says it was not found.

## How the evidence is shaped

On GCP the SNP launch measurement covers only the virtual firmware. The boot files that carry the application are
measured by the cloud's vTPM. So one piece of evidence is two documents from two signers:

| part | signed by | carries |
|---|---|---|
| SNP attestation report (1,184 bytes) | the chip's VCEK, chaining to AMD's ARK | the firmware's launch digest, the guest policy, TCB versions, 64 bytes of REPORT_DATA |
| vTPM attestation (go-tpm-tools `attest.Attestation`) | Google's attestation key (AK), chaining to Google's "EK/AK CA Root" | quotes over the PCRs with the verifier's nonce, the TCG event log |

They are bound by **REPORT_DATA = SHA3-512 of the vTPM attestation bytes**: the AMD signature covers REPORT_DATA, so
it covers exactly the vTPM evidence carried next to it. SHA3-512 gives 64 bytes, the size of REPORT_DATA, so the hash
goes in unchanged.

The binding was SHA-512 until **2026-11-11**, when it was changed to SHA3-512 by decision of the project lead.
*Record:* SHA-512 is what the 2026-11 test scripts used; those scripts are no longer available (build-info gaps #5).
Results from 2026-11-10 and earlier were produced with SHA-512. Evidence bound with SHA-512 is now refused
(`TestVerifySig_SHA512BindingIsRefused`, on the one SHA-512 recording this repository keeps).

The SHA3-512 binding applies to the GCP path, the only one we use; if an AWS path is ever added, its binding must be
changed to match.

### Only GCP

SEV-SNP is done on GCP only (decided 2026-11-11); AWS is used for Nitro Enclaves. **The AWS path is not ours:** on
AWS, SEV-SNP is an instance option and the vTPM (NitroTPM) an AMI attribute, so a guest with both needs a snapshot of
a running instance re-registered as an AMI with TPM support (`e2e/cloud/aws-snp.sh`, kept for reference). *Record:*
the 2026-11 AWS runs, made with scripts that are no longer available, reported the same results as GCP, including the
same-VM weakness below; they cannot be re-run.

### Decision: one CBOR container, two format strings

**Container.** `RADoc.Raw` holds one CBOR map with three byte strings:

```
{ "report": <SNP report>, "certs": <the host's certificate table, may be empty>, "vtpm": <vTPM attestation> }
```

Why: the wire protocol carries one format string and one byte string (`tee/request/*/wire.go`), so the two documents
must travel as one. CBOR is what Nitro documents already use and `fxamacker/cbor` is already a dependency; byte
strings need no base64; named keys allow a field to be added later. `snp.Encode` writes CBOR's core deterministic
encoding, so the same evidence always gives the same bytes. The decoder refuses unknown keys and duplicate keys, so
an old verifier cannot silently ignore a field it does not understand. The host's certificate table is carried so
that the AMD chain is checked offline, without asking AMD's key server.

**Format strings.** Two names for the same container:

| format | the application is | values registered |
|---|---|---|
| `amd-sev-snp-initrd` | a cpio archive appended to the base initrd (Option 1) | MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD |
| `amd-sev-snp-verity` | a squashfs image on disk, checked by dm-verity at boot against a root hash on the kernel command line; the result is extended into PCR12 (Option 2) | MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD, PCR12 |

TDX does not need this split: there the Option 2 image goes inside the initrd, which RTMR2 already covers, so
`intel-tdx` carries both options with the same four names (`docs/build/tdx.md`, "Option 1 and Option 2"). On SEV-SNP
the image sits on disk, so a boot-time hook extends PCR12 and a sixth value appears.

Why two: the on-chain `measurementHash` is computed over the sorted list of `name=value` lines. One format with five
values in one case and six in another would give hashes that can never be compared, and a User could not tell
which list a registration used. With two names each list is fixed, and the platform string in the record key keeps
the two apart on chain.

## The values

| name | what it is | where it comes from |
|---|---|---|
| MEASUREMENT | the SNP launch digest (48 bytes): the virtual firmware the guest started in | the AMD-signed report |
| PCR4 | the boot applications the firmware ran (shim and GRUB on GCP's Ubuntu images) | the vTPM quote, SHA-256 bank |
| KERNEL | SHA-256 of the kernel file GRUB loaded | GRUB's measurement into PCR9, replayed from the event log |
| CMDLINE | SHA-256 of the kernel command line GRUB passed, as text | GRUB's `kernel_cmdline` event |
| INITRD | SHA-256 of the initrd file GRUB loaded (base initrd + application archive) | GRUB's measurement into PCR9 |
| PCR12 | the dm-verity result (`verity` only) | the vTPM quote, SHA-256 bank |

Why more than the application's hash: GRUB and the kernel are what measure and load the initrd. Registering INITRD
alone would let a changed boot loader or kernel report the expected initrd while booting another. PCR4, KERNEL and
CMDLINE close that gap. The same reasoning made TDX register MRTD and RTMR0–2, and Nitro PCR0–2.

Why SHA-256: the verity hook (`internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh`) extends only the
SHA-256 bank, and PCR12's expected value is computed with SHA-256 (`app_fixture_test.go`; it matched the Option 2
recordings of 2026-11-11).

Left out, because they describe the machine or the run rather than the image: `CHIP_ID`, the TCB versions, the
report's `HOST_DATA`, and every other PCR. Nitro leaves out PCR3, PCR4 and PCR8 for the same reason.

The checks `ra-parse` makes, in order (confirmed against the recorded evidence, `internal/tee/parse/snp/*_test.go`):

1. The VCEK from the certificate table chains to the AMD ARK go-sev-guest embeds, and the report's signature
   verifies. go-sev-guest does both in one call, so a certificate problem here is reported as a signature failure.
2. The AK certificate chains to Google's EK/AK CA Root (embedded as `root.pem`).
3. The quotes' signatures, and the event log replayed against the quoted PCRs (go-tpm-tools).
4. REPORT_DATA = SHA3-512(vTPM attestation).
5. The quotes carry the nonce the verifier sent.
6. POLICY.DEBUG (bit 19) is clear. A debug guest is refused.
7. GRUB loaded an initrd. An initrdless boot is refused (see "Variables").
8. For `verity`: the command line has `attestamp.roothash=` and PCR12 is not zero.

## Time and freshness

A Nitro document carries a `timestamp`, and `internal/tee/parse/aws/verify.go` judges the certificate chain as of
that time, because a Nitro leaf certificate lives about three hours. Is there an equivalent here?

**There is no time in either document.**

- The SNP report has no timestamp or wall-clock field. Confirmed: AMD SEV-SNP Firmware ABI Specification (pub. 56860,
  rev. 1.59, Aug 2026), Table 27, offsets 00h–49Fh. The only time-like feature in that spec is Secure TSC (§7.9),
  the guest's own cycle counter, which is not in the report.
- The TPM quote's `TPMS_CLOCK_INFO.clock` is "a time value in milliseconds that advances while the TPM is powered";
  where zero is "out of the scope of this specification". It is reset by `TPM2_Clear` and can be set forward by
  `TPM2_ClockSet`. Confirmed: TCG TPM 2.0 Library Part 2 r1.59, Table 120 (§10.11.1); Part 1 §36.3 calls its
  accuracy approximate. It is a counter, not a trusted wall-clock time.

**The TCB fields are versions, not times.** Confirmed in 56860, Table 27 and §3.3–3.4:

| field | meaning |
|---|---|
| CURRENT_TCB (38h) | the TCB version of the firmware running now |
| REPORTED_TCB (180h) | the TCB version used to derive the VCEK that signed this report; set by the hypervisor, never above the installed TCB |
| COMMITTED_TCB (1E0h) | the TCB the firmware cannot be rolled back below |
| LAUNCH_TCB (1F0h) | the TCB when the guest was launched. Table 27 says "the CommittedTcb at the time the guest was launched", the guest-context table says "the CurrentTcb" — the spec is inconsistent |

In our recorded GCP report all four are `0xe11f000000000004` (confirmed, `gcp-n2d-initrdless-report.bin`).

**The certificates live for years, so judging them "now" is enough.**

| certificate | lifetime | source |
|---|---|---|
| VCEK in our GCP evidence | 2026-11-01 → 2033-11-01 | confirmed: `gcp-n2d-auxblob.bin` |
| VCEK in go-sev-guest's testdata (Milan) | 2022-09-24 → 2029-09-24 | confirmed: `verify/testdata/vcek.testcer` |
| the same chip's VCEK fetched from AMD's KDS on 2026-11-10 | 2026-11-09 → 2033-11-09, same public key | observed: KDS reissues it with the fetch date |
| ARK-Milan, ASK (SEV-Milan) | 2020-10-22 → 2045-10-22 | confirmed: `kdsintf.amd.com/vcek/v1/Milan/cert_chain` |
| Google AK certificate in our evidence | 2026-11-10 → 2056-11-02 | confirmed: `gcp-n2d-initrdless-att.pb` |
| Google EK/AK CA Root | 2022-07-08 → 2122-07-08 | confirmed: `root.pem` |

What AMD's KDS specification (pub. 57230) says about VCEK lifetime is **not confirmed**: the document could not be
retrieved.

**Which key signs: the VCEK.** Google's page says "Version Chip Endorsement Key (VCEK) certificates are cached on
the VM instead of requested directly from AMD's key distribution system"
(docs.cloud.google.com/confidential-computing/confidential-vm/docs/attestation), and both our recorded reports have
SIGNING_KEY = 0 (VCEK) with a VCEK in the host's certificate table (confirmed). Whether GCP ever uses a VLEK is not
confirmed.[^vlek]

[^vlek]: The other AMD key, the VLEK, is not on our path. AWS signs with a VLEK on shared tenancy (and a VCEK on
    Dedicated Hosts), per AWS's EC2 user guide (snp-attestation page). Its chain is
    `kdsintf.amd.com/vlek/v1/Milan/cert_chain`: ASVK (SEV-VLEK-Milan) 2022-11-16 → 2047-11-16 (confirmed from that
    URL). A VLEK lives from one day before issue to seven years after (confirmed: VLEK spec, pub. 58369 rev. 0.10,
    Table 8). The parser has never been run on VLEK-signed evidence.

**Decision.**

1. Both chains are judged at verification time (`Parser.Now`, default the current time). A seven-year VCEK and a
   thirty-year AK certificate make "valid when the report was made" and "valid now" practically the same question,
   and the first cannot be asked anyway: nothing in the evidence says when it was made.
2. Freshness comes only from the nonce. The verifier's nonce is in the quotes' `extraData`; the quotes are inside the
   vTPM attestation; its SHA3-512 is REPORT_DATA. So a report replayed from an earlier request carries an earlier
   nonce and is refused. The verifier's own clock at the time it made the nonce bounds when the evidence was made.
3. Staleness of the platform is a TCB question, not a time question. go-sev-guest's `validate` package can require
   minimum TCB versions (`MinimumTCB`, `MinimumLaunchTCB`) and check that the VCEK's TCB equals REPORTED_TCB.
   Attestamp does not apply any of that yet (see "Not done yet").
4. The TPM `clock` is not used. It could only order quotes from the same TPM within one reset period.

Library behaviour relied on (confirmed in go-sev-guest v0.15.0 source): `verify.Options.Now` is "the time at which
to verify the validity of certificates. If unset, uses time.Now()"; CRL checking (`CheckRevocations`) is off by
default; `DisableCertFetching` keeps it offline. go-sev-guest v0.15.0 accepts report versions 2–5 only. Spec 1.59
defines version 6; our GCP report is version 5 (confirmed). A version 6 report would be refused until the library is
updated.

## Variables

*Record:* each row changed a hash in the 2026-11 tests, with the same source code (observed on GCP and AWS, with
scripts that are no longer available and cannot be re-run). Where this repository has measured the same thing itself,
the row says so.

| variable | what changed | how we kept it fixed |
|---|---|---|
| Go version | Go 1.22.2 and 1.23.4 gave different application binaries (record) | the same Go version on both sides. **Ours:** the responder built with go1.26.1 and go1.26.2 differs (Nitro baseline 2026-11-12, `toolchain-pin-check.sh`), and `build` refuses another version. **On a SEV-SNP guest (2026-11-16, T2):** the same application under the two versions booted to INITRD `48e09d59…` vs `bca95760…` |
| build path | without `-trimpath`, two folders gave two different binaries (record) | `-trimpath`. **Ours:** `TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths`, and two checkout paths gave the same Nitro PCRs (2026-11-12). **On a SEV-SNP guest (2026-11-16, T3):** one Go application from two checkout paths, built into an initrd from two folders of different depth, gave the same INITRD `48e09d59…` (booted and measured) and, for Option 2, the same image and root hash `4a517772…` |
| cpio tool | GNU cpio 2.13 (Amazon Linux 2023) and 2.15 (Ubuntu) packed the same files into different bytes (directory link counts) (record) | our own cpio writer: `internal/setup/build/cpio` (sorted names, time 0, owner 0, fixed inode numbers). It was byte-identical to the 2026-11 scripts' `mkcpio.py`, which is no longer available; the golden files in `internal/setup/build/cpio/testdata` pin that output |
| distribution base initrd | `mkinitramfs` and `dracut` build a different file every time (it contains a random seed) | none possible. The Auditor uses the base initrd the Vendor publishes and rebuilds only the application part |
| folder permissions (umask) | the same files in a squashfs image gave a different image and root hash under umask 022 (folders 755) and umask 002 (folders 775) | **fixed (2026-11-11):** `internal/setup/build/cpio/testdata/make-verity-image.sh` sets **`umask 022`** and sets every folder and file to mode **755** explicitly. With it, the image built on `attestamp-snp-vendor` and on `attestamp-tdx-vendor` was byte-identical (`app.img` sha256 `28bdfcff…ec1c`, root hash `e29ec085…af0c9`). A build that sets neither still depends on the caller's umask |
| squashfs timestamps | `mksquashfs` writes the build time by default | `-mkfs-time 0 -all-time 0`, file times set to 0 |
| dm-verity salt and UUID | `veritysetup format` picks them at random | fixed `--salt` and `--uuid` |

*Record:* with all of the above fixed, the application, its cpio archive and the dm-verity image hashed the same on GCP
(Ubuntu 24.04, 26.04) and AWS (Amazon Linux 2023), across squashfs-tools 4.4, 4.6.1 and 4.7.5, in the 2026-11 runs.
**Ours:** the dm-verity image was byte-identical on the SEV-SNP and the TDX guest (2026-11-11, squashfs-tools 4.6.1 on
both; `docs/plan/test-matrix.md`, T1).

What else changes the registered values (from the evidence's structure; **not measured by changing each one**):

| value | changes with |
|---|---|
| MEASUREMENT | the virtual firmware GCP launches the guest with; GCP may update it. And the **vCPU count** (**measured** 2026-11-16: `n2d-standard-2` `f6790085…`, `n2d-standard-4` `2279d213…`, `n2d-highmem-2` `f6790085…`; see "The same inputs"); not the zone (`us-central1-b` gave `f6790085…` too) |
| PCR4 | shim and GRUB, i.e. the OS image and its updates |
| KERNEL | the kernel package |
| CMDLINE | the boot entry. It names the root by the file system's UUID (stock entry with an initrd) or the partition's PARTUUID (the initrdless one). Both come with the image version, not the disk: two guests of one image had the same CMDLINE (**measured** 2026-11-15), so the Auditor's guest must be made from the Vendor's exact image ("Use the same image as the other side") |
| INITRD | the base initrd and the application archive, **where the archive goes**, and the padding between them. On SEV-SNP the archive is appended after the base initrd (`cpio.AppLast`), which since 2026-11-11 zero-pads the base to a multiple of 4 bytes. Without that padding the kernel discards the archive and the guest still boots, and INITRD still changes with the application (**measured**, see "Correction: SEV-SNP Option 1 as appended in 2026-11" below) |

**Initrdless boot.** GCP's Ubuntu 24.04 image sets `GRUB_FORCE_PARTUUID`, and GRUB then boots the stock entry
**without any initrd** (confirmed: our first recording, `gcp-n2d-initrdless-*`, logs `GRUB_FORCE_PARTUUID set,
attempting initrdless boot.` and measures no initrd). An application in an initrd is then never loaded or measured.
`ra-parse` refuses evidence without an initrd measurement. Turning the setting off, as below, makes the stock entry
load the initrd; `internal/setup/build/cpio/testdata/make-grub-entry.sh` makes a separate entry from it for an
application (`docs/plan/app-fixture-recording.md`). (*Record:* the 2026-11 scripts avoided it by booting their own
GRUB entry with `grub-reboot`.)

### Correction: SEV-SNP Option 1 as appended in 2026-11

The 2026-10-23 slides (slide 7) say both options work on AWS and GCP. **For SEV-SNP Option 1 that is wrong**, as
measured on 2026-11-11 on `attestamp-snp-vendor` with the same unpadded append the 2026-11 scripts used (*record:* they
ran `cat i.bin app.cpio`):

- The base initrd, `/boot/initrd.img-7.0.0-1017-gcp`, is 50,100,283 bytes: **length mod 4 = 3**.
- The application archive appended directly after it was discarded by the kernel:
  `Initramfs unpacking failed: invalid magic at start of compressed archive` (dmesg, both boots).
- The marker the archive carries (`/dev/attestamp-app-marker`, see `docs/plan/app-fixture-recording.md`) was
  **absent**: the application never ran.
- The guest booted normally on the base initramfs, and GRUB measured the whole combined file, so **INITRD changed with
  the application although the application was not there**. Nothing in the evidence shows it.
- The cause is alignment, not the kernel or zstd. The kernel only accepts an uncompressed archive after a compressed
  segment at a 4-byte-aligned offset (`init/initramfs.c`).
- With the base zero-padded to 50,100,284 bytes (`truncate -s %4`), the same append unpacked: no error line, and the
  marker read `attestamp app v1` (and `v2` on the next boot).

Fixed in `cpio.Join(AppLast)`, which now pads the base (commit `3c57d4c`). Both pairs are kept as fixtures with their
boot logs: `internal/tee/parse/snp/testdata/gcp-n2d-opt1-*` (unpadded) and `gcp-n2d-opt1p-*` (padded).

Whether the 2026-11 SEV-SNP Option 1 runs, on GCP or AWS, actually ran the application depends on the length of the
base initrd they used. Those files, their scripts and any boot logs are gone, so it cannot be checked.

Option 2 does not have this problem. The image is on disk, and a failed check shows as `fail` in PCR12 or a zero PCR12
(measured: `status ok` and the expected PCR12 in both Option 2 recordings).

### The boot setting changed on `attestamp-snp-vendor` (2026-11-11)

Done to record `gcp-n2d-*`, the evidence with an initrd. Every step was checked on the instance:

1. The setting is not in `/etc/default/grub` but in `/etc/default/grub.d/40-force-partuuid.cfg`, line 3:
   `GRUB_FORCE_PARTUUID=<the root partition's PARTUUID>`. The file's own comment says "Remove this line to enable boot
   with an initramfs".
2. Original kept: `sudo cp -p /etc/default/grub.d/40-force-partuuid.cfg /root/40-force-partuuid.cfg.orig`.
3. Line commented out: `sudo sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' /etc/default/grub.d/40-force-partuuid.cfg`.
4. `sudo update-grub`. It printed `Found initrd image: /boot/initrd.img-7.0.0-1017-gcp`, and the default entry in
   `/boot/grub/grub.cfg` became `linux /vmlinuz-7.0.0-1017-gcp root=UUID=<file system UUID> ro console=ttyS0,115200`
   followed by `initrd /initrd.img-7.0.0-1017-gcp`. Before, the command line was `root=PARTUUID=<…> ro
   console=ttyS0,115200 panic=-1` with no initrd.
5. `sudo systemctl reboot`; `/proc/sys/kernel/random/boot_id` changed. The serial console showed `Trying to unpack
   rootfs image as initramfs...` and `Freeing initrd memory: 48928K`, and no longer `attempting initrdless boot`.
6. The TPM event log (`/sys/kernel/security/tpm0/binary_bios_measurements`) holds GRUB's command `initrd
   /initrd.img-7.0.0-1017-gcp` and a measurement of that file. The parser reads that measurement as INITRD, and it equals
   `sha256sum /boot/initrd.img-7.0.0-1017-gcp` on the instance (`4703ec07…b0a2`).

To undo: `sudo cp -p /root/40-force-partuuid.cfg.orig /etc/default/grub.d/40-force-partuuid.cfg`, `sudo update-grub`,
reboot.

The setting stays changed on that instance; it boots with the initrd from now on. One side effect seen: the kernel
command line lost `panic=-1`, which only the initrdless entry adds (observed in the two recordings).

## The instance

`e2e/cloud/gcp-cvm.sh snp create <roles…>` creates it. The settings and why:

| setting | value | why |
|---|---|---|
| machine type | `n2d-standard-2` | SEV-SNP on GCP runs on N2D only (script header, checked against Google's docs 2026-11) |
| CPU platform | `--min-cpu-platform="AMD Milan"` | the script header lists AMD Milan as the CPU platform for SEV-SNP on N2D |
| confidential type | `--confidential-compute-type=SEV_SNP` | |
| maintenance | `--maintenance-policy=TERMINATE` | these VMs cannot live-migrate |
| image | `ubuntu-os-cloud/ubuntu-2404-lts-amd64` | must carry the `SEV_SNP_CAPABLE` guest OS feature; the script checks it |
| zone | `us-central1-a` (default) | one of the zones that offer SEV-SNP |
| vTPM | nothing to set | `/dev/tpm0` and `/dev/tpmrm0` were present without a flag (observed 2026-11-10) |

Our recorded guest, `attestamp-snp-vendor` (read with `gcloud compute instances describe` and `gcloud compute disks
describe` on 2026-11-11):

| | |
|---|---|
| image | `ubuntu-os-cloud/global/images/ubuntu-2404-noble-amd64-v20261105` (the family moves; this is the exact image) |
| boot disk | 64 GB, `pd-standard` |
| CPU platform | AMD Milan (minimum) |
| Shielded VM | Secure Boot **off**, vTPM on, integrity monitoring on |
| maintenance | `TERMINATE` |
| OS, kernel | Ubuntu 24.04.5, `7.0.0-1017-gcp`, with `GRUB_FORCE_PARTUUID` turned off since 2026-11-11 |
| devices | `/dev/sev-guest`, `/dev/tpm0`, `/dev/tpmrm0`; ConfigFS TSM provider `sev_guest`; SEV-SNP active in `dmesg` |
| tools on the guest | OpenSSL 3.0.13 (image default); tpm2-tools 5.6-1build4, installed by hand later the same day for the Option 2 hook. No Go, `snpguest` or `gotpm` |

## From scratch: record and check one evidence pair

1. Create the instance: `bash e2e/cloud/gcp-cvm.sh snp create vendor`. On the instance, set a safety stop:
   `sudo shutdown -h +240`.
2. Build the recorder (`internal/tee/parse/snp/testdata/README.md`, "Recording another one") and copy it over with
   `gcloud compute scp`.
3. On the instance: `sudo ./record -out fx -nonce-text <your text>`. It writes `att.pb`, `report.bin`, `auxblob.bin`
   and `record.txt` (the nonce and REPORT_DATA).
4. Copy the files back and stop the instance: `gcloud compute instances stop <name> --zone <zone>`.
5. Pack the three files into the document `ra-parse` reads, and check it:
   `go run ./internal/tee/parse/snp/testdata/pack -report report.bin -certs auxblob.bin -vtpm att.pb -out doc.cbor`,
   then `att-user ra-parse --doc doc.cbor --format amd-sev-snp-initrd --nonce <hex nonce>`.

On a stock GCP boot step 5 verifies both signatures and the nonce and then refuses, because the boot was initrdless.
Turn `GRUB_FORCE_PARTUUID` off first, as in "The boot setting changed on `attestamp-snp-vendor`" above; the stock
entry then loads the distribution initrd and `amd-sev-snp-initrd` yields five values. **To run an application, use
`build` and `launch`** ("Getting started" above). To record a **test fixture** that carries an application (Option 1),
or the dm-verity image with its root hash on the command line (Option 2), follow `docs/plan/app-fixture-recording.md`:
it builds the initrd with `mkappinitrd`, the image with `make-verity-image.sh`, and the boot entry with
`make-grub-entry.sh` from the instance's own stock entry, so nothing is hard-coded.

The two roots of trust in one SEV-SNP measurement set (AMD for MEASUREMENT, Google's vTPM for the rest), and the
check that MEASUREMENT is identical across our initrd and initrdless recordings, are written up in
`docs/build/ra-comparison.md`, "Roots of trust".

## Byte order

Sources: AMD SEV-SNP Firmware ABI, pub. 56860, **rev. 1.58** (May 2025; the revision we could retrieve, cited elsewhere
in this file at 1.59); GHCB, pub. 56421 rev. 2.04; TCG TPM 2.0 Library Part 1 r1.59; TCG PC Client Platform Firmware
Profile v1.05 r23. "Pinned" means a test fails if the order were the other one.

**SNP report** (offsets into the 0x4A0-byte report). Rev. 1.58 states little endian only for the signature and
public-key formats (Tables 141, 142); for the other fields it gives no general convention. Little endian there is
go-sev-guest's reading, backed by the recorded report, where reserved bits make the wrong order visible.

| field | offset, size | order | spec | pinned by |
|---|---|---|---|---|
| VERSION | 0x00, u32 | LE | Table 23 ("Set to 5h"); order not stated | the signature test (read BE the version is outside 2–5 and go-sev-guest refuses it) |
| POLICY | 0x08, u64 | LE | Table 23; Table 9: bit 17 "must be one", bit 19 DEBUG, bits 63:26 MBZ | `TestEndian_PolicyIsLittleEndian` (LE gives 0x30000; BE would clear bit 17), `TestEndian_PolicyHighReservedBitIsRefused`, `TestParse_DebugGuestIsRefused` |
| SIGNER_INFO (SIGNING_KEY bits 4:2) | 0x48, u32 | LE | Table 23: bits 31:5 MBZ | `TestEndian_SignerInfoReservedBitsAreRefused` (the fixture's value is 0, so it alone could not tell) |
| SIGNATURE_ALGO | 0x34, u32 | LE | Table 23; Table 139: 1 = ECDSA P-384 with SHA-384 | signature test |
| signature R, S | 0x2A0, 0x2E8, 72 bytes each | LE, zero-extended | Table 141: "zero-extended little-endian encoded" | `TestEndian_SignatureRIsLittleEndianZeroExtended`; signature test |
| GUEST_SVN, VMPL, CURRENT/REPORTED/COMMITTED/LAUNCH_TCB, PLATFORM_INFO, mitigation vectors | various u32/u64 | LE | Table 23; order not stated | **not pinned**: nothing judges these values today. It will matter once TCB or VMPL checks are added |

**Certificate table** (the host's `auxblob`, GHCB §4.1.8.1). GUIDs are "formatted and encoded as defined in RFC 4122"
(big-endian fields), pinned by `TestEndian_CertTableGUIDIsRFC4122`: rewritten in the EFI mixed-endian order the VCEK is
no longer found. The entry's offset and length are u32 with no stated order; read LE they point at the certificates
(every decode depends on it).

**vTPM attestation.**

| structure | order | spec | pinned by |
|---|---|---|---|
| TPMS_ATTEST in each quote (magic, type, size prefixes, clockInfo, PCR selection) | BE | TPM 2.0 Part 1 §7.1: "The octet at offset zero within the array is the most significant octet" | `TestCheckNonce`, `TestInfo` (the nonce comes out of `extraData`), signature test |
| TPMT_SIGNATURE, TPMT_PUBLIC | BE | same | signature test |
| TCG event log (event headers, Spec ID event, digests' algorithm IDs and sizes) | LE | PC Client PFP §2.7: "All constants and data SHALL be represented as Little Endian format unless otherwise explicitly stated"; §10.1 | the replay against the quoted PCRs, and KERNEL, CMDLINE, INITRD in `TestParse_RealEvidence` |

So one piece of SEV-SNP evidence mixes three conventions: AMD's little-endian report, the TPM's big-endian quotes and
the firmware profile's little-endian event log, plus RFC 4122 GUIDs in the certificate table and EFI-ordered GUIDs in
the event log. Each library handles its own part.

As with TDX, go-sev-guest verifies the report signature over bytes rebuilt from the parsed fields
(`ReportToAbiBytes`), so a passing signature alone does not pin byte order; the reserved-bit checks above do.

Seen while reading, not verified further: in the path we use (no certificate fetching), go-sev-guest does not appear to
compare REPORTED_TCB with the VCEK certificate's TCB extension, so the TCB is not judged (as "Not done yet" says).

## Runtime measurement: what SEV-SNP can and cannot do

Researched 2026-11-11, in answer to the remark that "TDX has runtime checks and SEV-SNP is written as if it can, but in
practice it cannot". The remark is right for SEV-SNP hardware, and for SEV-SNP on GCP today.

**TDX (confirmed).** RTMR0–3 can be extended by the guest at run time. Since Linux 6.16 the kernel exposes them as
`/sys/devices/virtual/misc/tdx_guest/measurements/rtmr[0-3]:sha384`: writing 48 bytes extends the register
(`TDG.MR.RTMR.EXTEND`). The values appear in the next quote, which Intel's Quoting Enclave signs. Attestamp registers
RTMR0–2 and leaves RTMR3, the runtime register, out.

**SEV-SNP (confirmed, ABI 56860 rev. 1.58).**

- The attestation report has **no register the guest can extend**.
- MEASUREMENT ("calculated at launch"), HOST_DATA, ID_KEY_DIGEST, AUTHOR_KEY_DIGEST, FAMILY_ID and IMAGE_ID are all
  fixed at launch.
- At request time the guest supplies only REPORT_DATA (64 bytes, "the firmware does not interpret this data") and a
  VMPL (Table 22, MSG_REPORT_REQ).
- None of the guest message types (CPUID, KEY, REPORT, EXPORT, IMPORT, ABSORB, VMRK, ABSORB_NOMA, TSC_INFO) extends a
  measurement.
- REPORT_DATA does not accumulate. A "runtime" value can only be hashed into it per request, with no
  hardware-kept history.

**Linux (confirmed).** The kernel's measurement-register interface (`drivers/virt/coco/tsm-mr.c`, 6.16) is used by
`tdx-guest` only. `sev-guest` registers report generation and no measurement registers.

**The vTPM on GCP (confirmed, Google's attestation overview, re-read 2026-11-11).** The vTPM of a GCP SEV-SNP VM is
"managed by the host hypervisor" and belongs to a "Software root of trust (hypervisor dependent)"; for SEV-SNP the page
lists "Bootloader, kernel, and userspace measurements: Shielded VM vTPM". (An earlier version of this paragraph quoted
"software-attested instead of hardware" from that page; the phrase is not on it.) Anything extended into PCR10–23 at run
time (IMA, for example) is signed by Google's attestation key, not by AMD, and that vTPM sits outside the SNP trust
boundary. That is also why our PCR4, KERNEL, CMDLINE, INITRD and PCR12 are Google-signed (see "Same VM").

**What would give SEV-SNP a runtime register.**

- A vTPM inside the guest's trust boundary: an SVSM running at VMPL0, as AMD's SVSM specification (pub. 58019)
  describes.
- Linux has had the guest driver since 6.16 (`drivers/char/tpm/tpm_svsm.c`, `CONFIG_TCG_SVSM`); Canonical backported it
  to `linux-gcp` (June 2025, per the research; not checked on our kernel).
- COCONUT-SVSM's vTPM is ephemeral: its EK is regenerated every boot.
- Azure's Confidential VMs run their vTPM "inside the VM in a hardware-based protected memory region… isolated and
  encrypted by AMD SEV-SNP" (Microsoft's docs).
- For GCP, the only mention found is a Google community blog (June 2026) calling SVSM vTPM key isolation "available in
  preview". Google's documentation and release notes have no entry for it (not confirmed).
- How an SVSM vTPM's key is bound into an SNP report could not be confirmed: spec 58019 could not be retrieved.

| capability | TDX | SEV-SNP on GCP |
|---|---|---|
| launch measurement signed by the hardware vendor | MRTD | MEASUREMENT |
| register the guest can extend, signed by the hardware vendor | RTMR0–3 | **none** |
| Linux interface to extend it | sysfs `measurements/rtmr*`, 6.16 | none |
| per-request guest data | REPORTDATA, 64 bytes | REPORT_DATA, 64 bytes |
| runtime PCRs | GCP vTPM (Google-signed) | GCP vTPM (Google-signed, hypervisor-managed) |
| hardware-rooted vTPM | not needed (RTMRs) | SVSM vTPM: kernel driver since 6.16; on GCP only a "preview" claim in a blog |

Attestamp does not use any runtime measurement on either platform today, and this document makes no runtime claim for
SEV-SNP. A future runtime check on SEV-SNP would rest on Google's vTPM, which the "Same VM" weakness below already
limits.

## Same VM

The binding proves that the AMD-signed report covers this vTPM evidence. It does not prove that both come from the
same VM: a vTPM attestation from an approved VM A, hashed into an SNP report requested on another VM B, verifies.
That is test-matrix row T10.

**The attack.** VM B computes SHA3-512 of A's vTPM attestation and asks its own AMD firmware for a report with that
as REPORT_DATA. B's report is genuine and signed by AMD; A's attestation is genuine and signed by Google's AK; and
REPORT_DATA really is the hash of the attestation next to it, so the binding holds by construction. Simply putting
A's attestation next to a report B made over its *own* attestation is not the attack: the binding refuses that, as
`TestVerifySig_MixedRecordingsFailTheBinding` shows (on recordings of one VM).

**Why a verifier cannot notice: nothing on the AMD side names the VM.** Read from our own eight reports of
`attestamp-snp-vendor` (2026-11-10 and 2026-11-11), and now pinned by `TestReports_IdentifyFirmwareNotVM`:

| field of the SNP report | what it was in our recordings | can it tell A from B? |
|---|---|---|
| MEASUREMENT | **the same on every boot**: initrdless, with the distribution initrd, and with each application (Option 1 and 2) | no: it identifies the firmware GCP launched, which every VM of the same type shares |
| FAMILY_ID, IMAGE_ID, HOST_DATA, ID_KEY_DIGEST, AUTHOR_KEY_DIGEST | **all zero** in every report | no: GCP sets no launch identity at all |
| REPORT_ID | **different on every boot** of the same VM | no: it does not stay with a VM, and nothing in A's vTPM evidence carries it |
| CHIP_ID | **changed** between our recordings of the same VM, as it moved hosts | no: it names the physical host, not the VM, and two VMs may share a host |

The two findings fit together. T10 was an observation (the 2026-11 record says the relay verified); these
measurements explain why it can work: B's report contains nothing a verifier could compare with A's vTPM evidence, so
once the binding holds, there is no remaining check that could tell the two VMs apart. Whatever names A sits on the
Google side (the AK certificate), and nothing on the AMD side matches it.

**Reproduced on 2026-11-16.** Two guests of one image: A ran the registered application, B a one-byte-changed one. B's
report requested over SHA3-512 of A's vTPM attestation (`record-snp -relay-att`) passed `ra-parse` with exit 0,
`signature ok` and `nonce ok`, yielded A's values, and those found the Auditor's registration for A; B's own pair had
values nobody registered. The control, B's own report next to A's attestation, was refused at the binding (exit 5).
The binding works; the relay passes it. A and B ran on different hosts (different `CHIP_ID`s). The four documents,
the table of where each value comes from, and a mitigation to study (pinning the AK certificate's instance, with its
cost): `docs/build/ra-comparison.md`, "The SEV-SNP relay, measured". A live result: the recordings carry instance
identifiers and are not committed (`docs/plan/app-fixture-recording.md`, "Reproducing T10").

Nitro and TDX return one document from one TEE and do not have this problem. On SEV-SNP it goes away only if the boot
files are in the SNP launch measurement itself (direct kernel boot; whether GCP allows it is not confirmed), or if
the vTPM runs inside the SNP guest's own trust boundary, or if GCP set a per-VM value such as HOST_DATA that the vTPM
evidence also carried.

## Not done yet

| what | why |
|---|---|
| a verity boot that fails its check | a verity boot made by `launch` passed `--check` on 2026-11-15 with PCR12 the predicted value; the tamper case (an image that does not match its root hash) was not run |
| TCB minimums, CRLs | not applied; the platform's patch level is not judged |
| AMD chain errors as `ErrChain` | go-sev-guest checks the chain and the signature in one call |
| T10 as a code test | reproduced live on 2026-11-16, but the recordings are not committed (each `att.pb` carries the project ID and an instance's name and ID in the signed AK certificate), so no test runs it without a VM |

AWS evidence is not on this list: SEV-SNP on AWS was decided against (see "Only GCP").

**The 2026-10/11 shell scripts (removed 2026-11-15).** *Record:* the SEV-SNP experiments of 2026-10 and 2026-11 used
shell scripts outside this repository (`vendor/`, `auditor/`, `user/`, `option1-initrd/`, `option2-dm-verity/`,
`relay-test/`, `mkcpio.py`). Their author has left the lab and they are no longer available anywhere (build-info gaps
#5, #6). This document no longer points to them. Everything they did that this repository relies on was redone here:
the recordings, their measured values and the procedures in `docs/plan/app-fixture-recording.md`. Lost and not
replaced: "the seven 2026-11 tests" as one runnable set, and the relay test (T10, see above).

## Checking

`build --verify` builds twice on the same machine and fails if the two differ (both formats; used in step 6). Across
machines, the Auditor's half compares the Auditor's own build with the Vendor's (`initrd_sha256`, and for
`amd-sev-snp-verity` `root_hash` and `pcr12`) before anything is registered; on 2026-11-16 that comparison caught two
wrong inputs (`docs/results/sessions-2026-11-16/`).
