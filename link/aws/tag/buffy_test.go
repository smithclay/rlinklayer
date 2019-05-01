package tag

import (
	"bytes"
	"io"
	"testing"
)

func setup(cap int) *Buffy {
	return NewBuffy(cap)
}

func TestBuffy_Read(t *testing.T) {
	capacity := 16
	b := setup(capacity)
	originalText := []byte("helloworld")
	inputBytes := []byte("aGVsbG93b3JsZA==")
	_, err := b.Write(inputBytes)
	if err != nil {
		t.Errorf("Unexpected write error %v", err)
		return
	}

	// Replace has no impact on buffer cursor position
	b.Replace(inputBytes)
	b.Replace(inputBytes)

	decodedBytes, err := b.DecodedBytes()
	if err != nil {
		t.Errorf("Unexpected read error %v", err)
		return
	}

	if !bytes.Equal(originalText, decodedBytes) {
		t.Errorf("Expected decoded to be '%s' (len: %d) got '%s' (len: %d)", originalText, len(originalText), decodedBytes, len(decodedBytes))
		return
	}
	if b.Len() != 0 {
		t.Errorf("0: Expected buffer length to be 0")
		return
	}

	_, err = b.DecodedBytes()
	if err != io.EOF {
		t.Errorf("Expected read error %v", err)
		return
	}

	if b.Len() != 0 {
		t.Errorf("1: Expected buffer length to be 0")
		return
	}
}

func TestBuffy_Replace(t *testing.T) {
	capacity := 16
	b := setup(capacity)
	tables := []struct {
		write        []byte
		replaceBytes []byte
		writeErr     error
	}{
		{make([]byte, 0), []byte("aGVsbG8="), nil},
		{make([]byte, 3), []byte("aGVsbG8="), nil},
		{[]byte("aGVsbG8="), make([]byte, 3), nil},
		{make([]byte, 3), []byte("aGVsbG8="), nil},
	}
	for i, table := range tables {
		b.Reset()
		_, err := b.Write(table.write)
		if err != table.writeErr {
			t.Errorf("[%d] Expected write error %v, got: %v", i, table.writeErr, err)
			continue
		}
		b.Replace(table.replaceBytes)
		if !bytes.Equal(table.replaceBytes, b.EncodedBytes()) {
			t.Errorf("[%d] Expected internal buffer %v, got: %v", i, table.replaceBytes, b.EncodedBytes())
			continue
		}
	}
}

func TestBuffy_WriteEncoded(t *testing.T) {
	capacity := 16
	b := setup(capacity)
	tables := []struct {
		write          []byte
		internalBuffer []byte
		read           []byte
		writeErr       error
		readErr        error
	}{
		{make([]byte, 0), make([]byte, 0), make([]byte, 0), nil, nil},
		{make([]byte, 3), []byte("AAAA"), make([]byte, 3), nil, nil}, // is this right?
		{[]byte("hello"), []byte("aGVsbG8="), []byte("hello"), nil, nil},
		{[]byte("this-is-over-capacity"), make([]byte, 0), make([]byte, 0), ErrOverCapacity, nil},
	}

	for i, table := range tables {
		b.Reset()
		_, err := b.WriteUnencoded(table.write)
		if err != table.writeErr {
			t.Errorf("[%d] Expected write error %v, got: %v", i, table.writeErr, err)
			continue
		}
		if !bytes.Equal(table.internalBuffer, b.EncodedBytes()) {
			t.Errorf("[%d] Expected internal buffer %v, got: %v", i, table.internalBuffer, b.EncodedBytes())
			continue
		}
		//decodedBytes := make([]byte, 4)
		decodedBytes, err := b.DecodedBytes()
		if err != table.readErr {
			t.Errorf("[%d] Expected read error %v, got: %v", i, table.readErr, err)
			continue
		}
		if !bytes.Equal(table.read, decodedBytes) {
			t.Errorf("[%d] Expected read buffer %v (\"%s\"), got: %v", i, table.read, table.read, decodedBytes)
			continue
		}
	}
}
func TestBuffy_ReadWriteRaw(t *testing.T) {
	b := setup(4)
	tables := []struct {
		write        []byte
		read         []byte
		writtenBytes int
		readBytes    int
		readBufSize  int
		readErr      error
		writeErr     error
	}{
		{make([]byte, 0), make([]byte, 0), 0, 0, 0, nil, nil},
		{[]byte{0x1, 0x2, 0x3}, []byte{0x1, 0x2, 0x3}, 3, 3, 3, nil, nil},
		{[]byte{0x1, 0x2, 0x3, 0x4}, []byte{0x1, 0x2, 0x3, 0x4}, 4, 4, 4, nil, nil},
		{[]byte{0x1, 0x2, 0x3, 0x4}, []byte{0x1, 0x2, 0x3, 0x4}, 4, 4, 16, nil, nil},
		{[]byte{0x1, 0x2, 0x3, 0x4}, []byte{0x1, 0x2}, 4, 2, 2, nil, nil},
		{[]byte{0x1, 0x2, 0x3, 0x4, 0x5}, []byte{}, 0, 0, 0, nil, ErrOverCapacity},
	}
	for i, table := range tables {
		b.Reset()
		n, err := b.Write(table.write)
		if err != table.writeErr {
			t.Errorf("[%d] Expected write error %v, got: %v", i, table.writeErr, err)
			continue
		}
		if n != table.writtenBytes {
			t.Errorf("[%d] Expected %d bytes written, got %d", i, table.writtenBytes, n)
			continue
		}

		readBuffer := make([]byte, table.readBytes)
		m, err := b.Read(readBuffer)
		if err != table.readErr {
			t.Errorf("[%d] Expected read error %v, got: %v", i, table.readErr, err)
			continue
		}
		if m != table.readBytes {
			t.Errorf("[%d] Expected %d bytes read, got: %d", i, table.writtenBytes, m)
			continue
		}
		if !bytes.Equal(table.read, readBuffer) {
			t.Errorf("[%d] Expected read buffer %v, got: %v", i, table.read, readBuffer)
			continue
		}

		if b.Len() != (table.writtenBytes - table.readBytes) {
			t.Errorf("[%d] Expected buffer length to be %d, got: %d", i, (table.writtenBytes - table.readBytes), b.Len())
		}
	}
}
