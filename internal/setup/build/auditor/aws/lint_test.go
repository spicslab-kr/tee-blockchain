package aws

import (
	"strings"
	"testing"
)

func TestLintDockerfile(t *testing.T) {
	df := `FROM alpine:3.22
# a comment mentioning date and make must not count
RUN apk add --no-cache \
      nginx=1.28.0-r0 \
      curl
RUN set -eux; \
    wget https://x/y.tar.gz; \
    ./configure; make -j; make install
RUN echo "$(date) $(uname -n)" > /etc/build-info
RUN openssl rand -hex 16 > /etc/secret
RUN mktemp
RUN git clone https://github.com/x/y
RUN curl -fsSL https://get.example.com | sh
ENV X=1
`
	risks, err := lintReader(strings.NewReader(df))
	if err != nil {
		t.Fatal(err)
	}
	got := map[int][]string{}
	for _, r := range risks {
		got[r.Line] = append(got[r.Line], r.Why)
	}
	has := func(line int, sub string) bool {
		for _, w := range got[line] {
			if strings.Contains(w, sub) {
				return true
			}
		}
		return false
	}
	if !has(1, "by tag") {
		t.Errorf("FROM by tag not flagged: %v", got[1])
	}
	if len(got[2]) != 0 {
		t.Errorf("comment flagged: %v", got[2])
	}
	if has(3, "apk packages") {
		t.Errorf("apk add with a pinned version flagged: %v", got[3])
	}
	if !has(6, "compiled from source") {
		t.Errorf("configure/make not flagged: %v", got[6])
	}
	if !has(9, "build time") || !has(9, "host's name") {
		t.Errorf("date/uname not flagged: %v", got[9])
	}
	if !has(10, "random bytes") {
		t.Errorf("openssl rand not flagged: %v", got[10])
	}
	if has(11, "mktemp") {
		t.Errorf("mktemp in /tmp flagged although normalization empties /tmp: %v", got[11])
	}
	if !has(12, "cloned at build time") || !has(13, "unverified") {
		t.Errorf("git clone / curl|sh not flagged: %v %v", got[12], got[13])
	}
	if len(got[14]) != 0 {
		t.Errorf("ENV flagged: %v", got[14])
	}
}

func TestLintDockerfile_Digest(t *testing.T) {
	risks, _ := lintReader(strings.NewReader("FROM alpine@sha256:0123 AS build\nFROM scratch\nRUN apk add --no-cache curl=8.0.0-r0\n"))
	if len(risks) != 0 {
		t.Fatalf("digest-pinned FROM / scratch / pinned apk flagged: %+v", risks)
	}
}
