package doccheck_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// TestDocumentReferences checks every tracked text file (Markdown, Go, shell, .gitignore, …) for three kinds of
// reference, and fails on any that does not resolve:
//
//  1. Markdown links [text](target) in .md files: the target file must exist, and a #anchor must be a heading of
//     the target (GitHub's rules for heading anchors).
//  2. Repository paths written as text: any token that starts with docs/, internal/, cmd/, e2e/ or contracts/, or
//     with ../ (resolved from the file's folder). {a,b} groups are expanded and each alternative must exist; a *
//     must match at least one tracked file or folder; a bare prefix that a tool completes (pack -fixture <prefix>,
//     which reads <prefix>-att.pb and so on) counts if tracked files start with it and "-".
//  3. Templates with <role> or <plat>: at least one real role (Vendor, auditor, user) and platform (aws, tdx, snp,
//     gcp, azure) must turn every alternative into an existing path. Other placeholders (<feature>, <pkg>, …, "…")
//     are not checked.
//
// Only tracked files count as existing, so a reference to a file that exists only on the author's machine fails.
// The files of this package are not scanned: they quote the exceptions below as text.
//
// When this test fails, fix the reference. The two lists below are not a place to make a failure go away: each
// entry names one file and one exact token, says why it is not a broken reference, and the test fails if an entry
// stops matching, so the lists can only shrink. An entry is added only when the token is, by its nature, not a
// reference to a file of this repository.
func TestDocumentReferences(t *testing.T) {
	r := loadRepo(t)

	used := map[int]bool{}
	usedUntracked := map[int]bool{}
	var broken []string
	refs, links := 0, 0

	for _, f := range r.files {
		if strings.HasPrefix(f, "internal/doccheck/") {
			continue
		}
		full := filepath.Join(r.root, filepath.FromSlash(f))
		if st, err := os.Stat(full); err == nil && st.IsDir() {
			continue // a submodule (contracts/evm/lib/openzeppelin-contracts): not this repository's text
		}
		b, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.IndexByte(b[:min(len(b), 8192)], 0) >= 0 {
			continue // binary: a recorded attestation document, a quote, an image
		}
		dir := path.Dir(f)
		for no, line := range strings.Split(string(b), "\n") {
			at := fmt.Sprintf("%s:%d", f, no+1)

			if strings.HasSuffix(f, ".md") {
				for _, m := range mdLink.FindAllStringSubmatch(line, -1) {
					target := m[1]
					if scheme.MatchString(target) {
						continue
					}
					links++
					file, frag, _ := strings.Cut(target, "#")
					full := f
					if file != "" {
						full = path.Join(dir, file)
						if !r.exists(full) {
							broken = append(broken, at+": link target missing: "+target)
							continue
						}
					}
					if frag != "" && strings.HasSuffix(full, ".md") && !r.anchors(t, full)[frag] {
						broken = append(broken, at+": anchor missing: "+target)
					}
				}
			}

			for _, loc := range pathToken.FindAllStringIndex(line, -1) {
				if loc[0] > 0 && strings.ContainsRune(`abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./-`, rune(line[loc[0]-1])) {
					continue // inside a longer word or path, e.g. ../../docs/x or a URL
				}
				tok := strings.TrimRight(line[loc[0]:loc[1]], ".,;:`'\")")
				if i := exception(f, tok); i >= 0 {
					used[i] = true
					continue
				}
				if strings.Contains(tok, "<role>") || strings.Contains(tok, "<plat>") {
					refs++
					if !r.templateResolves(tok) {
						broken = append(broken, at+": no role or platform makes this an existing path: "+tok)
					}
					continue
				}
				if strings.ContainsAny(tok, "<>…") || strings.HasSuffix(tok, "...") {
					continue // a placeholder this test cannot fill in
				}
				for _, cand := range expand(tok) {
					refs++
					full := strings.TrimSuffix(cand, "/")
					if strings.HasPrefix(cand, "../") {
						full = path.Join(dir, cand)
					}
					if i := untrackedFile(full); i >= 0 {
						usedUntracked[i] = true
						continue
					}
					if !r.resolves(full) {
						broken = append(broken, at+": path missing: "+cand)
					}
				}
			}
		}
	}

	for _, b := range broken {
		t.Error(b)
	}
	for i, e := range exceptions {
		if !used[i] {
			t.Errorf("exception no longer matches anything, remove it: %s %q", e.file, e.token)
		}
	}
	for i, u := range untracked {
		if !usedUntracked[i] {
			t.Errorf("untracked-by-design entry no longer referenced, remove it: %s", u.path)
		}
		if !r.ignored(t, u.path) {
			t.Errorf("%s is listed as untracked by design but .gitignore does not ignore it", u.path)
		}
	}
	t.Logf("%d tracked files, %d Markdown links, %d path references, %d broken", len(r.files), links, refs, len(broken))
}

// exceptions are tokens that look like repository paths and are not references to a file of this repository. Each
// is one exact token in one file.
var exceptions = []struct{ file, token, why string }{
	{
		file:  "docs/build/app-fixture-recording.md",
		token: "internal/tee/parse/snp/testdata/gcp-n2d-t10-{a,b,relay}-*",
		why: "the names the T10 relay recordings would have had, under \"What committing would have meant\"; " +
			"they are deliberately not committed (they carry instance identifiers)",
	},
	{
		file:  "docs/results/build-info-gaps-2026-11.md",
		token: "../../releases/latest",
		why:   "gap #16 quotes the link the README used to have, which is why it was a gap; the README no longer has it",
	},
	{
		file:  "internal/setup/launch/grub/grub_test.go",
		token: "../x.img",
		why:   "an input the test feeds to the GRUB entry writer to check that a path leaving /boot is refused",
	},
	{
		file:  "internal/setup/launch/grub/grub_test.go",
		token: "../etc/passwd",
		why:   "the same: a path-traversal input that must be refused",
	},
	{
		file:  "internal/tee/parse/snp/testdata/pack/main.go",
		token: "internal/tee/parse/snp.Encode",
		why:   "a Go identifier, the function Encode of package internal/tee/parse/snp, not a file",
	},
}

func exception(file, token string) int {
	for i, e := range exceptions {
		if e.file == file && e.token == token {
			return i
		}
	}
	return -1
}

// untracked are files the documents name but that are not in the repository on purpose: the reader or a tool
// creates them, and .gitignore keeps them out (the test checks that it does).
var untracked = []struct{ path, why string }{
	{"e2e/env.local.sh", "the reader's instance addresses, copied from e2e/env.local.example.sh; must never be committed"},
	{"docs/status/night-log.md", "the overnight sessions' first-person work log, kept locally; what other documents cited from it was moved into them"},
	{"docs/status/cvm-port.md", "the confidential-VM port's working plan, kept locally since the work is done; dated records still name it, and what other documents cited from it was moved into them"},
	{"contracts/evm/out/Registry.abi", "written by forge build in the go:generate chain of internal/chain/register/*/ether, one line before abigen reads it"},
	{"contracts/evm/out/Registry.bin", "the same: forge's output, read by abigen"},
}

func untrackedFile(p string) int {
	for i, u := range untracked {
		if u.path == p {
			return i
		}
	}
	return -1
}

var (
	mdLink    = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	scheme    = regexp.MustCompile(`^[a-z]+:`)
	pathToken = regexp.MustCompile(`(?:\.\./)+[\w./-]+|(?:docs|internal|cmd|e2e|contracts)/[\w.{},*<>…/-]*`)
	heading   = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	inlineMD  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	braces    = regexp.MustCompile(`\{([^{}]*)\}`)
)

type repo struct {
	root    string
	files   []string
	tracked map[string]bool
	dirs    map[string]bool
	anchor  map[string]map[string]bool
}

func loadRepo(t *testing.T) *repo {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repository root not found at %s", root)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed: the tracked files cannot be listed")
	}
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("not a git checkout (%v): the tracked files cannot be listed", err)
	}
	r := &repo{root: root, tracked: map[string]bool{}, dirs: map[string]bool{}, anchor: map[string]map[string]bool{}}
	for _, f := range strings.Split(string(out), "\x00") {
		if f == "" {
			continue
		}
		r.files = append(r.files, f)
		r.tracked[f] = true
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			r.dirs[d] = true
		}
	}
	return r
}

func (r *repo) exists(p string) bool { return r.tracked[p] || r.dirs[p] }

// resolves reports whether p names a tracked file or folder, matches one with a *, or is a prefix that tracked
// files continue with "-".
func (r *repo) resolves(p string) bool {
	if strings.Contains(p, "*") {
		for f := range r.tracked {
			if ok, _ := path.Match(p, f); ok {
				return true
			}
		}
		for d := range r.dirs {
			if ok, _ := path.Match(p, d); ok {
				return true
			}
		}
		return false
	}
	if r.exists(p) {
		return true
	}
	// Only "-", never ".": a prefix continued with "." would accept a reference that lost its extension
	// (internal/tee/parse/<role>/aws for aws.go, the very error this test was first written after).
	for f := range r.tracked {
		if strings.HasPrefix(f, p+"-") {
			return true
		}
	}
	return false
}

func (r *repo) templateResolves(tok string) bool {
	for _, role := range []string{"Vendor", "auditor", "user"} {
		for _, plat := range []string{"aws", "tdx", "snp", "gcp", "azure"} {
			all := true
			for _, c := range expand(strings.NewReplacer("<role>", role, "<plat>", plat).Replace(tok)) {
				if !r.resolves(strings.TrimSuffix(c, "/")) {
					all = false
					break
				}
			}
			if all {
				return true
			}
		}
	}
	return false
}

func (r *repo) ignored(t *testing.T, p string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", "--no-index", p)
	cmd.Dir = r.root
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("git check-ignore: %v", err)
	}
	return err == nil
}

// anchors returns the heading anchors of a Markdown file as GitHub makes them: lower case, punctuation dropped,
// spaces to hyphens, and -1, -2, … for repeats. Headings inside fenced code blocks do not count.
func (r *repo) anchors(t *testing.T, file string) map[string]bool {
	if a, ok := r.anchor[file]; ok {
		return a
	}
	b, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	a, seen, fenced := map[string]bool{}, map[string]int{}, false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		m := heading.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if fenced || m == nil {
			continue
		}
		s := slug(m[2])
		if n := seen[s]; n > 0 {
			a[fmt.Sprintf("%s-%d", s, n)] = true
		} else {
			a[s] = true
		}
		seen[s]++
	}
	r.anchor[file] = a
	return a
}

func slug(h string) string {
	h = strings.ReplaceAll(inlineMD.ReplaceAllString(h, "$1"), "`", "")
	var s strings.Builder
	for _, c := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case c == ' ':
			s.WriteRune('-')
		case c == '-' || c == '_' || unicode.IsLetter(c) || unicode.IsNumber(c):
			s.WriteRune(c)
		}
	}
	return s.String()
}

// expand turns every {a,b} group into its alternatives.
func expand(tok string) []string {
	loc := braces.FindStringSubmatchIndex(tok)
	if loc == nil {
		return []string{tok}
	}
	var out []string
	for _, alt := range strings.Split(tok[loc[2]:loc[3]], ",") {
		out = append(out, expand(tok[:loc[0]]+alt+tok[loc[1]:])...)
	}
	return out
}
