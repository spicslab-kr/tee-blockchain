//go:build linux

package aws

import (
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// AF_VSOCK 은 Go 의 net 패키지가 모르는 주소 체계라 소켓을 직접 다룬다.
const (
	afVsock      = 40
	vmaddrCIDAny = 0xffffffff
)

type sockaddrVM struct {
	family    uint16
	reserved1 uint16
	port      uint32
	cid       uint32
	zero      [4]byte
}

type vsockAddr struct{ cid, port uint32 }

func (a vsockAddr) Network() string { return "vsock" }
func (a vsockAddr) String() string {
	return strconv.FormatUint(uint64(a.cid), 10) + ":" + strconv.FormatUint(uint64(a.port), 10)
}

// ListenVsock 은 엔클레이브 안에서 vsock 포트를 연다. 부모 인스턴스가 여기로 이어 준다.
func ListenVsock(port uint32) (net.Listener, error) {
	fd, err := syscall.Socket(afVsock, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	sa := sockaddrVM{family: afVsock, port: port, cid: vmaddrCIDAny}
	if _, _, errno := syscall.Syscall(syscall.SYS_BIND, uintptr(fd), uintptr(unsafe.Pointer(&sa)), unsafe.Sizeof(sa)); errno != 0 {
		syscall.Close(fd)
		return nil, errno
	}
	if err := syscall.Listen(fd, 16); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return &vsockListener{f: os.NewFile(uintptr(fd), "vsock"), addr: vsockAddr{vmaddrCIDAny, port}}, nil
}

// DialVsock 은 부모 인스턴스에서 엔클레이브(cid)의 포트로 연결한다.
func DialVsock(cid, port uint32) (net.Conn, error) {
	fd, err := syscall.Socket(afVsock, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	sa := sockaddrVM{family: afVsock, port: port, cid: cid}
	if _, _, errno := syscall.Syscall(syscall.SYS_CONNECT, uintptr(fd), uintptr(unsafe.Pointer(&sa)), unsafe.Sizeof(sa)); errno != 0 {
		syscall.Close(fd)
		return nil, &net.OpError{Op: "dial", Net: "vsock", Addr: vsockAddr{cid, port}, Err: errno}
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return &vsockConn{f: os.NewFile(uintptr(fd), "vsock"), remote: vsockAddr{cid, port}}, nil
}

type vsockListener struct {
	f    *os.File
	addr vsockAddr
}

func (l *vsockListener) Accept() (net.Conn, error) {
	rc, err := l.f.SyscallConn()
	if err != nil {
		return nil, err
	}
	var nfd int
	var peer sockaddrVM
	var acceptErr error
	err = rc.Read(func(fd uintptr) bool {
		n := unsafe.Sizeof(peer)
		r, _, errno := syscall.Syscall6(syscall.SYS_ACCEPT4, fd, uintptr(unsafe.Pointer(&peer)), uintptr(unsafe.Pointer(&n)), syscall.SOCK_CLOEXEC|syscall.SOCK_NONBLOCK, 0, 0)
		if errno == syscall.EAGAIN {
			return false
		}
		if errno != 0 {
			acceptErr = errno
		}
		nfd = int(r)
		return true
	})
	if err != nil {
		if l.closed() {
			return nil, net.ErrClosed
		}
		return nil, err
	}
	if acceptErr != nil {
		return nil, acceptErr
	}
	return &vsockConn{f: os.NewFile(uintptr(nfd), "vsock"), local: l.addr, remote: vsockAddr{peer.cid, peer.port}}, nil
}

func (l *vsockListener) Close() error   { return l.f.Close() }
func (l *vsockListener) Addr() net.Addr { return l.addr }
func (l *vsockListener) closed() bool {
	_, err := l.f.Stat()
	return err != nil
}

// vsockConn 은 받은 연결을 net.Conn 으로 감싼다. 마감은 os.File 이 처리한다.
type vsockConn struct {
	f             *os.File
	local, remote vsockAddr
}

func (c *vsockConn) Read(b []byte) (int, error)         { return c.f.Read(b) }
func (c *vsockConn) Write(b []byte) (int, error)        { return c.f.Write(b) }
func (c *vsockConn) Close() error                       { return c.f.Close() }
func (c *vsockConn) LocalAddr() net.Addr                { return c.local }
func (c *vsockConn) RemoteAddr() net.Addr               { return c.remote }
func (c *vsockConn) SetDeadline(t time.Time) error      { return c.f.SetDeadline(t) }
func (c *vsockConn) SetReadDeadline(t time.Time) error  { return c.f.SetReadDeadline(t) }
func (c *vsockConn) SetWriteDeadline(t time.Time) error { return c.f.SetWriteDeadline(t) }

// CloseWrite 는 보내는 방향만 닫는다. 상대는 EOF 를 읽고, 우리는 남은 응답을 계속 받을 수 있다.
func (c *vsockConn) CloseWrite() error {
	rc, err := c.f.SyscallConn()
	if err != nil {
		return err
	}
	var serr error
	if err := rc.Control(func(fd uintptr) { serr = syscall.Shutdown(int(fd), syscall.SHUT_WR) }); err != nil {
		return err
	}
	return serr
}
