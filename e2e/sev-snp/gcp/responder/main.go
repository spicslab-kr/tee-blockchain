// snp-aproxy: 닫힌 SEV-SNP VM 안에서 Ollama를 띄우고, 질문마다 답과 함께 그 답을 묶은 증명을 돌려준다.
//
// d = SHA-256("attestamp-ollama-v1" || nonce || SHA-256(prompt) || SHA-256(response))
// vTPM 증명의 nonce = d, SNP 보고서의 Report Data = SHA-512(vTPM 증명 바이트)
// 그래서 받는 쪽은 "이 답은, 이 커널·initrd·부팅 설정으로 부팅한 SNP VM 안에서, 내 난수에 대해 만들어졌다"를 확인할 수 있다.
// 여는 경로는 /ask 하나뿐이다. Ollama의 모델 받기·바꾸기 API는 밖으로 내보내지 않는다.
package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/go-sev-guest/client"
	spb "github.com/google/go-sev-guest/proto/sevsnp"
	tpmclient "github.com/google/go-tpm-tools/client"
	"github.com/google/go-tpm-tools/proto/attest"
	legacytpm2 "github.com/google/go-tpm/legacy/tpm2"
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

// 닫힌 VM에서는 기본 경로를 쓰고, 일반 OS에서 미리 시험할 때만 환경 변수로 바꾼다.
func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func startOllama() error {
	bin, models := envOr("APROXY_OLLAMA", "/usr/bin/ollama"), envOr("APROXY_MODELS", "/models")
	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST=127.0.0.1:11434", "OLLAMA_MODELS="+models, "HOME=/root")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
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

func vtpmAttestation(nonce []byte) (*attest.Attestation, error) {
	rwc, err := legacytpm2.OpenTPM()
	if err != nil {
		return nil, err
	}
	defer rwc.Close()
	ak, err := tpmclient.GceAttestationKeyRSA(rwc)
	if err != nil {
		return nil, err
	}
	defer ak.Close()
	elog, err := tpmclient.GetEventLog(rwc)
	if err != nil {
		return nil, err
	}
	return ak.Attest(tpmclient.AttestOpts{Nonce: nonce, TCGEventLog: elog})
}

func snpReport(rd [64]byte) (*spb.Attestation, error) {
	d, err := client.OpenDevice()
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return client.GetExtendedReport(d, rd)
}

type askReq struct {
	Nonce  string `json:"nonce"`
	Prompt string `json:"prompt"`
}

type askResp struct {
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
	VTPM     []byte `json:"vtpm"`
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
	att, err := vtpmAttestation(digest(nonce, q.Prompt, resp))
	if err != nil {
		http.Error(w, "vtpm: "+err.Error(), http.StatusInternalServerError)
		return
	}
	raw, _ := proto.Marshal(att)
	snp, err := snpReport(sha512.Sum512(raw))
	if err != nil {
		http.Error(w, "snp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sb, _ := proto.Marshal(snp)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(askResp{Model: model, Prompt: q.Prompt, Response: resp, VTPM: raw, SNP: sb})
}

func main() {
	if os.Getpid() == 1 {
		lockKernel()
	}
	if err := loUp(); err != nil {
		log.Printf("lo up: %v", err)
	}
	if err := startOllama(); err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ask", handler(generate, ""))
	mux.HandleFunc("/status", handler(func(string) (string, error) { return snapshot() }, statusPrompt))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	log.Printf("snp-aproxy: model %s, listening %s", model, listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}
