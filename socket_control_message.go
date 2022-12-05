// {{{ Copyright (c) Paul R. Tagliamonte <paultag@gmail.com>, 2020-2022
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE. }}}

package socketcontrol

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// SendFd will send a file descriptor over the provided UnixConn. This allows
// the peer to recieve the fd (using socketcontrol.RecieveFd), and communicate
// without relying on this program to continue to relay messages.
func SendFd(sock *net.UnixConn, fd uintptr) error {
	sockFd, err := sock.File()
	if err != nil {
		return err
	}

	rights := syscall.UnixRights(int(fd))
	return syscall.Sendmsg(int(sockFd.Fd()), nil, rights, nil, 0)

}

// RecieveFd will accept a file descriptor sent by the peer over the provided
// UnixConn. If the peer exits or otherwise ignores the fd, the recieved fd
// will still be usable and allow for communication.
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

// SendFile will pull the underlying *os.File OS File Descriptor and pass
// the file over the provided UnixConn to a peer.
func SendFile(sock *net.UnixConn, fd *os.File) error {
	return SendFd(sock, fd.Fd())
}

// RecieveFile will accept a file descriptor from the peer and return
// an *os.File created from that descriptor.
func RecieveFile(sock *net.UnixConn) (*os.File, error) {
	fd, err := RecieveFd(sock)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "<socketcontrol>"), nil
}

// SendConn will send the underlying net.Conn OS File Descriptor, and pass
// the connection over the provided UnixConn to a peer.
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

// RecieveConn will accept the file descriptor sent by a peer, and return
// a wrapped net.Conn wrapping that provided connection.
func RecieveConn(sock *net.UnixConn) (net.Conn, error) {
	fd, err := RecieveFile(sock)
	if err != nil {
		return nil, err
	}
	return net.FileConn(fd)
}

// vim: foldmethod=marker
