package datastream

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestNewBlock(t *testing.T) {
	ptr := Pointer{Index: 0, Offset: -1}
	block := NewBlock(ptr, binary.LittleEndian, 8)
	if block.This != ptr {
		t.Errorf("Expected This to be %v, got %v", ptr, block.This)
	}
	if block.Endian != binary.LittleEndian {
		t.Error("Expected LittleEndian")
	}
	if block.Alignment != 8 {
		t.Error("Expected alignment 8")
	}
	if len(block.PutBuffer) != 16 {
		t.Error("Expected PutBuffer len 16")
	}
}

func TestWriteI8(t *testing.T) {
	block := NewBlock(Pointer{Index: 0, Offset: -1}, binary.LittleEndian, 8)
	block.writeI8(42)
	data := block.Writer.Bytes()
	if len(data) != 1 || data[0] != 42 {
		t.Errorf("Expected [42], got %v", data)
	}
}

func TestWriteU16(t *testing.T) {
	block := NewBlock(Pointer{Index: 0, Offset: -1}, binary.LittleEndian, 8)
	block.writeU16(0x1234)
	data := block.Writer.Bytes()
	expected := []byte{0x34, 0x12} // little endian
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}
}

func TestAlign(t *testing.T) {
	block := NewBlock(Pointer{Index: 0, Offset: -1}, binary.LittleEndian, 8)
	block.writeI8(1) // pos=1
	block.align(4)   // gap=3, write 3 zeros
	data := block.Writer.Bytes()
	expected := []byte{1, 0, 0, 0}
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}
}

func TestWriteString(t *testing.T) {
	block := NewBlock(Pointer{Index: 0, Offset: -1}, binary.LittleEndian, 8)
	block.writeString("hi")
	data := block.Writer.Bytes()
	// align 4: pos=0, gap=0
	// len=2 as uint32: 2,0,0,0
	// "hi" : h,i
	// null: 0
	expected := []byte{2, 0, 0, 0, 'h', 'i', 0}
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}
}

func TestWritePtr(t *testing.T) {
	block := NewBlock(Pointer{Index: 0, Offset: -1}, binary.LittleEndian, 8)
	ptr := Pointer{Index: 1, Offset: 0}
	block.writePtr(ptr)
	data := block.Writer.Bytes()
	// align 8: pos=0, gap=0
	// write placeholder offset, but ptr.Offset set to 0
	// PutUint64 with 0
	expected := make([]byte, 8)
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}
	if ptr.Offset != 0 {
		t.Errorf("Expected ptr.Offset 0, got %d", ptr.Offset)
	}
}

func TestBlockFinalize(t *testing.T) {
	block := NewBlock(Pointer{Index: 0, Offset: -1}, binary.LittleEndian, 8)
	ptr := Pointer{Index: 1, Offset: 0}
	block.writePtr(ptr)
	pointers := map[int]int64{1: 100}
	var ptroffsets []int64
	ptroffsets = block.finalize(0, pointers, ptroffsets)
	data := block.Writer.Bytes()
	// should have updated to 100
	expected := make([]byte, 8)
	binary.LittleEndian.PutUint64(expected, 100)
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}
	if len(ptroffsets) != 1 || ptroffsets[0] != 0 {
		t.Errorf("Expected ptroffsets [0], got %v", ptroffsets)
	}
}
