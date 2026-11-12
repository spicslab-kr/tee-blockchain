// snp-aproxy-aws: 닫힌 AWS SEV-SNP VM 안에서 Ollama를 띄우고, 질문마다 답과 함께 그 답을 묶은 증명을 돌려준다.
//
// d = SHA-256("attestamp-ollama-v1" || nonce || SHA-256(prompt) || SHA-256(response))
// NitroTPM 증명 문서의 nonce = d, SNP 보고서의 Report Data = SHA-512(NitroTPM 문서)
// 부팅 기록(이벤트 로그)도 같이 돌려줘서, 받는 쪽이 커널·initrd·부팅 설정 해시를 확인할 수 있게 한다.
// initramfs에 DHCP 도구가 없어서 네트워크는 이 프로그램이 직접 잡는다.
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/go-sev-guest/client"
	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpm2/transport"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

const (
	model      = "qwen2.5:0.5b"
	ollamaURL  = "http://127.0.0.1:11434"
	listenAddr = ":8443"
	digestTag  = "attestamp-ollama-v1"
)

var tpmMu sync.Mutex

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func loUp() error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	ifr, err := unix.NewIfreq("lo")
	if err != nil {
		return err
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFFLAGS, ifr); err != nil {
		return err
	}
	ifr.SetUint16(ifr.Uint16() | unix.IFF_UP | unix.IFF_RUNNING)
	return unix.IoctlIfreq(fd, unix.SIOCSIFFLAGS, ifr)
}

// 첫 번째 이더넷 장치에 DHCP로 주소와 기본 경로를 잡는다. 이미 주소가 있으면(일반 OS에서 시험할 때) 건너뛴다.
func dhcpUp() error {
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}
	var link netlink.Link
	for _, l := range links {
		if l.Attrs().Name != "lo" && l.Type() == "device" {
			link = l
			break
		}
	}
	if link == nil {
		return fmt.Errorf("no network device")
	}
	if addrs, _ := netlink.AddrList(link, netlink.FAMILY_V4); len(addrs) > 0 && os.Getenv("APROXY_DHCP_TEST") == "" {
		log.Printf("network: %s already has %s", link.Attrs().Name, addrs[0].IPNet)
		return nil
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return err
	}
	c, err := nclient4.New(link.Attrs().Name, nclient4.WithTimeout(10*time.Second), nclient4.WithRetry(6))
	if err != nil {
		return err
	}
	defer c.Close()
	lease, err := c.Request(context.Background())
	if err != nil {
		return err
	}
	ack := lease.ACK
	ipnet := &net.IPNet{IP: ack.YourIPAddr, Mask: ack.SubnetMask()}
	if os.Getenv("APROXY_DHCP_TEST") != "" {
		log.Printf("dhcp test: %s offered %s gateway %v", link.Attrs().Name, ipnet, ack.Router())
		return nil
	}
	if err := netlink.AddrAdd(link, &netlink.Addr{IPNet: ipnet}); err != nil {
		return err
	}
	if gws := ack.Router(); len(gws) > 0 {
		if err := netlink.RouteAdd(&netlink.Route{LinkIndex: link.Attrs().Index, Gw: gws[0]}); err != nil {
			return err
		}
		log.Printf("network: %s %s gateway %s", link.Attrs().Name, ipnet, gws[0])
	}
	return nil
}

func startOllama() error {
	cmd := exec.Command(envOr("APROXY_OLLAMA", "/usr/bin/ollama"), "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST=127.0.0.1:11434", "OLLAMA_MODELS="+envOr("APROXY_MODELS", "/models"), "HOME=/root")
	cmd.Stdout = io.MultiWriter(&logbuf, os.Stdout)
	cmd.Stderr = io.MultiWriter(&logbuf, os.Stderr)
	if err := cmd.Start(); err != nil {
		return err
	}
	for i := 0; i < 120; i++ {
		if r, err := http.Get(ollamaURL + "/api/version"); err == nil {
			r.Body.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ollama did not start")
}

func digest(nonce []byte, prompt, response string) []byte {
	p, r := sha256.Sum256([]byte(prompt)), sha256.Sum256([]byte(response))
	h := sha256.New()
	h.Write([]byte(digestTag))
	h.Write(nonce)
	h.Write(p[:])
	h.Write(r[:])
	return h.Sum(nil)
}

func generate(prompt string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": model, "prompt": prompt, "stream": false,
		"options": map[string]interface{}{"temperature": 0, "seed": 1},
	})
	r, err := http.Post(ollamaURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	var out struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", fmt.Errorf("ollama: %s", out.Error)
	}
	return out.Response, nil
}

// ───────── NitroTPM 증명 문서 (AWS nitro-tpm-attest 와 같은 순서, 시험 코드 go-verifier/aws 에서 가져옴) ─────────

func u16(v int) []byte    { b := make([]byte, 2); binary.BigEndian.PutUint16(b, uint16(v)); return b }
func u32(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
func rawTPM(t transport.TPM, tag uint16, cc uint32, body []byte) ([]byte, error) {
	cmd := append(append(u16(int(tag)), u32(uint32(10+len(body)))...), u32(cc)...)
	cmd = append(cmd, body...)
	rsp, err := t.Send(cmd)
	if err != nil {
		return nil, err
	}
	if len(rsp) < 10 {
		return nil, fmt.Errorf("TPM 응답이 너무 짧음")
	}
	if rc := binary.BigEndian.Uint32(rsp[6:10]); rc != 0 {
		return nil, fmt.Errorf("TPM 응답 코드 0x%x (명령 0x%x)", rc, cc)
	}
	return rsp[10:], nil
}

func nitroTPMDocument(nonce []byte) ([]byte, error) {
	tpm, err := transport.OpenTPM("/dev/tpm0")
	if err != nil {
		return nil, fmt.Errorf("TPM 열기: %w", err)
	}
	defer tpm.Close()
	cp, err := tpm2.CreatePrimary{PrimaryHandle: tpm2.TPMRHEndorsement, InPublic: tpm2.New2B(tpm2.RSAEKTemplate)}.Execute(tpm)
	if err != nil {
		return nil, fmt.Errorf("EK 만들기: %w", err)
	}
	defer tpm2.FlushContext{FlushHandle: cp.ObjectHandle}.Execute(tpm)
	ekPub, _ := cp.OutPublic.Contents()
	rsaDetail, _ := ekPub.Parameters.RSADetail()
	rsaUnique, _ := ekPub.Unique.RSA()
	ekKey, err := tpm2.RSAPub(rsaDetail, rsaUnique)
	if err != nil {
		return nil, err
	}
	auth := make([]byte, 64)
	rand.Read(auth)
	sess, closeSess, err := tpm2.HMACSession(tpm, tpm2.TPMAlgSHA512, 16, tpm2.Salted(cp.ObjectHandle, *ekPub), tpm2.AESEncryption(128, tpm2.EncryptIn))
	if err != nil {
		return nil, fmt.Errorf("세션 시작: %w", err)
	}
	var idx tpm2.TPMHandle
	defined := false
	for i := 0; i < 64 && !defined; i++ {
		idx = tpm2.TPMHandle(0x01000100 + i)
		_, err = tpm2.NVDefineSpace{
			AuthHandle: tpm2.TPMRHOwner,
			Auth:       tpm2.TPM2BAuth{Buffer: auth},
			PublicInfo: tpm2.New2B(tpm2.TPMSNVPublic{NVIndex: idx, NameAlg: tpm2.TPMAlgSHA512, Attributes: tpm2.TPMANV{AuthRead: true, AuthWrite: true, NoDA: true}, DataSize: 8192}),
		}.Execute(tpm, sess)
		if err == nil {
			defined = true
		}
	}
	closeSess()
	if !defined {
		return nil, fmt.Errorf("NV 칸 만들기: %w", err)
	}
	pubRsp, err := tpm2.NVReadPublic{NVIndex: idx}.Execute(tpm)
	if err != nil {
		return nil, err
	}
	named := tpm2.NamedHandle{Handle: idx, Name: pubRsp.NVName}
	defer func() { tpm2.NVUndefineSpace{AuthHandle: tpm2.TPMRHOwner, NVIndex: named}.Execute(tpm) }()
	req, _ := cbor.Marshal(map[string]interface{}{"Attestation": map[string]interface{}{"user_data": nil, "nonce": nonce, "public_key": nil}})
	if _, err = (tpm2.NVWrite{AuthHandle: tpm2.AuthHandle{Handle: idx, Name: pubRsp.NVName, Auth: tpm2.PasswordAuth(auth)}, NVIndex: named, Data: tpm2.TPM2BMaxNVBuffer{Buffer: req}, Offset: 0}).Execute(tpm); err != nil {
		return nil, fmt.Errorf("NSM 요청 쓰기: %w", err)
	}
	pubRsp, err = tpm2.NVReadPublic{NVIndex: idx}.Execute(tpm)
	if err != nil {
		return nil, err
	}
	nvName := pubRsp.NVName.Buffer
	named = tpm2.NamedHandle{Handle: idx, Name: pubRsp.NVName}

	nonceCaller := make([]byte, 64)
	rand.Read(nonceCaller)
	salt := make([]byte, 32)
	rand.Read(salt)
	encSalt, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, ekKey, salt, []byte("SECRET\x00"))
	if err != nil {
		return nil, err
	}
	body := append(u32(uint32(cp.ObjectHandle)), u32(0x40000007)...)
	body = append(body, u16(len(nonceCaller))...)
	body = append(body, nonceCaller...)
	body = append(body, u16(len(encSalt))...)
	body = append(body, encSalt...)
	body = append(body, 0x00)
	body = append(body, u16(0x0010)...)
	body = append(body, u16(0x000d)...)
	r, err := rawTPM(tpm, 0x8001, 0x176, body)
	if err != nil {
		return nil, err
	}
	sessHandle := binary.BigEndian.Uint32(r[:4])
	nl := int(binary.BigEndian.Uint16(r[4:6]))
	nonceTPM := r[6 : 6+nl]
	kh := hmac.New(sha512.New, salt)
	kh.Write(u32(1))
	kh.Write([]byte("ATH\x00"))
	kh.Write(nonceTPM)
	kh.Write(nonceCaller)
	kh.Write(u32(512))
	sessionKey := kh.Sum(nil)
	ch := sha512.New()
	ch.Write(u32(0x20000001))
	ch.Write(nvName)
	ch.Write(nvName)
	cpHash := ch.Sum(nil)
	nonce2 := make([]byte, 64)
	rand.Read(nonce2)
	attrs := byte(0x01)
	ah := hmac.New(sha512.New, append(append([]byte{}, sessionKey...), auth...))
	ah.Write(cpHash)
	ah.Write(nonce2)
	ah.Write(nonceTPM)
	ah.Write([]byte{attrs})
	authArea := append(u32(sessHandle), u16(len(nonce2))...)
	authArea = append(authArea, nonce2...)
	authArea = append(authArea, attrs)
	mac := ah.Sum(nil)
	authArea = append(authArea, u16(len(mac))...)
	authArea = append(authArea, mac...)
	vb := append(u32(uint32(idx)), u32(uint32(idx))...)
	vb = append(vb, u32(uint32(len(authArea)))...)
	vb = append(vb, authArea...)
	if _, err = rawTPM(tpm, 0x8002, 0x20000001, vb); err != nil {
		return nil, fmt.Errorf("AWS 전용 TPM 명령: %w", err)
	}
	rawTPM(tpm, 0x8001, 0x165, u32(sessHandle))
	var all []byte
	for off := 0; off < 8192; off += 1024 {
		rr, err := tpm2.NVRead{AuthHandle: tpm2.AuthHandle{Handle: idx, Name: pubRsp.NVName, Auth: tpm2.PasswordAuth(auth)}, NVIndex: named, Size: 1024, Offset: uint16(off)}.Execute(tpm)
		if err != nil {
			return nil, fmt.Errorf("NV 읽기: %w", err)
		}
		all = append(all, rr.Data.Buffer...)
	}
	var resp map[string]map[string][]byte
	if err := cbor.NewDecoder(bytes.NewReader(all)).Decode(&resp); err != nil {
		return nil, fmt.Errorf("응답 해석: %w", err)
	}
	doc := resp["Attestation"]["document"]
	if len(doc) == 0 {
		return nil, fmt.Errorf("응답에 문서가 없음")
	}
	return doc, nil
}

type askReq struct {
	Nonce  string `json:"nonce"`
	Prompt string `json:"prompt"`
}

type askResp struct {
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
	Doc      []byte `json:"nitrotpm_doc"`
	EventLog []byte `json:"eventlog"`
	SNP      []byte `json:"snp"`
}

func handler(gen func(string) (string, error), fixedPrompt string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { serve(w, r, gen, fixedPrompt) }
}

func serve(w http.ResponseWriter, r *http.Request, gen func(string) (string, error), fixedPrompt string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var q askReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&q); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	nonce, err := hex.DecodeString(q.Nonce)
	if err != nil || len(nonce) != 32 {
		http.Error(w, "nonce must be 32 bytes hex", http.StatusBadRequest)
		return
	}
	if fixedPrompt != "" {
		q.Prompt = fixedPrompt
	}
	resp, err := gen(q.Prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tpmMu.Lock()
	defer tpmMu.Unlock()
	doc, err := nitroTPMDocument(digest(nonce, q.Prompt, resp))
	if err != nil {
		http.Error(w, "nitrotpm: "+err.Error(), http.StatusInternalServerError)
		return
	}
	el, err := os.ReadFile("/sys/kernel/security/tpm0/binary_bios_measurements")
	if err != nil {
		http.Error(w, "eventlog: "+err.Error(), http.StatusInternalServerError)
		return
	}
	d, err := client.OpenDevice()
	if err != nil {
		http.Error(w, "snp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer d.Close()
	snp, err := client.GetExtendedReport(d, sha512.Sum512(doc))
	if err != nil {
		http.Error(w, "snp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sb, _ := proto.Marshal(snp)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(askResp{Model: model, Prompt: q.Prompt, Response: resp, Doc: doc, EventLog: el, SNP: sb})
}

// 닫힌 VM에서는 이 프로그램이 첫 프로세스(rdinit)다. 셸 없이 직접 시스템 폴더를 올리고 드라이버를 싣는다.
func initSystem() {
	os.Setenv("PATH", "/usr/sbin:/usr/bin:/sbin:/bin")
	for _, m := range []struct{ src, dst, fs string }{
		{"proc", "/proc", "proc"}, {"sysfs", "/sys", "sysfs"}, {"devtmpfs", "/dev", "devtmpfs"},
		{"securityfs", "/sys/kernel/security", "securityfs"}, {"tmpfs", "/run", "tmpfs"}, {"tmpfs", "/tmp", "tmpfs"},
	} {
		os.MkdirAll(m.dst, 0o755)
		if err := unix.Mount(m.src, m.dst, m.fs, 0, ""); err != nil && err != unix.EBUSY {
			log.Printf("mount %s: %v", m.dst, err)
		}
	}
	for _, mod := range []string{"ena", "sev-guest"} {
		if out, err := exec.Command("modprobe", mod).CombinedOutput(); err != nil {
			log.Printf("modprobe %s: %v %s", mod, err, out)
		}
	}
	for i := 0; i < 20; i++ {
		if _, err := os.Stat("/dev/sev-guest"); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("attestamp: closed boot (no disk, no shell, no ssh)")
}

// ───────── 시험 준비용 디버그 (부팅 설정에 attestamp_debuglog=1 이 있을 때만) ─────────
// 닫힌 VM은 화면을 볼 수 없어서, 시험 준비 단계에서만 기록을 디스크에 남기고 재부팅한다. 본 시험 이미지는 이 설정 없이 부팅한다.

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.b.Len() > 4<<20 {
		return len(p), nil
	}
	return l.b.Write(p)
}

var logbuf lockedBuf

func debugMode() bool {
	b, _ := os.ReadFile("/proc/cmdline")
	return bytes.Contains(b, []byte("attestamp_debuglog=1"))
}

func saveDebugAndReboot(extra []byte) {
	exec.Command("modprobe", "xfs").Run()
	os.MkdirAll("/mnt", 0o755)
	if err := unix.Mount("/dev/nvme0n1p1", "/mnt", "xfs", 0, ""); err == nil {
		k := make([]byte, 1<<20)
		n, _ := unix.Klogctl(3, k)
		logbuf.mu.Lock()
		out := append(append(append([]byte{}, logbuf.b.Bytes()...), "\n=== kernel log ===\n"...), k[:n]...)
		logbuf.mu.Unlock()
		os.WriteFile("/mnt/var/tmp/closed-debug.log", out, 0o644)
		if extra != nil {
			os.WriteFile("/mnt/var/tmp/closed-selftest.json", extra, 0o644)
		}
		unix.Sync()
		unix.Unmount("/mnt", 0)
	}
	unix.Sync()
	unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
}

func fatal(format string, a ...interface{}) {
	log.Printf(format, a...)
	if os.Getpid() == 1 {
		if debugMode() {
			saveDebugAndReboot(nil)
		}
		unix.Sync()
		unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
	}
	os.Exit(1)
}

func selfTest() {
	time.Sleep(2 * time.Second)
	nonce := make([]byte, 32)
	body, _ := json.Marshal(map[string]string{"nonce": hex.EncodeToString(nonce), "prompt": "What is the capital of France? Answer in one word."})
	r, err := http.Post("http://127.0.0.1"+listenAddr+"/ask", "application/json", bytes.NewReader(body))
	var raw []byte
	if err != nil {
		log.Printf("selftest: %v", err)
	} else {
		raw, _ = io.ReadAll(r.Body)
		r.Body.Close()
		log.Printf("selftest: %s, %d bytes", r.Status, len(raw))
	}
	saveDebugAndReboot(raw)
}

func main() {
	log.SetOutput(io.MultiWriter(&logbuf, os.Stderr))
	defer func() {
		if r := recover(); r != nil {
			fatal("panic: %v", r)
		}
	}()
	if os.Getpid() == 1 {
		initSystem()
		if !debugMode() {
			lockKernel() // 디버그 부팅은 기록을 디스크에 남겨야 해서 잠그지 않는다 (xfs는 커널에 내장이라 모듈은 필요 없지만, 본 시험과 같게 두려고 분리)
		}
	}
	if err := loUp(); err != nil {
		log.Printf("lo up: %v", err)
	}
	if err := dhcpUp(); err != nil {
		fatal("network: %v", err)
	}
	if err := startOllama(); err != nil {
		fatal("ollama: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ask", handler(generate, ""))
	mux.HandleFunc("/status", handler(func(string) (string, error) { return snapshot() }, statusPrompt))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	log.Printf("snp-aproxy-aws: model %s, listening %s", model, listenAddr)
	if os.Getpid() == 1 && debugMode() {
		go selfTest()
	}
	fatal("listen: %v", http.ListenAndServe(listenAddr, mux))
}
