package datastream

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestNewStream(t *testing.T) {
	stream := NewStream(SizeOfPointer64, binary.LittleEndian)
	if stream.Endian != binary.LittleEndian {
		t.Error("Expected LittleEndian")
	}
	if stream.Alignment != 8 {
		t.Error("Expected alignment 8")
	}
	if stream.Current == nil {
		t.Error("Expected Current not nil")
	}
	if len(stream.Stack) != 1 {
		t.Error("Expected Stack len 1")
	}
	if len(stream.Blocks) != 1 {
		t.Error("Expected Blocks len 1")
	}
}

func TestOpenCloseBlock(t *testing.T) {
	stream := NewStream(SizeOfPointer64, binary.LittleEndian)
	initialCurrent := stream.Current
	stream.OpenBlock()
	if stream.Current == initialCurrent {
		t.Error("Expected Current to change")
	}
	if len(stream.Blocks) != 2 {
		t.Error("Expected Blocks len 2")
	}
	if len(stream.Stack) != 2 {
		t.Error("Expected Stack len 2")
	}
	stream.CloseBlock()
	if stream.Current != initialCurrent {
		t.Error("Expected Current back to initial")
	}
	if len(stream.Stack) != 1 {
		t.Error("Expected Stack len 1 after close")
	}
}

func TestWriteMethods(t *testing.T) {
	stream := NewStream(SizeOfPointer64, binary.LittleEndian)
	stream.WriteI8(42)
	stream.Align2() // you need to align manually
	stream.WriteU16(0x1234)
	stream.Align4()
	str := "test"
	stream.WriteU32(uint32(len(str)))
	stream.WriteBytes([]byte(str))
	stream.WriteU8(0) // null terminator
	data := stream.Current.Writer.Bytes()
	// I8: 42
	// U16 align 2: pos=1, gap=1, write 0, then 34 12
	// String: align 4: pos=4, gap=0, len=4,0,0,0, t,e,s,t,0
	expected := []byte{42, 0, 0x34, 0x12, 4, 0, 0, 0, 't', 'e', 's', 't', 0}
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}
}

func TestFinalize(t *testing.T) {
	stream := NewStream(SizeOfPointer64, binary.LittleEndian)
	stream.WriteI8(1)
	ptr := stream.OpenBlock()
	stream.WriteI8(2)
	stream.CloseBlock()
	stream.Align8()      // align before writing pointer
	stream.WritePtr(ptr) // ptr to the second block

	// Create temp files
	dataFile, err := os.CreateTemp("", "data")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dataFile.Name())

	relocFile, err := os.CreateTemp("", "reloc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(relocFile.Name())

	err = stream.Finalize()

	writer := bytes.NewBuffer(nil)
	var pointerOffsets []int64
	pointerOffsets, err = stream.Write(writer)
	if err != nil {
		t.Fatal(err)
	}
	if len(pointerOffsets) != 1 {
		t.Fatalf("Expected 1 pointer offset, got %d", len(pointerOffsets))
	}
	if pointerOffsets[0] != 8 {
		t.Errorf("Expected pointer offset 8, got %d", pointerOffsets[0])
	}

	// Read data file
	data := writer.Bytes()

	// Block 0: 1, 7 bytes padding, 8-byte pointer to block 1
	// Block 1: 2
	// Total 17 bytes
	expectedData := make([]byte, 17)
	expectedData[0] = 1
	binary.LittleEndian.PutUint64(expectedData[8:16], 16)
	expectedData[16] = 2
	if !bytes.Equal(data, expectedData) {
		t.Errorf("Expected data %v, got %v", expectedData, data)
	}

}
