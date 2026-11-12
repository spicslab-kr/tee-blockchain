// snp-ask: User 쪽. 닫힌 SEV-SNP VM 안 Ollama에 질문하고, 돌아온 답이 등록된 커널·initrd·부팅 설정으로
// 부팅한 VM 안에서 내 난수에 대해 만들어졌는지 확인한다.
//
//	snp-ask -url http://<VM 주소>:8443 -expected expected.json -prompt "..."
//	-save f.json      받은 답과 증명을 저장
//	-replay f.json    저장해 둔 답을 새 난수로 다시 검증 (재사용 시험)
//	-tamper           받은 답의 첫 글자를 바꾼 뒤 검증 (중간 변조 시험)
package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	goattest "github.com/google/go-attestation/attest"
	spb "github.com/google/go-sev-guest/proto/sevsnp"
	"github.com/google/go-sev-guest/verify"
	"github.com/google/go-tpm-tools/proto/attest"
	"github.com/google/go-tpm-tools/server"
	"google.golang.org/protobuf/proto"
)

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

func fetchURL(url string) []byte {
	c := http.Client{Timeout: 20 * time.Second}
	r, err := c.Get(url)
	if err != nil {
		return nil
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return b
}

func verifyAKChain(att *attest.Attestation) *x509.Certificate {
	cert, err := x509.ParseCertificate(att.GetAkCert())
	if err != nil {
		check(false, "AK 인증서 읽기", err.Error())
		return nil
	}
	inter, roots := x509.NewCertPool(), x509.NewCertPool()
	cur, rootName := cert, ""
	for i := 0; i < 5 && len(cur.IssuingCertificateURL) > 0; i++ {
		c, err := x509.ParseCertificate(fetchURL(cur.IssuingCertificateURL[0]))
		if err != nil {
			break
		}
		if c.CheckSignatureFrom(c) == nil {
			roots.AddCert(c)
			rootName = c.Subject.CommonName
		} else {
			inter.AddCert(c)
		}
		cur = c
	}
	_, err = cert.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	check(err == nil, "vTPM AK 인증서 체인이 Google 루트까지 이어짐", rootName)
	if err != nil {
		return nil
	}
	return cert
}

func quotePCRs(att *attest.Attestation) map[uint32][]byte {
	for _, q := range att.GetQuotes() {
		if q.GetPcrs().GetHash().String() == "SHA256" {
			return q.GetPcrs().GetPcrs()
		}
	}
	return nil
}

func replayLog(att *attest.Attestation, pcrs map[uint32][]byte) map[int]map[string]bool {
	have := map[int]map[string]bool{8: {}, 9: {}}
	el, err := goattest.ParseEventLog(att.GetEventLog())
	if err != nil {
		check(false, "이벤트 로그 읽기", err.Error())
		return have
	}
	state := map[int][]byte{8: make([]byte, 32), 9: make([]byte, 32)}
	for _, e := range el.Events(goattest.HashSHA256) {
		if e.Index != 8 && e.Index != 9 {
			continue
		}
		h := sha256.Sum256(append(append([]byte{}, state[e.Index]...), e.Digest...))
		state[e.Index] = h[:]
		have[e.Index][hex.EncodeToString(e.Digest)] = true
	}
	for _, p := range []int{8, 9} {
		check(bytes.Equal(state[p], pcrs[uint32(p)]), fmt.Sprintf("이벤트 로그 재생 = 서명된 PCR%d", p), "")
	}
	return have
}

type answer struct {
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
	VTPM     []byte `json:"vtpm"`
	SNP      []byte `json:"snp"`
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8443", "닫힌 VM의 주소")
	expPath := flag.String("expected", "expected.json", "등록된 기대값 (kernel, initrd, cmdline의 sha256)")
	prompt := flag.String("prompt", "What is the capital of France? Answer in one word.", "질문")
	save := flag.String("save", "", "받은 답과 증명을 저장")
	replay := flag.String("replay", "", "저장한 답을 새 난수로 다시 검증")
	tamper := flag.Bool("tamper", false, "받은 답의 첫 글자를 바꾼 뒤 검증")
	status := flag.Bool("status", false, "질문 대신 VM 안 상태 보고서(프로세스, 포트, 마운트, 커널 설정)를 받아 검증")
	flag.Parse()

	var exp struct{ Kernel, Initrd, Cmdline string }
	if b, err := os.ReadFile(*expPath); err != nil || json.Unmarshal(b, &exp) != nil {
		fmt.Println("기대값 파일을 읽을 수 없음:", *expPath)
		os.Exit(2)
	}

	nonce := make([]byte, 32)
	rand.Read(nonce)
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

	att, snp := &attest.Attestation{}, &spb.Attestation{}
	if proto.Unmarshal(a.VTPM, att) != nil || proto.Unmarshal(a.SNP, snp) != nil {
		fmt.Println("증명 읽기 실패")
		os.Exit(1)
	}

	fmt.Println("\n검증: vTPM 증명 (Google 서명)")
	d := digest(nonce, a.Prompt, a.Response)
	if cert := verifyAKChain(att); cert != nil {
		clean := proto.Clone(att).(*attest.Attestation)
		clean.EventLog = nil
		_, err := server.VerifyAttestation(clean, server.VerifyOpts{Nonce: d, TrustedAKs: []crypto.PublicKey{cert.PublicKey}})
		msg := ""
		if err != nil {
			msg = err.Error()
			if len(msg) > 80 {
				msg = msg[:80] + "…"
			}
		}
		check(err == nil, "vTPM 서명 + 그 안의 값 = 내 난수 + 질문 + 받은 답의 해시", msg)
	}
	pcrs := quotePCRs(att)
	have := replayLog(att, pcrs)
	fmt.Println("검증: 등록된 이미지로 부팅했는가")
	check(have[9][exp.Kernel], "커널 해시 = 등록값 (PCR9)", exp.Kernel[:16]+"…")
	check(have[9][exp.Initrd], "initrd 해시 = 등록값 (PCR9, Ollama·모델·응답기 전부 포함)", exp.Initrd[:16]+"…")
	check(have[8][exp.Cmdline], "부팅 설정 해시 = 등록값 (PCR8, 디스크로 안 넘어가는 rdinit 포함)", exp.Cmdline[:16]+"…")
	fmt.Println("검증: SEV-SNP 보고서 (AMD 서명)")
	sum := sha512.Sum512(a.VTPM)
	check(bytes.Equal(snp.GetReport().GetReportData(), sum[:]), "Report Data = SHA-512(vTPM 증명)", short(snp.GetReport().GetReportData()))
	err := verify.SnpAttestation(snp, verify.DefaultOptions())
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	check(err == nil, "AMD 서명 (ARK → ASK → VCEK → 보고서)", msg)
	check(snp.GetReport().GetPolicy()&(1<<19) == 0, "디버그 허용 안 됨 (SNP 정책)", "")
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
