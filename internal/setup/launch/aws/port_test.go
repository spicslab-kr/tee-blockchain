package aws

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseCIDR(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7/32": "203.0.113.7/32",
		" 10.1.0.0/16 ":  "10.1.0.0/16",
		"0.0.0.0/0":      "0.0.0.0/0",
	} {
		if got, err := ParseCIDR(in); err != nil || got != want {
			t.Errorf("ParseCIDR(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for in, mention := range map[string]string{
		"":               "not a CIDR",
		"203.0.113.7":    "203.0.113.7/32", // a bare address: suggests /32, never widens it
		"10.0.0.5/24":    "10.0.0.0/24",    // host bits: asks, does not guess
		"2001:db8::/32":  "IPv4",
		"everyone":       "not a CIDR",
		"300.1.1.1/32":   "not a CIDR",
		"203.0.113.7/33": "not a CIDR",
	} {
		_, err := ParseCIDR(in)
		if err == nil || !strings.Contains(err.Error(), mention) {
			t.Errorf("ParseCIDR(%q) = %v; want an error mentioning %q", in, err, mention)
		}
	}
}

func TestOpenCommand(t *testing.T) {
	if got := OpenCommand("eu-north-1", "sg-1", 5000, "203.0.113.7/32"); got != "aws ec2 authorize-security-group-ingress --region eu-north-1 --group-id sg-1 --protocol tcp --port 5000 --cidr 203.0.113.7/32" {
		t.Errorf("got %q", got)
	}
	got := OpenCommand("", "", 5000, "")
	for _, s := range []string{"<region>", "<group id>", "--cidr " + CIDRPlaceholder} {
		if !strings.Contains(got, s) {
			t.Errorf("%q must contain %q", got, s)
		}
	}
	if strings.Contains(got, "0.0.0.0/0") {
		t.Errorf("the placeholder command must not suggest 0.0.0.0/0: %q", got)
	}
}

// fakeAWS stands in for the instance metadata and the aws CLI, recording every aws call.
type fakeAWS struct {
	sources    string // JSON answer of describe-security-groups
	denyRead   bool
	denyChange bool
	calls      []string
}

func (f *fakeAWS) install(t *testing.T) {
	t.Helper()
	oldCLI, oldID := awsCLI, instanceIdentity
	t.Cleanup(func() { awsCLI, instanceIdentity = oldCLI, oldID })
	instanceIdentity = func(context.Context) (string, string, error) { return "eu-north-1", "sg-test", nil }
	awsCLI = func(_ context.Context, region string, args ...string) ([]byte, error) {
		f.calls = append(f.calls, strings.Join(args, " "))
		switch args[1] {
		case "describe-security-groups":
			if f.denyRead {
				return nil, ErrNoPermission
			}
			return []byte(f.sources), nil
		case "authorize-security-group-ingress":
			if f.denyChange {
				return nil, ErrNoPermission
			}
			return []byte("{}"), nil
		}
		return nil, errors.New("unexpected aws call: " + strings.Join(args, " "))
	}
}

func (f *fakeAWS) authorized() []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "ec2 authorize-security-group-ingress") {
			out = append(out, c)
		}
	}
	return out
}

// Without a CIDR, launch only reads and reports: no authorize call, and the printed command has a placeholder source.
func TestReportPort_WithoutCIDROpensNothing(t *testing.T) {
	f := &fakeAWS{sources: `["198.51.100.4/32"]`}
	f.install(t)
	var log bytes.Buffer
	ReportPort(context.Background(), 5000, "", &log)
	if got := f.authorized(); len(got) != 0 {
		t.Fatalf("nothing may be opened without a CIDR, but: %v", got)
	}
	for _, s := range []string{"already allows TCP 5000 from: 198.51.100.4/32", "Nothing was opened", "--cidr " + CIDRPlaceholder, "--open-port-to"} {
		if !strings.Contains(log.String(), s) {
			t.Errorf("output must contain %q:\n%s", s, log.String())
		}
	}
}

// Even when the group cannot be read, the default changes nothing and still prints the command.
func TestReportPort_WithoutCIDRAndNoPermission(t *testing.T) {
	f := &fakeAWS{denyRead: true}
	f.install(t)
	var log bytes.Buffer
	ReportPort(context.Background(), 5000, "", &log)
	if len(f.authorized()) != 0 || !strings.Contains(log.String(), "--group-id sg-test") || !strings.Contains(log.String(), "Nothing was opened") {
		t.Fatalf("unexpected:\n%s\ncalls %v", log.String(), f.calls)
	}
}

// With a CIDR it opens exactly that source and logs what it opened, on which group.
func TestOpenPort_OpensExactlyTheGivenSourceAndLogsIt(t *testing.T) {
	f := &fakeAWS{sources: `[]`}
	f.install(t)
	var log bytes.Buffer
	if _, group, err := OpenPort(context.Background(), 5000, "203.0.113.7/32", &log); err != nil || group != "sg-test" {
		t.Fatalf("OpenPort: %v %q", err, group)
	}
	got := f.authorized()
	if len(got) != 1 || !strings.HasSuffix(got[0], "--cidr 203.0.113.7/32") || !strings.Contains(got[0], "--port 5000") {
		t.Fatalf("authorize calls %v", got)
	}
	if !strings.Contains(log.String(), "OPENED security group sg-test (eu-north-1): inbound TCP 5000 from 203.0.113.7/32") {
		t.Errorf("the log must say what was opened to whom:\n%s", log.String())
	}
	if strings.Contains(log.String(), "everyone") {
		t.Errorf("a /32 is not everyone:\n%s", log.String())
	}
}

// 0.0.0.0/0 is opened only when it is the CIDR written, and the log says it is everyone and how to undo it.
func TestOpenPort_EveryoneOnlyWhenWritten(t *testing.T) {
	f := &fakeAWS{sources: `[]`}
	f.install(t)
	var log bytes.Buffer
	if _, _, err := OpenPort(context.Background(), 5000, "0.0.0.0/0", &log); err != nil {
		t.Fatal(err)
	}
	if got := f.authorized(); len(got) != 1 || !strings.HasSuffix(got[0], "--cidr 0.0.0.0/0") {
		t.Fatalf("authorize calls %v", got)
	}
	if !strings.Contains(log.String(), "everyone") || !strings.Contains(log.String(), "revoke-security-group-ingress") {
		t.Errorf("opening to everyone must be said, with the way back:\n%s", log.String())
	}
}

// A source already allowed is not added again; a different, wider rule does not count as the one asked for.
func TestOpenPort_AlreadyAllowedSourceIsLeftAlone(t *testing.T) {
	f := &fakeAWS{sources: `["0.0.0.0/0","203.0.113.7/32"]`}
	f.install(t)
	var log bytes.Buffer
	if _, _, err := OpenPort(context.Background(), 5000, "203.0.113.7/32", &log); err != nil {
		t.Fatal(err)
	}
	if got := f.authorized(); len(got) != 0 || !strings.Contains(log.String(), "already allowed; nothing changed") {
		t.Fatalf("authorize calls %v\n%s", got, log.String())
	}
	f2 := &fakeAWS{sources: `["0.0.0.0/0"]`}
	f2.install(t)
	if _, _, err := OpenPort(context.Background(), 5000, "203.0.113.7/32", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := f2.authorized(); len(got) != 1 {
		t.Fatalf("the /32 rule must be added even when 0.0.0.0/0 exists: %v", got)
	}
}

// A bad CIDR is refused before anything is read or changed.
func TestOpenPort_BadCIDRTouchesNothing(t *testing.T) {
	f := &fakeAWS{sources: `[]`}
	f.install(t)
	if _, _, err := OpenPort(context.Background(), 5000, "203.0.113.7", &bytes.Buffer{}); err == nil {
		t.Fatal("a bare address must be refused")
	}
	if len(f.calls) != 0 {
		t.Fatalf("no aws call may happen for a bad CIDR: %v", f.calls)
	}
}

// Without permission to change the group, ReportPort says it was not opened and prints the command for that source.
func TestReportPort_NoPermissionPrintsCommandForTheGivenSource(t *testing.T) {
	f := &fakeAWS{sources: `[]`, denyChange: true}
	f.install(t)
	var log bytes.Buffer
	ReportPort(context.Background(), 5000, "203.0.113.7/32", &log)
	for _, s := range []string{"was NOT opened", "no permission", "--group-id sg-test --protocol tcp --port 5000 --cidr 203.0.113.7/32", "source 203.0.113.7/32"} {
		if !strings.Contains(log.String(), s) {
			t.Errorf("output must contain %q:\n%s", s, log.String())
		}
	}
}
