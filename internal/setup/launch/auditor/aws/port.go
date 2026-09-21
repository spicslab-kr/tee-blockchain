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

// OpenPort 는 이 인스턴스의 보안 그룹에 TCP port 인바운드를 연다. 인스턴스 메타데이터로 자기 ID 와 리전을 알아내고
// aws CLI 를 부른다. 인스턴스에 EC2 쓰기 권한(IAM 역할)이 없으면 ErrNoPermission 과 함께 손으로 칠 명령을 돌려준다.
func OpenPort(ctx context.Context, port int, log io.Writer) (command string, err error) {
	region, sg, err := identity(ctx)
	if err != nil {
		return "", fmt.Errorf("instance metadata: %w", err)
	}
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
		return command, err
	}
	var rules []any
	if json.Unmarshal(out, &rules) == nil && len(rules) > 0 {
		fmt.Fprintf(log, "보안 그룹 %s: TCP %d 이미 열려 있음\n", sg, port)
		return command, nil
	}
	if _, err := aws("ec2", "authorize-security-group-ingress", "--group-id", sg, "--protocol", "tcp", "--port", p, "--cidr", "0.0.0.0/0"); err != nil {
		return command, err
	}
	fmt.Fprintf(log, "보안 그룹 %s: TCP %d 인바운드 열었음 (0.0.0.0/0)\n", sg, port)
	return command, nil
}

var ErrNoPermission = errors.New("this instance has no permission to change its security group")

// identity — IMDSv2 로 리전과 첫 보안 그룹 ID. IAM 권한 없이 읽을 수 있다.
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
