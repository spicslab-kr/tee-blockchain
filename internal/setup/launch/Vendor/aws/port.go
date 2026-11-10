package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// OpenPort opens TCP port inbound on this instance's security group. It finds its own ID and region from the instance metadata
// and calls the aws CLI. If the instance has no EC2 write permission (IAM role), it returns ErrNoPermission along with the command to type by hand.
// group is that security group's ID, where to look when opening it in the console. If the metadata cannot be read it is empty and command is filled with a placeholder.
func OpenPort(ctx context.Context, port int, log io.Writer) (command, group string, err error) {
	region, sg, err := identity(ctx)
	if err != nil {
		command = fmt.Sprintf("aws ec2 authorize-security-group-ingress --region <region> --group-id <group id> --protocol tcp --port %d --cidr 0.0.0.0/0", port)
		return command, "", fmt.Errorf("instance metadata: %w", err)
	}
	group = sg
	command = fmt.Sprintf("aws ec2 authorize-security-group-ingress --region %s --group-id %s --protocol tcp --port %d --cidr 0.0.0.0/0", region, sg, port)
	aws := func(args ...string) ([]byte, error) {
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
	p := strconv.Itoa(port)
	out, err := aws("ec2", "describe-security-groups", "--group-ids", sg,
		"--query", "SecurityGroups[0].IpPermissions[?IpProtocol=='tcp' && FromPort<=`"+p+"` && ToPort>=`"+p+"`]")
	if err != nil {
		return command, group, err
	}
	var rules []any
	if json.Unmarshal(out, &rules) == nil && len(rules) > 0 {
		fmt.Fprintf(log, "security group %s: inbound TCP %d already open\n", sg, port)
		return command, group, nil
	}
	if _, err := aws("ec2", "authorize-security-group-ingress", "--group-id", sg, "--protocol", "tcp", "--port", p, "--cidr", "0.0.0.0/0"); err != nil {
		return command, group, err
	}
	fmt.Fprintf(log, "opened inbound TCP %d on %s (0.0.0.0/0)\n", port, sg)
	return command, group, nil
}

var ErrNoPermission = errors.New("this instance has no permission to change its security group")

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
