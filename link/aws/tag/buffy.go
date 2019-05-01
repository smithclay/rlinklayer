package tag

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type Buffy struct {
	off        int
	cap        int
	encodedBuf []byte
}

var ErrOverCapacity = errors.New("Buffy: over capacity")

func NewBuffy(cap int) *Buffy {
	return &Buffy{off: 0, cap: cap, encodedBuf: make([]byte, 0, cap)}
}

func (b *Buffy) String() string {
	return fmt.Sprintf("Buffer (cap: %d, len: %d)",
		cap(b.encodedBuf), len(b.encodedBuf))
}

func (b *Buffy) WriteUnencoded(p []byte) (n int, err error) {
	encodedBytes := make([]byte, base64.StdEncoding.EncodedLen(len(p)))
	base64.StdEncoding.Encode(encodedBytes, p)
	return b.Write(encodedBytes)
}

func (b *Buffy) DecodedBytes() ([]byte, error) {
	// read all
	encodedBytes := make([]byte, len(b.encodedBuf))
	n, err := b.Read(encodedBytes)
	if err != nil {
		return nil, err
	}

	decodedBytes := make([]byte, base64.StdEncoding.DecodedLen(n))
	m, err := base64.StdEncoding.Decode(decodedBytes, encodedBytes[:n])
	if err != nil {
		return nil, err
	}
	return decodedBytes[:m], nil
}

func (b *Buffy) resize(p int) (int, error) {
	m := len(b.encodedBuf)
	if p <= b.cap-m {
		b.encodedBuf = b.encodedBuf[:m+p]
		return m + p, nil
	} else {
		return 0, ErrOverCapacity
	}
}

func (b *Buffy) Write(p []byte) (n int, err error) {
	m := len(b.encodedBuf)
	_, err = b.resize(len(p))
	if err != nil {
		return 0, ErrOverCapacity
	}

	return copy(b.encodedBuf[m:], p), nil
}

func (b *Buffy) empty() bool {
	return len(b.encodedBuf) <= b.off
}

func (b *Buffy) EncodedBytes() []byte {
	return b.encodedBuf
}

func (b *Buffy) EncodedBytesString() string {
	return string(b.encodedBuf)
}

func (b *Buffy) Reset() {
	b.encodedBuf = b.encodedBuf[:0]
	b.off = 0
}

func (b *Buffy) Replace(p []byte) (bool, error) {
	if bytes.Equal(p, b.encodedBuf) {
		return false, nil
	}
	b.Reset()
	_, err := b.resize(len(p))
	if err != nil {
		return false, err
	}
	copy(b.encodedBuf, p)
	return true, nil
}

func (b *Buffy) Len() int {
	return len(b.encodedBuf) - b.off
}

func (b *Buffy) Offset() int {
	return b.off
}

func (b *Buffy) Read(p []byte) (n int, err error) {
	if b.empty() {
		b.Reset()
		if len(p) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}
	n = copy(p, b.encodedBuf[b.off:])
	b.off += n
	return n, nil
}
