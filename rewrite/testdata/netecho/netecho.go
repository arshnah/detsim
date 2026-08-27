package netecho

import "net"

func Serve(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		handle(conn)
	}()
	return nil
}

func handle(conn net.Conn) {
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}
	conn.Write(buf[:n])
}

func Echo(addr string, msg []byte) ([]byte, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	buf := make([]byte, len(msg))
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func RemoteAddrOf(addr string) (net.Addr, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return conn.RemoteAddr(), nil
}
