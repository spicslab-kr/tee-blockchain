package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// The RA port on the parent instance's security group.
//
// launch never widens the security group on its own. Without a CIDR it only reports which sources the group allows on
// the port now (when the instance may read its group) and prints the command that would open it. It opens the port
// only when asked with an explicit CIDR, and to everyone (0.0.0.0/0) only when that is the CIDR written. Whatever it
// opens, it logs: which port, on which group, to which source.
//
// The evidence the responder hands out is not secret, but nobody needs it from the whole internet: the usual source is
// the Auditor's or a User's address (/32). docs/build/aws-nitro.md, "Opening the RA port".

// ErrNoPermission means the instance's role may not read or change its security group.
var ErrNoPermission = errors.New("this instance has no permission to read or change its security group")

// ErrNotIPv4 means a CIDR that authorize-security-group-ingress --cidr does not take.
var ErrNotIPv4 = errors.New("only IPv4 CIDRs can be opened from here")

// CIDRPlaceholder stands in for the source in the printed command when no CIDR was given.
const CIDRPlaceholder = "<verifier-ip>/32"

// ParseCIDR checks the source the port is to be opened to and returns it in canonical form. It must be a full IPv4
// CIDR: a bare address, an address with host bits set (10.0.0.5/24) and IPv6 are refused, so the source that gets
// opened is exactly the one written.
func ParseCIDR(s string) (string, error) {
	ip, n, err := net.ParseCIDR(strings.TrimSpace(s))
	if err != nil {
		if net.ParseIP(strings.TrimSpace(s)) != nil {
			return "", fmt.Errorf("%q is an address, not a CIDR: write %s/32 for that one address", s, strings.TrimSpace(s))
		}
		return "", fmt.Errorf("%q is not a CIDR: write e.g. 203.0.113.7/32 (one address) or 0.0.0.0/0 (everyone)", s)
	}
	if ip.To4() == nil {
		return "", fmt.Errorf("%q: %w; add an IPv6 rule in the console", s, ErrNotIPv4)
	}
	if !ip.Equal(n.IP) {
		return "", fmt.Errorf("%q has host bits set: did you mean %s?", s, n.String())
	}
	return n.String(), nil
}

// OpenCommand is the aws command that opens port to cidr on group in region. Empty region or group become
// placeholders, and an empty cidr becomes CIDRPlaceholder.
func OpenCommand(region, group string, port int, cidr string) string {
	if region == "" {
		region = "<region>"
	}
	if group == "" {
		group = "<group id>"
	}
	if cidr == "" {
		cidr = CIDRPlaceholder
	}
	return fmt.Sprintf("aws ec2 authorize-security-group-ingress --region %s --group-id %s --protocol tcp --port %d --cidr %s", region, group, port, cidr)
}

// Port is what is known about the RA port on this instance's security group. Sources is nil when the group could not
// be read.
type Port struct {
	Region, Group string
	Number        int
	Sources       []string // IPv4 CIDRs that a rule allows on Number (tcp covering it, or all traffic)
}

// awsCLI and instanceIdentity are variables so tests can stand in for the aws CLI and the instance metadata.
var (
	awsCLI           = runAWS
	instanceIdentity = identity
)

// ReadPort reads, and never changes, which sources the security group allows on port.
func ReadPort(ctx context.Context, port int) (Port, error) {
	region, sg, err := instanceIdentity(ctx)
	if err != nil {
		return Port{Number: port}, fmt.Errorf("instance metadata: %w", err)
	}
	p := Port{Region: region, Group: sg, Number: port}
	n := strconv.Itoa(port)
	out, err := awsCLI(ctx, region, "ec2", "describe-security-groups", "--group-ids", sg, "--query",
		"SecurityGroups[0].IpPermissions[?(IpProtocol=='tcp' && FromPort<=`"+n+"` && ToPort>=`"+n+"`) || IpProtocol=='-1'].IpRanges[].CidrIp")
	if err != nil {
		return p, err
	}
	var cidrs []string
	if err := json.Unmarshal(out, &cidrs); err != nil {
		return p, fmt.Errorf("describe-security-groups: %w", err)
	}
	p.Sources = append([]string{}, cidrs...)
	return p, nil
}

// OpenPort opens port to cidr on this instance's security group, unless a rule already allows exactly that source,
// and logs what it opened and to whom. cidr is checked with ParseCIDR first; there is no default. It returns the
// command to run by hand and the group ID (empty if the metadata could not be read), and ErrNoPermission when the
// instance's role may not change the group.
func OpenPort(ctx context.Context, port int, cidr string, log io.Writer) (command, group string, err error) {
	cidr, err = ParseCIDR(cidr)
	if err != nil {
		return "", "", err
	}
	p, err := ReadPort(ctx, port)
	command = OpenCommand(p.Region, p.Group, port, cidr)
	if err != nil {
		return command, p.Group, err
	}
	for _, s := range p.Sources {
		if s == cidr {
			fmt.Fprintf(log, "security group %s: inbound TCP %d from %s is already allowed; nothing changed\n", p.Group, port, cidr)
			return command, p.Group, nil
		}
	}
	if _, err := awsCLI(ctx, p.Region, "ec2", "authorize-security-group-ingress", "--group-id", p.Group,
		"--protocol", "tcp", "--port", strconv.Itoa(port), "--cidr", cidr); err != nil {
		return command, p.Group, err
	}
	fmt.Fprintf(log, "OPENED security group %s (%s): inbound TCP %d from %s, at %s\n", p.Group, p.Region, port, cidr,
		time.Now().UTC().Format(time.RFC3339))
	if cidr == "0.0.0.0/0" {
		fmt.Fprintf(log, "  that is everyone on the internet, as asked. To narrow it later: aws ec2 revoke-security-group-ingress --region %s --group-id %s --protocol tcp --port %d --cidr 0.0.0.0/0\n", p.Region, p.Group, port)
	}
	return command, p.Group, nil
}

// ReportPort is what launch does after the enclave answers. With an empty cidr it changes nothing: it says which
// sources the group allows now (if it may read the group) and prints the command that would open the port. With a
// cidr it opens the port to exactly that source, or prints the command when it may not.
func ReportPort(ctx context.Context, port int, cidr string, log io.Writer) {
	fmt.Fprintf(log, "Security group: inbound TCP %d\n", port)
	if cidr == "" {
		p, err := ReadPort(ctx, port)
		switch {
		case err != nil:
			fmt.Fprintf(log, "  could not read this instance's security group (%v)\n", err)
		case len(p.Sources) == 0:
			fmt.Fprintf(log, "  security group %s allows TCP %d from nowhere yet\n", p.Group, port)
		default:
			fmt.Fprintf(log, "  security group %s already allows TCP %d from: %s\n", p.Group, port, strings.Join(p.Sources, ", "))
		}
		fmt.Fprintf(log, `
Nothing was opened: launch changes the security group only when given --open-port-to <CIDR>.
To let a verifier reach the enclave, open the port to that verifier's address, for example:
  %s
or launch again with --open-port-to <verifier-ip>/32. --open-port-to 0.0.0.0/0 opens it to everyone.
`, OpenCommand(p.Region, p.Group, port, ""))
		return
	}
	command, group, err := OpenPort(ctx, port, cidr, log)
	if err == nil {
		fmt.Fprintf(log, "✓ the enclave is up and TCP %d is reachable from %s:  att-user verify --tee-addr <this instance's public IP>:%d --rpc … --registry …\n", port, cidr, port)
		return
	}
	why := "This instance has no permission to change it. From an AWS account that does, run:"
	if !errors.Is(err, ErrNoPermission) {
		why = fmt.Sprintf("Could not open it from here (%v). From an AWS account that can, run:", err)
	}
	if group == "" {
		group = "<this instance's security group>"
	}
	fmt.Fprintf(log, `
Inbound TCP %d was NOT opened.
%s
  %s
or open it in the console: EC2 → Security Groups → %s → Inbound rules → Custom TCP %d, source %s.
`, port, why, command, group, port, cidr)
}

// runAWS runs the aws CLI in region with JSON output. Permission and credential errors become ErrNoPermission.
func runAWS(ctx context.Context, region string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "aws", append([]string{"--region", region, "--output", "json"}, args...)...)
	out, err := c.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := strings.TrimSpace(string(ee.Stderr))
			if strings.Contains(msg, "UnauthorizedOperation") || strings.Contains(msg, "Unable to locate credentials") || strings.Contains(msg, "AccessDenied") {
				return nil, ErrNoPermission
			}
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return out, nil
}

// identity returns the region and first security group ID via IMDSv2. It can be read without IAM permissions.
func identity(ctx context.Context) (region, sg string, err error) {
	client := &http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	token, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	get := func(path string) (string, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/"+path, nil)
		req.Header.Set("X-aws-ec2-metadata-token", string(token))
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("%s: %s", path, resp.Status)
		}
		return strings.TrimSpace(string(b)), nil
	}
	doc, err := get("dynamic/instance-identity/document")
	if err != nil {
		return "", "", err
	}
	var ident struct {
		Region string `json:"region"`
	}
	if json.Unmarshal([]byte(doc), &ident) != nil || ident.Region == "" {
		return "", "", errors.New("not on an EC2 instance")
	}
	mac, err := get("meta-data/mac")
	if err != nil {
		return "", "", err
	}
	sgs, err := get("meta-data/network/interfaces/macs/" + mac + "/security-group-ids")
	if err != nil {
		return "", "", err
	}
	sg, _, _ = strings.Cut(sgs, "\n")
	if sg == "" {
		return "", "", errors.New("security group not found")
	}
	return ident.Region, sg, nil
}
