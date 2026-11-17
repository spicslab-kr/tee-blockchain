package doccheck_test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestCommitReferences checks that every commit a document names exists in this repository and is named
// unambiguously. The documents' convention: a commit is a bare hex string of 7 to 12 characters in backticks
// (`a9a45f2`); a value is written with a trailing "…" (`c61c988e…`) and is not a commit, so it is not checked here.
//
// It needs the full history: in a shallow clone it skips. If the history is ever rewritten, every reference to a
// commit before the rewrite fails here, which is the point: the documents would then name commits that no longer
// exist.
//
// The exceptions are commits of other repositories, named on purpose. As in TestDocumentReferences, each is one
// exact token with its reason, and the test fails when one is no longer used, so the list can only shrink.
func TestCommitReferences(t *testing.T) {
	r := loadRepo(t)
	shallow := exec.Command("git", "rev-parse", "--is-shallow-repository")
	shallow.Dir = r.root
	if out, err := shallow.Output(); err != nil || strings.TrimSpace(string(out)) == "true" {
		t.Skip("a shallow clone (or no git history): the commits cannot be checked")
	}

	where := map[string][]string{}
	var order []string
	for _, f := range r.files {
		if strings.HasPrefix(f, "internal/doccheck/") {
			continue
		}
		full := filepath.Join(r.root, filepath.FromSlash(f))
		if st, err := os.Stat(full); err != nil || st.IsDir() {
			continue
		}
		b, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.IndexByte(b[:min(len(b), 8192)], 0) >= 0 {
			continue
		}
		for no, line := range strings.Split(string(b), "\n") {
			for _, m := range commitToken.FindAllStringSubmatch(line, -1) {
				tok := m[1]
				if strings.Trim(tok, "0123456789") == "" {
					continue // digits only: a date such as 20261115, not a commit
				}
				if _, seen := where[tok]; !seen {
					order = append(order, tok)
				}
				where[tok] = append(where[tok], f+":"+strconv.Itoa(no+1))
			}
		}
	}

	// One git call for all of them: "<hash> commit <size>", "<token> missing" or "<token> ambiguous".
	cmd := exec.Command("git", "cat-file", "--batch-check=%(objecttype)")
	cmd.Dir = r.root
	cmd.Stdin = strings.NewReader(strings.Join(order, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git cat-file: %v", err)
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	used := map[string]bool{}
	commits := 0
	for _, tok := range order {
		if !sc.Scan() {
			t.Fatal("git cat-file gave fewer answers than questions")
		}
		answer := sc.Text()
		if _, external := externalCommits[tok]; external {
			used[tok] = true
			continue
		}
		switch {
		case answer == "commit":
			commits++
		case strings.HasSuffix(answer, " ambiguous"):
			t.Errorf("%s is ambiguous here, more than one object starts with it: write more of the hash (%s)", tok, strings.Join(where[tok], ", "))
		case strings.HasSuffix(answer, " missing"):
			t.Errorf("%s is not a commit of this repository (%s)", tok, strings.Join(where[tok], ", "))
		default:
			t.Errorf("%s names a %s, not a commit (%s)", tok, answer, strings.Join(where[tok], ", "))
		}
	}
	for tok := range externalCommits {
		if !used[tok] {
			t.Errorf("external commit %s is no longer named anywhere: remove it from the list", tok)
		}
	}
	t.Logf("%d distinct commit references, %d commits of this repository, %d of other repositories", len(order), commits, len(used))
}

// externalCommits are commits of other repositories that the documents name on purpose.
var externalCommits = map[string]string{
	"61ae26af":     "Foundry (anvil and forge 1.8.0), the build the chain tests and the binding were checked with",
	"cab19933":     "OpenZeppelin Contracts v5.7.0, the submodule commit the contract is compiled against",
	"ef5a25a6":     "the docker-nginx commit the Nitro test application is pinned to",
	"067c5c24cef2": "the Linux kernel commit whose initramfs buffer-format documentation tdx.md cites",
	"1ae0d03":      "the main branch of the public GitHub repository on 2026-11-15, recorded by the trial",
}

var commitToken = regexp.MustCompile("`([0-9a-f]{7,12})`")
