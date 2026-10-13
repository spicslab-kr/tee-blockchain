package user

import (
	"encoding/binary"
	"fmt"
	"io"
)

// request:  "TBRA" · version(1) · nonce(32)
// response: "TBRA" · version(1) · status(1) · platform len(1) · platform · doc len(4, big-endian) · doc
//        status 0 = ok, 1 = error (the doc field carries the message)

const (
	wireVersion = 1
	MaxDocSize  = 1 << 20 // 1 MiB; RA documents are a few KB, JWTs tens of KB
)

var magic = [4]byte{'T', 'B', 'R', 'A'}

func writeRequest(w io.Writer, nonce Nonce) error {
	if err := nonce.Validate(); err != nil {
		return err
	}
	buf := make([]byte, 0, 5+NonceSize)
	buf = append(buf, magic[:]...)
	buf = append(buf, wireVersion)
	buf = append(buf, nonce...)
	_, err := w.Write(buf)
	return err
}

func readRequest(r io.Reader) (Nonce, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	if [4]byte(hdr[:4]) != magic || hdr[4] != wireVersion {
		return nil, fmt.Errorf("%w: bad magic or version", ErrBadRequest)
	}
	n := make([]byte, NonceSize)
	if _, err := io.ReadFull(r, n); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	return Nonce(n), nil
}

func writeResponse(w io.Writer, doc RADoc, extractErr error) error {
	status := byte(0)
	body := doc.Raw
	if extractErr != nil {
		status = 1
		body = []byte(extractErr.Error())
	}
	if len(body) > MaxDocSize {
		return ErrTooLarge
	}
	if len(doc.Format) > 255 {
		return fmt.Errorf("format name too long")
	}
	buf := make([]byte, 0, 7+len(doc.Format)+4+len(body))
	buf = append(buf, magic[:]...)
	buf = append(buf, wireVersion, status, byte(len(doc.Format)))
	buf = append(buf, doc.Format...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(body)))
	buf = append(buf, body...)
	_, err := w.Write(buf)
	return err
}

func readResponse(r io.Reader) (RADoc, error) {
	var hdr [7]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return RADoc{}, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	if [4]byte(hdr[:4]) != magic || hdr[4] != wireVersion {
		return RADoc{}, fmt.Errorf("%w: bad magic or version", ErrBadResponse)
	}
	status := hdr[5]
	plat := make([]byte, hdr[6])
	if _, err := io.ReadFull(r, plat); err != nil {
		return RADoc{}, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	var lenb [4]byte
	if _, err := io.ReadFull(r, lenb[:]); err != nil {
		return RADoc{}, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	n := binary.BigEndian.Uint32(lenb[:])
	if n > MaxDocSize {
		return RADoc{}, fmt.Errorf("%w: %d bytes", ErrTooLarge, n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return RADoc{}, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	if status != 0 {
		return RADoc{}, fmt.Errorf("%w: %s", ErrExtractFailed, body)
	}
	return RADoc{Format: string(plat), Raw: body}, nil
}
