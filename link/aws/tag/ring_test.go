package tag

import (
	"io"
	"log"
	"testing"
)

func TestRing_Read(t *testing.T) {

	r := NewTagRing(3, ReceiveType)
	r.Replace(0, []byte("aGVsbG8="))
	r.Replace(1, []byte("MTIzNA=="))
	r.Replace(1, []byte("MTIzNA=="))

	tables := []struct {
		read      []byte
		readBytes int
		readError error
		avail     int
	}{
		{[]byte("hello"), 4, nil, 1},
		{[]byte("1234"), 5, nil, 0},
		{nil, 0, io.EOF, 0},
	}

	for i, table := range tables {
		p := make([]byte, 255)
		n, err := r.Read(p)
		if err != table.readError {
			t.Fatalf("[%d] TestRing_Read: got unexpected error: %v", i, err)
		}
		if n != table.readBytes {
			t.Fatalf("[%d] TestRing_Read: error reading, expected %d, got %d", i, table.readBytes, n)
		}
		if r.avail != table.avail {
			t.Fatalf("[%d] TestRing_Read: expected %v available buffers, got %v", i, table.avail, r.avail)
		}
	}
}

func TestRing_Write(t *testing.T) {
	r := NewTagRing(4, TransmitType)
	r.Replace(0, []byte("aGVsbG8="))

	tables := []struct {
		write      []byte
		writeBytes int
		writeError error
		avail      int
	}{
		{[]byte{0x1, 0x2, 0x3}, 4, nil, 2},
		{[]byte{0x4, 0x5, 0x6}, 4, nil, 1},
		{make([]byte, 0), 0, nil, 1},
		{[]byte{0x7, 0x8, 0x9}, 4, nil, 0},
		{[]byte{0x7, 0x8, 0x9}, 0, FullBuffers, 0},
	}

	for i, table := range tables {
		n, err := r.Write(table.write)
		if err != table.writeError {
			t.Fatalf("[%d] TestRing_Write: got unexpected error: %v", i, err)
		}
		if n != table.writeBytes {
			log.Printf("[%d] TestRing_Write: error writing, got %d", i, n)
		}
		if r.avail != table.avail {
			t.Fatalf("[%d] TestRing_Write: expected 2 available buffers, got %v", i, r.avail)
		}
	}
}
