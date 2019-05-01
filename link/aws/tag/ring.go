package tag

import (
	"container/ring"
	"errors"
	"fmt"
	"io"
	"log"
)

type TagRingType uint8

const (
	TransmitType TagRingType = 0
	ReceiveType  TagRingType = 1
)

type TagRing struct {
	r           *ring.Ring
	ringSize    int
	avail       int // received packets in RX rings, empty slots in TX rings
	lastWriteOp int // position in ring of last write operation
	lastReadOp  int // position in ring of last read operation
	t           TagRingType
}

type TagBuffer struct {
	b            *Buffy
	ringPosition int
}

const TagRingNoOp = -1

func NewTagRing(cap int, t TagRingType) *TagRing {
	ring := ring.New(cap)
	for i := 0; i < cap; i++ {
		// TODO: replace with buffer config
		ring.Value = &TagBuffer{b: NewBuffy(255), ringPosition: i} // fixed size packet buffers
		ring = ring.Next()
	}

	var a int
	if t == TransmitType {
		a = cap
	} else if t == ReceiveType {
		a = 0
	}

	return &TagRing{r: ring, avail: a, lastReadOp: TagRingNoOp, lastWriteOp: TagRingNoOp, ringSize: cap, t: t}
}

var FullBuffers = errors.New("TagRing: Full Buffers")

func (tr *TagRing) Reset() {
	tr.lastWriteOp = TagRingNoOp
	tr.lastReadOp = TagRingNoOp

	if tr.t == TransmitType {
		tr.avail = tr.ringSize
	} else if tr.t == ReceiveType {
		tr.avail = 0
	}

	tr.r.Do(func(p interface{}) {
		p.(*TagBuffer).b.Reset()
	})
}

func (tr *TagRing) Seek(ndx int) *TagBuffer {
	if ndx > tr.ringSize || ndx < 0 {
		panic("Seek: out of bounds ring position")
	}

	for {
		cur := tr.r.Value.(*TagBuffer).ringPosition
		if ndx == cur {
			break
		}
		tr.r = tr.r.Next()
	}

	return tr.r.Value.(*TagBuffer)
}

// Replace injects a byte slice into a position in the ring.
func (tr *TagRing) Replace(ndx int, p []byte) error {
	tr.Seek(ndx)
	replaced, err := tr.r.Value.(*TagBuffer).b.Replace(p)

	if replaced && tr.t == TransmitType {
		tr.avail--
	}

	if replaced && tr.t == ReceiveType {
		tr.avail++
	}

	return err
}

// Write encodes a byte slice to the current ring buffer
func (tr *TagRing) Write(p []byte) (int, error) {
	if tr.t == ReceiveType {
		log.Fatalf("TagRing: fatal cannot write to a receive ring")
	}

	if tr.avail == 0 {
		return 0, FullBuffers
	}
	buf := tr.nextWriteBuffer()
	n, err := buf.b.WriteUnencoded(p)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		tr.lastWriteOp = buf.ringPosition
		tr.avail-- // one less empty slot
	}
	return n, err
}

func (tr *TagRing) currentBuffer() *Buffy {
	return tr.r.Value.(*Buffy)
}

func (tr *TagRing) nextWriteBuffer() *TagBuffer {
	if tr.avail == 0 {
		return nil
	}
	curRing := tr.r
	// todo: instead of loop just keep track of this with new var (?)
	for i := 0; i < tr.ringSize; i++ {
		curBuf := curRing.Value.(*TagBuffer)
		if curBuf.b.Len() == 0 {
			return curRing.Value.(*TagBuffer)
		}
		curRing = curRing.Next()
	}
	panic(fmt.Sprintf("TagRing: Encountered writable buffer (avail: %d) with no available slots.", tr.avail))
}

func (tr *TagRing) nextReadBuffer() *TagBuffer {
	if tr.avail == 0 {
		return nil
	}
	curRing := tr.r
	// todo: instead of loop just keep track of this with new var (?)
	for i := 0; i < tr.ringSize; i++ {
		curBuf := curRing.Value.(*TagBuffer)
		if curBuf.b.Len() > 0 {
			return curRing.Value.(*TagBuffer)
		}
		curRing = curRing.Next()
	}
	panic(fmt.Sprintf("TagRing: Encountered readable buffer (avail: %d) with no available slots.", tr.avail))
}

// Read decodes the value of the current ring buffer to a byte slice
func (tr *TagRing) Read(p []byte) (int, error) {
	if tr.t == TransmitType {
		log.Fatalf("TagRing: Cannot read from a transmit ring")
	}

	if tr.avail == 0 {
		return 0, io.EOF
	}
	buf := tr.nextReadBuffer()
	b, err := buf.b.DecodedBytes()

	if err != nil {
		return 0, err
	}
	n := copy(p, b)
	if n > 0 {
		tr.lastReadOp = buf.ringPosition
		tr.avail-- // one less slot to read
	}
	return n, nil
}
