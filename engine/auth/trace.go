package auth

import (
	"net"
	"sync"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

type traceConn struct {
	net.Conn
	recorder *protocoltrace.Recorder
	mu       sync.Mutex
	active   bool
	opcode   byte
	in       []byte
	out      []byte
}

func (c *traceConn) begin(opcode byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = true
	c.opcode = opcode
	c.in = c.in[:0]
	c.out = c.out[:0]
}

func (c *traceConn) end() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active {
		return
	}
	c.recorder.Record(protocoltrace.ClientToServer, uint32(c.opcode), c.in, "auth")
	if len(c.out) > 0 {
		c.recorder.Record(protocoltrace.ServerToClient, uint32(c.out[0]), c.out[1:], "auth")
	}
	c.active = false
	c.in = c.in[:0]
	c.out = c.out[:0]
}

func (c *traceConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.mu.Lock()
		if c.active {
			c.in = append(c.in, p[:n]...)
		}
		c.mu.Unlock()
	}
	return n, err
}

func (c *traceConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.mu.Lock()
		if c.active {
			c.out = append(c.out, p[:n]...)
		}
		c.mu.Unlock()
	}
	return n, err
}
