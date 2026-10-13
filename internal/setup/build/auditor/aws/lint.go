package aws

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Risk is a pattern found on one Dockerfile line that is known to break reproducible builds.
type Risk struct {
	Line int
	Text string // the line (trimmed)
	Why  string // why building twice can give different results
}

// riskPatterns are causes of non-reproducibility known from measurement or the literature. Things normalization already handles
// (apk virtual package versions, /tmp, package manager logs) are not here. This is a static check, so it only knows "may differ";
// --verify (building twice) tells whether it really does.
var riskPatterns = []struct {
	re  *regexp.Regexp
	why string
}{
	{regexp.MustCompile(`(?i)^FROM\s+(?:--platform=\S+\s+)?([^\s@]+)(?:\s+AS\s+\S+)?\s*$`),
		"base image pinned by tag, not digest — the tag can move between the Vendor's build and the Auditor's (use FROM img@sha256:…)"},
	{regexp.MustCompile(`\bdate\b`), "the build time is read here; if it ends up in a file, every build differs"},
	{regexp.MustCompile(`\b(uname\s+-n|hostname)\b`), "the build host's name is read here; if it ends up in a file, every host differs"},
	{regexp.MustCompile(`\$RANDOM|/dev/u?random|\bopenssl\s+rand\b|\buuidgen\b|\bssh-keygen\b|\bopenssl\s+(genrsa|ecparam|req)\b`),
		"random bytes / a key are generated at build time — they cannot be the same twice"},
	{regexp.MustCompile(`\bmktemp\b[^;&|]*(?:-p\s*|--tmpdir=)(?:/(?:[^tv\s]|t[^m]|tm[^p]|v[^a]|va[^r])\S*|/var/(?:[^t\s]|t[^m]|tm[^p])\S*)`),
		"mktemp outside /tmp and /var/tmp leaves a random-named file that normalization does not empty"},
	{regexp.MustCompile(`\bgit\s+clone\b`), "source cloned at build time — pin it with a commit checkout, or the two builds may see different heads"},
	{regexp.MustCompile(`\b(curl|wget)\b[^|&;]*\|\s*(sh|bash)\b`), "a script is downloaded and run unverified — its content can change under you"},
	{regexp.MustCompile(`\b(pip3?|npm|yarn|pnpm|gem|cargo|go)\s+(install|add|get)\b`), "dependencies resolved at build time — without a lockfile / exact versions the resolution can change"},
	{regexp.MustCompile(`\b(apt-get|apt)\s+install\b`), "apt packages without =version resolve to whatever the mirror has today"},
	{regexp.MustCompile(`\b(dnf|yum|microdnf)\s+install\b`), "rpm packages without -version resolve to whatever the mirror has today"},
	{regexp.MustCompile(`\bapk\s+add\b`), "apk packages without =version resolve to whatever the repository has today"},
	{regexp.MustCompile(`(\./configure\b|\bmake\b|\bgcc\b|\bcc\b|\bcmake\b|\bcargo\s+build\b|\bgo\s+build\b)`),
		"compiled from source — compilers and build scripts may embed the time, host or path (redis bakes hostname+time into redis-server). Run with --verify"},
}

// versionPinned: if any version pin is visible on a package install line, that warning is dropped (it does not check that everything is pinned).
var versionPinned = regexp.MustCompile(`\S+=\S*\d|\S+-\d+\.\d+\S*`)

// LintDockerfile scans the Dockerfile line by line for riskPatterns. Continued lines (\) are joined into one. Comments are skipped.
func LintDockerfile(path string) ([]Risk, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return lintReader(f)
}

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
					continue // on a FROM line, only the base image check applies
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

func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// printRisks prints them as warnings in the build log. The build continues: a static check says "may differ", not "does differ".
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
