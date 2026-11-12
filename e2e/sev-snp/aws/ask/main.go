// snp-ask-aws: User 쪽. 닫힌 AWS SEV-SNP VM 안 Ollama에 질문하고, 돌아온 답이 등록된 커널·initrd·부팅 설정으로
// 부팅한 VM 안에서 내 난수에 대해 만들어졌는지 확인한다.
//
//	snp-ask-aws -url http://<VM 주소>:8443 -expected expected-aws.json -prompt "..."
//	-save f.json      받은 답과 증명을 저장
//	-replay f.json    저장해 둔 답을 새 난수로 다시 검증 (재사용 시험)
//	-tamper           받은 답의 첫 글자를 바꾼 뒤 검증 (중간 변조 시험)
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/fxamacker/cbor/v2"
	spb "github.com/google/go-sev-guest/proto/sevsnp"
	"github.com/google/go-sev-guest/verify"
	"github.com/google/go-sev-guest/verify/trust"
	"github.com/veraison/go-cose"
	"google.golang.org/protobuf/proto"
)

// AWS Nitro 증명 루트 인증서 (https://aws-nitro-enclaves.amazonaws.com/AWS_NitroEnclaves_Root-G1.zip), 지문은 AWS 문서에 공개된 값
const awsRootFingerprint = "641a0321a3e244efe456463195d606317ed7cdcc3c1756e09893f3c68f79bb5b"
const awsRootPEM = `-----BEGIN CERTIFICATE-----
MIICETCCAZagAwIBAgIRAPkxdWgbkK/hHUbMtOTn+FYwCgYIKoZIzj0EAwMwSTEL
MAkGA1UEBhMCVVMxDzANBgNVBAoMBkFtYXpvbjEMMAoGA1UECwwDQVdTMRswGQYD
VQQDDBJhd3Mubml0cm8tZW5jbGF2ZXMwHhcNMTkxMDI4MTMyODA1WhcNNDkxMDI4
MTQyODA1WjBJMQswCQYDVQQGEwJVUzEPMA0GA1UECgwGQW1hem9uMQwwCgYDVQQL
DANBV1MxGzAZBgNVBAMMEmF3cy5uaXRyby1lbmNsYXZlczB2MBAGByqGSM49AgEG
BSuBBAAiA2IABPwCVOumCMHzaHDimtqQvkY4MpJzbolL//Zy2YlES1BR5TSksfbb
48C8WBoyt7F2Bw7eEtaaP+ohG2bnUs990d0JX28TcPQXCEPZ3BABIeTPYwEoCWZE
h8l5YoQwTcU/9KNCMEAwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUkCW1DdkF
R+eWw5b6cp3PmanfS5YwDgYDVR0PAQH/BAQDAgGGMAoGCCqGSM49BAMDA2kAMGYC
MQCjfy+Rocm9Xue4YnwWmNJVA44fA0P5W2OpYow9OYCVRaEevL8uO1XYru5xtMPW
rfMCMQCi85sWBbJwKKXdS6BptQFuZbT73o/gBh1qUxl/nNr12UO8Yfwr6wPLb+6N
IwLz3/Y=
-----END CERTIFICATE-----
`

const digestTag = "attestamp-ollama-v1"

func digest(nonce []byte, prompt, response string) []byte {
	p, r := sha256.Sum256([]byte(prompt)), sha256.Sum256([]byte(response))
	h := sha256.New()
	h.Write([]byte(digestTag))
	h.Write(nonce)
	h.Write(p[:])
	h.Write(r[:])
	return h.Sum(nil)
}

var failures int

func check(ok bool, name, detail string) {
	mark := "[통과]"
	if !ok {
		mark, failures = "[실패]", failures+1
	}
	if detail != "" {
		fmt.Printf("  %s %s  (%s)\n", mark, name, detail)
	} else {
		fmt.Printf("  %s %s\n", mark, name)
	}
}

func short(b []byte) string {
	h := hex.EncodeToString(b)
	if len(h) > 24 {
		return h[:16] + "…"
	}
	return h
}

type nitroDoc struct {
	ModuleID    string            `cbor:"module_id"`
	PCRs        map[uint64][]byte `cbor:"nitrotpm_pcrs"`
	Certificate []byte            `cbor:"certificate"`
	CABundle    [][]byte          `cbor:"cabundle"`
	Nonce       []byte            `cbor:"nonce"`
}

func verifyNitroDoc(raw, want []byte) *nitroDoc {
	var msg cose.Sign1Message
	if err := msg.UnmarshalCBOR(raw); err != nil {
		if err := msg.UnmarshalCBOR(append([]byte{0xd2}, raw...)); err != nil {
			check(false, "NitroTPM 문서 읽기", err.Error())
			return nil
		}
	}
	var d nitroDoc
	if err := cbor.Unmarshal(msg.Payload, &d); err != nil {
		check(false, "문서 내용 읽기", err.Error())
		return nil
	}
	block, _ := pem.Decode([]byte(awsRootPEM))
	fp := sha256.Sum256(block.Bytes)
	root, _ := x509.ParseCertificate(block.Bytes)
	roots, inter := x509.NewCertPool(), x509.NewCertPool()
	roots.AddCert(root)
	for _, c := range d.CABundle {
		if ic, err := x509.ParseCertificate(c); err == nil {
			inter.AddCert(ic)
		}
	}
	leaf, err := x509.ParseCertificate(d.Certificate)
	if err != nil {
		check(false, "서명 인증서 읽기", err.Error())
		return nil
	}
	_, cerr := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	check(cerr == nil && hex.EncodeToString(fp[:]) == awsRootFingerprint, "인증서 체인이 AWS 루트까지 이어짐", d.ModuleID)
	v, err := cose.NewVerifier(cose.AlgorithmES384, leaf.PublicKey)
	if err == nil {
		err = msg.Verify(nil, v)
	}
	check(err == nil, "NitroTPM 문서 서명 (ES384)", "")
	check(bytes.Equal(d.Nonce, want), "문서 안의 값 = 내 난수 + 질문 + 받은 답의 해시", short(d.Nonce))
	return &d
}

// TCG 이벤트 로그(crypto-agile)에서 PCR 8, 9의 SHA-384 기록만 꺼낸다.
type event struct {
	pcr, typ uint32
	d384     []byte
}

func parseLog(b []byte) ([]event, error) {
	le := binary.LittleEndian
	if len(b) < 32 {
		return nil, fmt.Errorf("로그가 너무 짧음")
	}
	// 첫 기록: 옛 형식(SHA-1 20바이트) 안에 알고리즘 목록이 있다
	sz := int(le.Uint32(b[28:32]))
	spec := b[32 : 32+sz]
	if !bytes.HasPrefix(spec, []byte("Spec ID Event03")) {
		return nil, fmt.Errorf("crypto-agile 로그가 아님")
	}
	n := int(le.Uint32(spec[24:28]))
	sizes := map[uint16]int{}
	for i := 0; i < n; i++ {
		o := 28 + 4*i
		sizes[le.Uint16(spec[o:])] = int(le.Uint16(spec[o+2:]))
	}
	var evs []event
	p := 32 + sz
	for p+12 <= len(b) {
		pcr, typ, cnt := le.Uint32(b[p:]), le.Uint32(b[p+4:]), int(le.Uint32(b[p+8:]))
		if pcr == 0xffffffff || (pcr == 0 && typ == 0 && cnt == 0) {
			break
		}
		q := p + 12
		e := event{pcr: pcr, typ: typ}
		for i := 0; i < cnt; i++ {
			alg := le.Uint16(b[q:])
			s, ok := sizes[alg]
			if !ok {
				return nil, fmt.Errorf("모르는 해시 0x%x", alg)
			}
			if alg == 0x000c {
				e.d384 = b[q+2 : q+2+s]
			}
			q += 2 + s
		}
		es := int(le.Uint32(b[q:]))
		p = q + 4 + es
		evs = append(evs, e)
	}
	return evs, nil
}

func fetch(url string) ([]byte, error) {
	c := http.Client{Timeout: 20 * time.Second}
	r, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

type answer struct {
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
	Doc      []byte `json:"nitrotpm_doc"`
	EventLog []byte `json:"eventlog"`
	SNP      []byte `json:"snp"`
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8443", "닫힌 VM의 주소")
	expPath := flag.String("expected", "expected-aws.json", "등록된 기대값 (kernel, initrd, cmdline의 sha384)")
	prompt := flag.String("prompt", "What is the capital of France? Answer in one word.", "질문")
	save := flag.String("save", "", "받은 답과 증명을 저장")
	replay := flag.String("replay", "", "저장한 답을 새 난수로 다시 검증")
	tamper := flag.Bool("tamper", false, "받은 답의 첫 글자를 바꾼 뒤 검증")
	status := flag.Bool("status", false, "질문 대신 VM 안 상태 보고서(프로세스, 포트, 마운트, 커널 설정)를 받아 검증")
	nonceHex := flag.String("nonce", "", "(-replay와 함께) 저장한 답을 이 난수로 검증. 비우면 새 난수")
	flag.Parse()

	var exp struct{ Kernel, Initrd, Cmdline string }
	if b, err := os.ReadFile(*expPath); err != nil || json.Unmarshal(b, &exp) != nil {
		fmt.Println("기대값 파일을 읽을 수 없음:", *expPath)
		os.Exit(2)
	}
	nonce := make([]byte, 32)
	rand.Read(nonce)
	if *nonceHex != "" {
		nonce, _ = hex.DecodeString(*nonceHex)
	}
	var a answer
	if *replay != "" {
		b, _ := os.ReadFile(*replay)
		json.Unmarshal(b, &a)
		fmt.Printf("(VM에 묻지 않고, 저장해 둔 답 %s 을 새 난수로 제출)\n", *replay)
	} else {
		body, _ := json.Marshal(map[string]string{"nonce": hex.EncodeToString(nonce), "prompt": *prompt})
		t0 := time.Now()
		path := "/ask"
		if *status {
			path = "/status"
		}
		r, err := http.Post(*url+path, "application/json", bytes.NewReader(body))
		if err != nil {
			fmt.Println("요청 실패:", err)
			os.Exit(1)
		}
		raw, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != 200 || json.Unmarshal(raw, &a) != nil {
			fmt.Println("응답 오류:", r.Status, string(raw))
			os.Exit(1)
		}
		if *status {
			fmt.Printf("상태 보고서 받음: %d 바이트 (%.1f초)\n", len(a.Response), time.Since(t0).Seconds())
		} else {
			fmt.Printf("질문: %s\n답: %q  (%.1f초)\n", a.Prompt, a.Response, time.Since(t0).Seconds())
		}
		if *save != "" {
			os.WriteFile(*save, raw, 0o644)
		}
	}
	if *tamper {
		a.Response = "X" + a.Response[1:]
		fmt.Printf("(받은 답을 %q 로 바꿔서 검증)\n", a.Response)
	}

	fmt.Println("\n검증: NitroTPM 증명 (AWS 서명)")
	d := verifyNitroDoc(a.Doc, digest(nonce, a.Prompt, a.Response))
	have := map[uint32]map[string]bool{8: {}, 9: {}}
	if d != nil {
		evs, err := parseLog(a.EventLog)
		if err != nil {
			check(false, "이벤트 로그 읽기", err.Error())
		}
		state := map[uint32][]byte{8: make([]byte, 48), 9: make([]byte, 48)}
		for _, e := range evs {
			if (e.pcr != 8 && e.pcr != 9) || e.typ == 3 || e.d384 == nil {
				continue
			}
			h := sha512.Sum384(append(append([]byte{}, state[e.pcr]...), e.d384...))
			state[e.pcr] = h[:]
			have[e.pcr][hex.EncodeToString(e.d384)] = true
		}
		for _, p := range []uint32{8, 9} {
			check(bytes.Equal(state[p], d.PCRs[uint64(p)]), fmt.Sprintf("이벤트 로그 재생 = 서명된 PCR%d", p), "")
		}
	}
	fmt.Println("검증: 등록된 이미지로 부팅했는가")
	check(have[9][exp.Kernel], "커널 해시 = 등록값 (PCR9)", exp.Kernel[:16]+"…")
	check(have[9][exp.Initrd], "initrd 해시 = 등록값 (PCR9, Ollama·모델·응답기 전부 포함)", exp.Initrd[:16]+"…")
	check(have[8][exp.Cmdline], "부팅 설정 해시 = 등록값 (PCR8, 디스크로 안 넘어가는 rdinit 포함)", exp.Cmdline[:16]+"…")

	fmt.Println("검증: SEV-SNP 보고서 (AMD 서명)")
	snp := &spb.Attestation{}
	if err := proto.Unmarshal(a.SNP, snp); err != nil {
		check(false, "SNP 보고서 읽기", err.Error())
	} else {
		sum := sha512.Sum512(a.Doc)
		check(bytes.Equal(snp.GetReport().GetReportData(), sum[:]), "Report Data = SHA-512(NitroTPM 문서)", short(snp.GetReport().GetReportData()))
		opts := verify.DefaultOptions()
		label := "AMD 서명 (ARK → ASK → VCEK → 보고서)"
		if len(snp.GetCertificateChain().GetVlekCert()) > 0 {
			label = "AMD 서명 (ARK → ASVK → VLEK → 보고서)"
			chain, err := fetch("https://kdsintf.amd.com/vlek/v1/Milan/cert_chain")
			root := trust.AMDRootCertsProduct("Milan")
			if err == nil {
				err = root.FromKDSCertBytes(chain)
			}
			if err != nil {
				check(false, "AMD VLEK 체인 받기", err.Error())
			}
			opts.TrustedRoots = map[string][]*trust.AMDRootCerts{"Milan": {root}}
		}
		err := verify.SnpAttestation(snp, opts)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		check(err == nil, label, msg)
		check(snp.GetReport().GetPolicy()&(1<<19) == 0, "디버그 허용 안 됨 (SNP 정책)", "")
	}
	if *status {
		check(a.Prompt == "attestamp-status", "이 항목은 상태 보고서임 (질문 칸 = attestamp-status)", "")
	}
	if failures == 0 && *status {
		fmt.Println("\n=== 판정: 아래 상태 보고서는 등록된 이미지로 부팅한 SEV-SNP VM 안의 코드가 내 난수에 대해 지금 만든 것이다 ===")
		printStatus(a.Response)
		return
	}
	if failures == 0 {
		fmt.Println("\n=== 판정: 이 답은 등록된 이미지로 부팅한 SEV-SNP VM 안에서, 내 질문에 대해 지금 만들어졌다 ===")
	} else {
		fmt.Printf("\n=== 판정: 거부 (실패 %d건) ===\n", failures)
		os.Exit(1)
	}
}
