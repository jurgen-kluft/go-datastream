package datastream

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
)

type Stream struct {
	Endian      binary.ByteOrder
	PointerSize SizeOfPointer
	Alignment   int
	Current     *Block
	Stack       []*Block
	Blocks      []*Block
}

// All pointers in the stream are represented as a 4 byte offset from the start of the stream,
// which points to the start of a block. The offset is stored in the block as a pointer, and the
// block itself is stored in the stream as a block.
// When finalizing the stream, we need to resolve all pointers to their final offsets, taking
// care of alignment and deduplication.
type Pointer struct {
	Index  int
	Offset int64
}

type SizeOfPointer int8

const (
	SizeOfPointer32 SizeOfPointer = 4
	SizeOfPointer64 SizeOfPointer = 8
)

func NewStream(pointerSize SizeOfPointer, endian binary.ByteOrder) *Stream {
	str := &Stream{
		Endian:      endian,
		PointerSize: pointerSize,
		Alignment:   8, // our largest type is 8 bytes
		Current:     nil,
		Blocks:      make([]*Block, 0),
	}

	root := NewBlock(Pointer{Index: len(str.Blocks), Offset: -1}, str.PointerSize, str.Endian, str.Alignment)
	str.Blocks = append(str.Blocks, root)
	str.Current = root
	str.Stack = append(str.Stack, str.Current)
	return str
}

func (str *Stream) OpenBlock() Pointer {
	b := NewBlock(Pointer{Index: len(str.Blocks), Offset: -1}, str.PointerSize, str.Endian, str.Alignment)
	str.Blocks = append(str.Blocks, b)
	str.Stack = append(str.Stack, str.Current)
	str.Current = b
	return b.This
}

func (str *Stream) CloseBlock() {
	str.Current.close()
	str.Current = str.Stack[len(str.Stack)-1]
	str.Stack = str.Stack[:len(str.Stack)-1]
}

func (str *Stream) Align(alignment int) {
	str.Current.align(alignment)
}

func (str *Stream) Align2() {
	str.Current.align(2)
}

func (str *Stream) Align4() {
	str.Current.align(4)
}

func (str *Stream) Align8() {
	str.Current.align(8)
}

func (str *Stream) WriteBytes(data []byte) {
	str.Current.writeBytes(data)
}

func (str *Stream) WriteI8(i int8) {
	str.Current.writeI8(i)
}

func (str *Stream) WriteI16(i int16) {
	str.Current.writeI16(i)
}

func (str *Stream) WriteI32(i int32) {
	str.Current.writeI32(i)
}

func (str *Stream) WriteI64(i int64) {
	str.Current.writeI64(i)
}

func (str *Stream) WriteU8(i uint8) {
	str.Current.writeU8(i)
}

func (str *Stream) WriteU16(i uint16) {
	str.Current.writeU16(i)
}

func (str *Stream) WriteU32(i uint32) {
	str.Current.writeU32(i)
}

func (str *Stream) WriteU64(i uint64) {
	str.Current.writeU64(i)
}

func (str *Stream) WriteFloat(f float32) {
	str.Current.writeF32(f)
}

func (str *Stream) WriteDouble(f float64) {
	str.Current.writeF64(f)
}

func (str *Stream) WritePtr(ptr Pointer) {
	str.Current.writePtr(ptr)
}

func (str *Stream) Finalize() error {
	if len(str.Stack) == 0 && str.Current == nil {
		return nil // already finalized
	}
	if len(str.Stack) > 1 {
		return fmt.Errorf("Cannot finalize stream with open blocks")
	}
	str.Current.close()
	str.Stack = str.Stack[:0]
	str.Current = nil
	return nil
}

// Finalize will write out a finalized stream to disk
func (str *Stream) Write(w io.Writer) (pointerOffsets []int64, err error) {
	// Make sure the stream is finalized
	if err := str.Finalize(); err != nil {
		return nil, err
	}

	// Compute the total number of pointer offsets we may use, so we can preallocate the slice
	countPtrOffsets := 0
	for _, block := range str.Blocks {
		countPtrOffsets += len(block.Pointers)
	}
	pointerOffsets = make([]int64, 0, countPtrOffsets)

	// deduplicate block then finalize all blocks, collecting all pointer offsets
	// Note: When we deduplicate a block, we need to update all pointers that point
	//       to it, which may cause more deduplications, so we need to keep iterating
	//       until no more deduplications are found.

	dedupMap := make(map[[20]byte]int, len(str.Blocks)) // map from hash to block index

	dedupNum := 1
	for dedupNum > 0 {
		dedupNum = 0

		// resolve all the pointers, taking care of blocks that have been deduplicated
		pointers := make(map[int]int64)

		// go over all unique blocks and assign offsets, taking care of alignment
		var globalOffset int64 = 0 // global offset
		for _, block := range str.Blocks {
			if block.Canonical >= 0 {
				// skip blocks that are inactive due to deduplication
				continue
			}
			if offset, ok := pointers[block.This.Index]; ok {
				block.This.Offset = offset
				continue
			}
			// block alignment
			if globalOffset%int64(block.Alignment) != 0 {
				padding := int64(block.Alignment) - (globalOffset % int64(block.Alignment))
				globalOffset += padding
			}
			block.This.Offset = globalOffset
			pointers[block.This.Index] = globalOffset
			globalOffset += int64(block.Size)
		}

		// finalize all blocks
		pointerOffsets = pointerOffsets[:0] // reset the slice while keeping the capacity
		for _, block := range str.Blocks {
			if block.Canonical >= 0 {
				// skip blocks that are inactive due to deduplication
				continue
			}
			pointerOffsets = block.finalize(block.This.Offset, pointers, pointerOffsets)
		}

		// deduplicate blocks using SHA1
		dedupNum = 0
		clear(dedupMap)
		hasher := sha1.New()
		for _, block := range str.Blocks {
			if block.Canonical >= 0 {
				// skip blocks that are inactive due to deduplication
				continue
			}
			hash := block.hash(hasher)
			if existing, ok := dedupMap[hash]; ok {
				block.Canonical = existing
				dedupNum++
			} else {
				dedupMap[hash] = block.This.Index
			}
		}
	}

	// write all blocks to a buffer, taking care of alignment
	buffer := bytes.Buffer{}
	for _, block := range str.Blocks {
		if block.Canonical >= 0 {
			// skip blocks that are inactive due to deduplication
			continue
		}
		pos := buffer.Len()

		// check if pos adheres to block alignment, if not write padding
		if pos%block.Alignment != 0 {
			padding := block.Alignment - (pos % block.Alignment)
			for i := 0; i < padding; i++ {
				buffer.WriteByte(0)
			}
		}

		block.writeTo(&buffer)
	}

	// write the full buffer to the output writer
	_, err = w.Write(buffer.Bytes())
	if err != nil {
		return nil, err
	}

	// sort the ptroffsets
	sort.Slice(pointerOffsets, func(i, j int) bool {
		return pointerOffsets[i] < pointerOffsets[j]
	})

	return pointerOffsets, nil
}
