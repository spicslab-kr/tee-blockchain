package aws

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Risk is a pattern found in one Dockerfile instruction that is known to break reproducible builds.
type Risk struct {
	Line int    // first line of the instruction (1-based)
	Text string // the instruction with whitespace collapsed, cut to 110 bytes
	Why  string // why building twice can give different results
}

// riskPatterns are causes of non-reproducibility known from measurement or the literature. Normalization already handles apk virtual
// package versions, scratch directories, package manager logs and build caches, so none of them has a pattern here. The check is
// static and can say "may differ"; --verify (building twice) shows whether it does. A message containing "without" is dropped when
// the instruction shows a version pin (see versionPinned), so every such message must keep that word. The first entry applies to FROM
// lines and all others to the remaining lines.
var riskPatterns = []struct {
	re  *regexp.Regexp
	why string
}{
	{regexp.MustCompile(`(?i)^FROM\s+(?:--platform=\S+\s+)?([^\s@]+)(?:\s+AS\s+\S+)?\s*$`),
		"base image referenced by tag: the tag can move between the Vendor's build and the Auditor's (pin it with FROM img@sha256:…)"},
	{regexp.MustCompile(`\bdate\b`), "the build time is read here; if it ends up in a file, every build differs"},
	{regexp.MustCompile(`\b(uname\s+-n|hostname)\b`), "the build host's name is read here; if it ends up in a file, every host differs"},
	{regexp.MustCompile(`\$RANDOM|/dev/u?random|\bopenssl\s+rand\b|\buuidgen\b|\bssh-keygen\b|\bopenssl\s+(genrsa|ecparam|req)\b`),
		"random bytes or a key are generated at build time, so two builds cannot give the same value"},
	{regexp.MustCompile(`\bmktemp\b[^;&|]*(?:-p\s*|--tmpdir=)(?:/(?:[^tv\s]|t[^m]|tm[^p]|v[^a]|va[^r])\S*|/var/(?:[^t\s]|t[^m]|tm[^p])\S*)`),
		"mktemp writes outside /tmp and /var/tmp, so its random-named file stays in the image (normalization empties those two directories)"},
	{regexp.MustCompile(`\bgit\s+clone\b`), "source cloned at build time: pin it with a commit checkout, or the two builds may see different heads"},
	{regexp.MustCompile(`\b(curl|wget)\b[^|&;]*\|\s*(sh|bash)\b`), "a script is downloaded and run unverified, so its content can change between builds"},
	{regexp.MustCompile(`\b(pip3?|npm|yarn|pnpm|gem|cargo|go)\s+(install|add|get)\b`), "dependencies resolved at build time: without a lockfile or exact versions the resolution can change"},
	{regexp.MustCompile(`\b(apt-get|apt)\s+install\b`), "apt packages without =version resolve to whatever the mirror has today"},
	{regexp.MustCompile(`\b(dnf|yum|microdnf)\s+install\b`), "rpm packages without -version resolve to whatever the mirror has today"},
	{regexp.MustCompile(`\bapk\s+add\b`), "apk packages without =version resolve to whatever the repository has today"},
	{regexp.MustCompile(`(\./configure\b|\bmake\b|\bgcc\b|\bcc\b|\bcmake\b|\bcargo\s+build\b|\bgo\s+build\b)`),
		"compiled from source: compilers and build scripts may embed the time, host or path (redis bakes hostname+time into redis-server). Run with --verify"},
}

// versionPinned matches a visible version pin (name=1.2 or name-1.2). One pin anywhere on an install line drops that line's "without" warning, even when other packages on it are unpinned.
var versionPinned = regexp.MustCompile(`\S+=\S*\d|\S+-\d+\.\d+\S*`)

// LintDockerfile scans the Dockerfile instruction by instruction for riskPatterns. Lines continued with a backslash are joined into one instruction, and comment lines are skipped.
func LintDockerfile(path string) ([]Risk, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return lintReader(f)
}

// lintReader is LintDockerfile on any reader. A FROM line is checked against the base image rule alone (a digest pin or scratch passes); every other line is checked against all other rules.
func lintReader(rd io.Reader) ([]Risk, error) {
	var risks []Risk
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var (
		logical string
		start   int
		n       int
	)
	flush := func() {
		if strings.TrimSpace(logical) == "" {
			logical = ""
			return
		}
		text := strings.TrimSpace(logical)
		logical = ""
		if strings.HasPrefix(text, "#") {
			return
		}
		for _, p := range riskPatterns {
			if !p.re.MatchString(text) {
				continue
			}
			if strings.HasPrefix(text, "FROM ") || strings.HasPrefix(text, "from ") {
				if p.re != riskPatterns[0].re {
					continue
				}
				if m := riskPatterns[0].re.FindStringSubmatch(text); m != nil && m[1] == "scratch" {
					continue
				}
			} else if p.re == riskPatterns[0].re {
				continue
			}
			if strings.Contains(p.why, "without") && versionPinned.MatchString(text) {
				continue
			}
			risks = append(risks, Risk{Line: start, Text: shorten(text, 110), Why: p.why})
		}
	}
	for sc.Scan() {
		n++
		line := sc.Text()
		if logical == "" {
			start = n
		}
		if head, ok := strings.CutSuffix(strings.TrimRight(line, " \t"), "\\"); ok {
			logical += head + " "
			continue
		}
		logical += line
		flush()
	}
	flush()
	return risks, sc.Err()
}

// shorten collapses whitespace in s and, when longer than n bytes, cuts it to n-1 bytes followed by "…".
func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// printRisks writes the findings to the build log as warnings and the build continues: a static scan says that two builds may differ, and --verify shows whether they do.
func printRisks(log io.Writer, dockerfile string, risks []Risk) {
	if len(risks) == 0 {
		fmt.Fprintf(log, "reproducibility scan of %s: nothing known to break it\n", dockerfile)
		return
	}
	fmt.Fprintf(log, "reproducibility scan of %s: %d thing(s) that can make two builds differ\n", dockerfile, len(risks))
	for _, r := range risks {
		fmt.Fprintf(log, "  line %d: %s\n    %s\n", r.Line, r.Text, r.Why)
	}
	fmt.Fprintln(log, "  the build goes on. --verify builds a second time and shows whether any of this actually changed the image")
}
