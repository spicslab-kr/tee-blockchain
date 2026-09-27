package aws

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Risk — Dockerfile 한 줄에서 찾은, 재현빌드를 깨는 것으로 알려진 패턴.
type Risk struct {
	Line int
	Text string // 해당 줄 (앞뒤 공백 제거)
	Why  string // 왜 두 번 빌드하면 달라질 수 있는지
}

// riskPatterns — 실측·문헌으로 아는 비재현 원인. 정규화가 이미 처리하는 것(apk 가상 패키지 버전, /tmp, 패키지 관리자 로그)은
// 여기 없다. 이건 정적 검사라 "달라질 수 있다"까지만 안다 — 정말 달라지는지는 --verify(두 번 빌드)가 안다.
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

// versionPinned — 패키지 설치 줄에 버전 고정이 하나라도 보이면 그 경고는 뺀다(전부 고정했는지까지는 안 본다).
var versionPinned = regexp.MustCompile(`\S+=\S*\d|\S+-\d+\.\d+\S*`)

// LintDockerfile — Dockerfile 을 줄 단위로 훑어 riskPatterns 를 찾는다. 이어지는 줄(\)은 한 줄로 합친다. 주석은 뺀다.
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
					continue // FROM 줄에는 베이스 이미지 검사만
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

// printRisks — 빌드 로그에 경고로 찍는다. 빌드는 계속한다: 정적 검사는 "달라질 수 있다"지 "달라진다"가 아니다.
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
