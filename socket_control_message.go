package socketcontrol

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

func SendFd(sock *net.UnixConn, fd uintptr) error {
	// TODO(paultag): support sending multiple

	sockFd, err := sock.File()
	if err != nil {
		return err
	}

	rights := syscall.UnixRights(int(fd))
	return syscall.Sendmsg(int(sockFd.Fd()), nil, rights, nil, 0)

}

func RecieveFd(sock *net.UnixConn) (uintptr, error) {
	sockFd, err := sock.File()
	if err != nil {
		return 0, err
	}

	// TODO(paultag): This 4 should be numEntries * 4, but we only support
	// one at a time for now.
	buf := make([]byte, syscall.CmsgSpace(4))
	_, _, _, _, err = syscall.Recvmsg(int(sockFd.Fd()), nil, buf, 0)
	if err != nil {
		return 0, err
	}

	var msgs []syscall.SocketControlMessage
	msgs, err = syscall.ParseSocketControlMessage(buf)
	if err != nil {
		return 0, err
	}

	for _, msg := range msgs {
		fds, err := syscall.ParseUnixRights(&msg)
		if err != nil {
			return 0, err
		}
		for _, fd := range fds {
			return uintptr(fd), nil
		}
	}

	return 0, fmt.Errorf("socketcontrol: no fds sent")
}

func SendFile(sock *net.UnixConn, fd *os.File) error {
	return SendFd(sock, fd.Fd())
}

func RecieveFile(sock *net.UnixConn) (*os.File, error) {
	fd, err := RecieveFd(sock)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "<socketcontrol>"), nil
}

func SendConn(sock *net.UnixConn, conn net.Conn) error {
	syscallable, ok := conn.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	if !ok {
		return fmt.Errorf("socketcontrol: can't get net.Conn fd")
	}

	syscallConn, err := syscallable.SyscallConn()
	if err != nil {
		return err
	}

	if err := syscallConn.Control(func(fd uintptr) {
		err = SendFd(sock, fd)
	}); err != nil {
		return err
	}
	// return err since SendFd above can set outside it's closure.
	return err
}

func RecieveConn(sock *net.UnixConn) (net.Conn, error) {
	fd, err := RecieveFile(sock)
	if err != nil {
		return nil, err
	}
	return net.FileConn(fd)
}
