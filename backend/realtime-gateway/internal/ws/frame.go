package ws

import (
	"encoding/binary"
	"io"
)

func readFrame(r io.Reader) (opcode byte, payload []byte, err error) {
	h := make([]byte, 2)
	if _, e := io.ReadFull(r, h); e != nil {
		return 0, nil, e
	}
	op := h[0] & 0x0f
	masked := h[1]&0x80 != 0
	n := int(h[1] & 0x7f)
	if n == 126 {
		b := make([]byte, 2)
		if _, e := io.ReadFull(r, b); e != nil {
			return 0, nil, e
		}
		n = int(binary.BigEndian.Uint16(b))
	}
	if n == 127 {
		b := make([]byte, 8)
		if _, e := io.ReadFull(r, b); e != nil {
			return 0, nil, e
		}
		n = int(binary.BigEndian.Uint64(b))
	}
	mask := make([]byte, 4)
	if masked {
		if _, e := io.ReadFull(r, mask); e != nil {
			return 0, nil, e
		}
	}
	p := make([]byte, n)
	if _, e := io.ReadFull(r, p); e != nil {
		return 0, nil, e
	}
	if masked {
		for i := range p {
			p[i] ^= mask[i%4]
		}
	}
	return op, p, nil
}

func writeFrame(w io.Writer, op byte, p []byte) error {
	h := []byte{0x80 | op}
	n := len(p)
	switch {
	case n < 126:
		h = append(h, byte(n))
	case n <= 65535:
		h = append(h, 126, byte(n>>8), byte(n))
	default:
		h = append(h, 127, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	if _, e := w.Write(h); e != nil {
		return e
	}
	_, e := w.Write(p)
	return e
}
