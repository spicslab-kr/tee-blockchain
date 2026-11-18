package aws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// scratchDirs are scratch directories that should be empty at run time anyway. Files left by mktemp and the like during the build have random
// names and differ on every build (seen before this repository, not recorded: the official memcached alpine Dockerfile left one /tmp/<random> file, and PCR0 and PCR2
// diverged although all binaries were identical). The directory stays; only its contents are emptied.
var scratchDirs = []string{"tmp", "var/tmp"}

// scrubBuildScratch empties scratchDirs and returns the relative paths of the directories it actually emptied.
func scrubBuildScratch(ctx context.Context, r runner, root string) []string {
	var scrubbed []string
	for _, d := range scratchDirs {
		dir := filepath.Join(root, filepath.FromSlash(d))
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) == 0 {
			continue
		}
		for _, e := range entries {
			full := filepath.Join(dir, e.Name())
			if err := os.RemoveAll(full); err != nil {
				if err := r.cmd(ctx, "rm", "-rf", full).Run(); err != nil {
					fmt.Fprintf(r.log, "warning: could not remove %s: %v\n", full, err)
				}
			}
		}
		scrubbed = append(scrubbed, d)
	}
	return scrubbed
}

// apkVirtualVersion is the default version apk add --virtual assigns: the install time, YYYYMMDD.HHMMSS.
var apkVirtualVersion = regexp.MustCompile(`^[0-9]{8}\.[0-9]{6}$`)

// apkFixedIdentity is the placeholder identifier written to match the pinned version. apk does not run again inside the enclave, so only the format matters.
const apkFixedIdentity = "C:Q10000000000000000000000000000="

// fixApkVirtualVersions handles apk virtual packages (--virtual, name starting with ".", bundling dependencies without files).
// Their version defaults to the install time, so the build time is embedded in /etc/apk/world and /lib/apk/db/installed
// (seen before this repository, not recorded: official haproxy and memcached alpine; of several hundred files only these two differed, and PCR0 and PCR2
// diverged). That records when it was installed, not what, so the version is pinned to 0 and the identifier (C:)
// derived from that version is replaced by a placeholder. It returns the relative paths of the files it fixed.
func fixApkVirtualVersions(ctx context.Context, r runner, root string) []string {
	var fixed []string

	world := filepath.Join(root, "etc", "apk", "world")
	if b, err := readRoot(ctx, r, world); err == nil {
		changed := false
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		for i, line := range lines {
			if name, ver, ok := strings.Cut(line, "="); ok && strings.HasPrefix(name, ".") && apkVirtualVersion.MatchString(ver) {
				lines[i] = name + "=0"
				changed = true
			}
		}
		if changed && writeRoot(ctx, r, world, []byte(strings.Join(lines, "\n")+"\n")) == nil {
			fixed = append(fixed, "etc/apk/world")
		}
	}

	installed := filepath.Join(root, "lib", "apk", "db", "installed")
	if b, err := readRoot(ctx, r, installed); err == nil {
		changed := false
		blocks := strings.Split(string(b), "\n\n")
		for i, blk := range blocks {
			lines := strings.Split(blk, "\n")
			virtual := false
			for _, l := range lines {
				if strings.HasPrefix(l, "P:.") {
					virtual = true
					break
				}
			}
			if !virtual {
				continue
			}
			for j, l := range lines {
				switch {
				case strings.HasPrefix(l, "V:") && apkVirtualVersion.MatchString(l[2:]):
					lines[j] = "V:0"
					changed = true
				case strings.HasPrefix(l, "C:") && l != apkFixedIdentity:
					lines[j] = apkFixedIdentity
					changed = true
				}
			}
			blocks[i] = strings.Join(lines, "\n")
		}
		if changed && writeRoot(ctx, r, installed, []byte(strings.Join(blocks, "\n\n"))) == nil {
			fixed = append(fixed, "lib/apk/db/installed")
		}
	}
	return fixed
}

// shadowFiles hold the "last password change" date of every account: field 3, in days since 1970-01-01. adduser and
// useradd write the day they ran, so the same Dockerfile built on two days gives two files (measured 2026-11-15: the
// official nginx alpine-slim image built from a 2026-11-12 cache had nginx:!:20769:..., the rebuild on 2026-11-15
// 20772, and PCR0 and PCR2 differed in that one field). /etc/shadow- is the backup shadow-utils writes beside it.
var shadowFiles = []string{"etc/shadow", "etc/shadow-"}

// fixShadowDates empties field 3 of every line in shadowFiles and returns the relative paths it changed. Empty is what
// the distributions' own base accounts carry (alpine: root:*::0:::::) and means "password ageing off". That changes
// nothing an enclave does: there is no login into an enclave, so no password is ever checked or aged there. A line
// with a real password hash is left as it is apart from the date; its salt is random anyway, and a Dockerfile that
// sets a password is not reproducible for that reason (docs/build/aws-nitro.md).
func fixShadowDates(ctx context.Context, r runner, root string) []string {
	var fixed []string
	for _, rel := range shadowFiles {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if !exists(ctx, r, path) {
			continue
		}
		b, err := readRoot(ctx, r, path)
		if err != nil {
			fmt.Fprintf(r.log, "warning: could not read /%s (the build day stays in the measurements): %v\n", rel, err)
			continue
		}
		out, changed := blankShadowDates(b)
		if !changed {
			continue
		}
		if err := writeRoot(ctx, r, path, out); err != nil {
			fmt.Fprintf(r.log, "warning: could not rewrite /%s (the build day stays in the measurements): %v\n", rel, err)
			continue
		}
		fixed = append(fixed, rel)
	}
	return fixed
}

// blankShadowDates empties the third colon-separated field of each line that has one. Other fields are untouched.
func blankShadowDates(b []byte) ([]byte, bool) {
	lines := strings.Split(string(b), "\n")
	changed := false
	for i, l := range lines {
		f := strings.Split(l, ":")
		if len(f) < 3 || f[2] == "" {
			continue
		}
		f[2] = ""
		lines[i] = strings.Join(f, ":")
		changed = true
	}
	return []byte(strings.Join(lines, "\n")), changed
}

// buildCaches are caches tools leave during the build. Their content is the state of that moment, such as times and inodes, not file
// content, so it differs on every build of the same source, and nothing inside the enclave uses it again (seen before this repository, not recorded: the official
// mariadb Dockerfile on two instances; of 13,063 entries only root/.wget-hsts (wget's HSTS store, with visit times) and
// var/cache/ldconfig/aux-cache (ldconfig's cache of library stat data) differed, and PCR0 and PCR2 diverged. tomcat's aux-cache differed too). They are removed.
var buildCaches = []string{
	"var/cache/ldconfig/aux-cache", // glibc ldconfig: records library inodes and mtimes. ld.so.cache is content-based, so it stays
	"root/.wget-hsts",              // wget: visited HSTS hosts and times
	"root/.gnupg/random_seed",      // gnupg: random seed
	"root/.cache",                  // cache tree used by pip, go, npm and others
}

// stripBuildCaches removes buildCaches from the extracted rootfs and returns the relative paths it actually removed.
func stripBuildCaches(ctx context.Context, r runner, root string) []string {
	var removed []string
	for _, p := range buildCaches {
		full := filepath.Join(root, filepath.FromSlash(p))
		if !exists(ctx, r, full) {
			continue
		}
		if err := os.RemoveAll(full); err != nil {
			if err := r.cmd(ctx, "rm", "-rf", full).Run(); err != nil {
				fmt.Fprintf(r.log, "warning: could not remove %s: %v\n", p, err)
				continue
			}
		}
		removed = append(removed, p)
	}
	return removed
}

// exists reports whether a path is present in the extracted tree. Inside directories only root can look into, such as /root (700) and /var/cache/ldconfig (700),
// os.Lstat fails with a permission error (seen before this repository: that is why mariadb's root/.wget-hsts was not removed), so then the runner's (sudo) test -e is used.
func exists(ctx context.Context, r runner, path string) bool {
	_, err := os.Lstat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return r.cmd(ctx, "test", "-e", path).Run() == nil
}

// readRoot and writeRoot: the extracted rootfs is root-owned because tar extracted it with sudo. If direct access fails, read and write through the runner (sudo).
func readRoot(ctx context.Context, r runner, path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		return b, nil
	}
	return r.output(ctx, "cat", path)
}

func writeRoot(ctx context.Context, r runner, path string, b []byte) error {
	if err := os.WriteFile(path, b, 0o644); err == nil {
		return nil
	}
	c := r.cmd(ctx, "tee", path)
	c.Stdin = bytes.NewReader(b)
	return c.Run()
}
