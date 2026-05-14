package datastream

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
)

// TODO going through bytes.Buffer and binary.Write is not the most efficient way to do this.

type Block struct {
	This      Pointer
	Size      int64
	Endian    binary.ByteOrder
	Alignment int
	PutBuffer []byte
	Writer    *bytes.Buffer
	Pointers  []Pointer
}

func NewBlock(ptr Pointer, endian binary.ByteOrder, alignment int) *Block {
	return &Block{
		This:      ptr,
		Size:      0,
		Endian:    endian,
		Alignment: alignment,
		PutBuffer: make([]byte, 16),
		Writer:    new(bytes.Buffer),
		Pointers:  make([]Pointer, 0),
	}
}

func (d *Block) writeTo(w io.Writer) {
	w.Write(d.Writer.Bytes())
}

func (d *Block) finalize(offset int64, pointers map[int]int64, ptrOffsets []int64) (updatedPtrOffsetSets []int64) {
	// Register the pointers and update the stream
	stream := d.Writer.Bytes()
	for _, ptr := range d.Pointers {
		o := offset + ptr.Offset
		ptrOffsets = append(ptrOffsets, o)
		d.Endian.PutUint64(stream[ptr.Offset:(ptr.Offset+8)], uint64(pointers[ptr.Index]))
	}
	return ptrOffsets
}

func align(value int64, alignment int) int64 {
	return (value + int64(alignment-1)) &^ int64(alignment-1)
}

func (d *Block) close() {
	d.Size = int64(d.Writer.Len())
}

// Align a block
func (d *Block) align(alignment int) {
	// Write out '0' bytes to align the block
	pos := d.Writer.Len()
	gap := align(int64(pos), alignment) - int64(pos)

	// If gap is 0, we're already aligned, so no need to write anything
	if gap == 0 {
		return
	}

	// Zero out PutBuffer to avoid writing garbage data
	for i := range d.PutBuffer {
		d.PutBuffer[i] = 0
	}

	for gap > 0 {
		chunk := int64(16)
		if chunk > gap {
			chunk = gap
		}
		d.writeBytes(d.PutBuffer[:chunk])
		gap -= chunk
	}
}

func (d *Block) writeString(s string) {
	strbytes := []byte(s)
	strlen := len(strbytes)
	d.align(4)
	d.Endian.PutUint32(d.PutBuffer, uint32(strlen))
	d.writeBytes(d.PutBuffer[:4])
	d.writeBytes(strbytes)
	d.writeBytes([]byte{0}) // null terminator
}

func (d *Block) writeBytes(b []byte) {
	d.Writer.Write(b)
}

func (d *Block) writeI8(i int8) {
	d.Writer.WriteByte(byte(i))
}

func (d *Block) writeI16(i int16) {
	d.align(2)
	d.Endian.PutUint16(d.PutBuffer, uint16(i))
	d.writeBytes(d.PutBuffer[:2])
}

func (d *Block) writeI32(i int32) {
	d.align(4)
	d.Endian.PutUint32(d.PutBuffer, uint32(i))
	d.writeBytes(d.PutBuffer[:4])
}

func (d *Block) writeI64(i int64) {
	d.align(8)
	d.Endian.PutUint64(d.PutBuffer, uint64(i))
	d.writeBytes(d.PutBuffer[:8])
}

func (d *Block) writeU8(i uint8) {
	d.Writer.WriteByte(byte(i))
}

func (d *Block) writeU16(i uint16) {
	d.align(2)
	d.Endian.PutUint16(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:2])
}

func (d *Block) writeU32(i uint32) {
	d.align(4)
	d.Endian.PutUint32(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:4])
}

func (d *Block) writeU64(i uint64) {
	d.align(8)
	d.Endian.PutUint64(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:8])
}

func (d *Block) writeF32(f float32) {
	d.align(4)
	d.writeU32(math.Float32bits(f))
}

func (d *Block) writeF64(f float64) {
	d.align(8)
	d.writeU64(math.Float64bits(f))
}

func (d *Block) writePtr(ptr Pointer) {
	d.align(8)
	ptr.Offset = int64(d.Writer.Len())
	d.Pointers = append(d.Pointers, ptr)
	d.Endian.PutUint64(d.PutBuffer, uint64(ptr.Offset))
	d.writeBytes(d.PutBuffer[:8])
}
