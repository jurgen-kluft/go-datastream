package datastream

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"io"
	"runtime"
	"sort"
)

type Stream struct {
	Endian      binary.ByteOrder
	PointerSize SizeOfPointer
	Alignment   int
	Current     *Block
	Stack       []*Block
	Blocks      []*Block
	context     *Context
	finalized   bool
	written     bool
}

// All pointers in the stream are represented as a signed displacement from the serialized pointer
// field itself to the start of a block. The pointer field is 4 or 8 bytes according to PointerSize.
// A zero displacement is reserved for null pointers.
// When finalizing the stream, we need to resolve all pointers to their final offsets, taking
// care of alignment and deduplication.
type Pointer struct {
	Index  int
	Offset int64
}

func NilPtr() Pointer {
	return Pointer{Index: -1, Offset: 0}
}

type SizeOfPointer int8

const (
	SizeOfPointer32 SizeOfPointer = 4
	SizeOfPointer64 SizeOfPointer = 8
)

func NewStream(pointerSize SizeOfPointer, endian binary.ByteOrder, ctx *Context) *Stream {
	if ctx == nil {
		panic("datastream: context cannot be nil")
	}
	str := &Stream{
		Endian:      endian,
		PointerSize: pointerSize,
		Alignment:   8, // our largest type is 8 bytes
		Current:     nil,
		Blocks:      make([]*Block, 0),
		context:     ctx,
	}

	root := NewBlock(Pointer{Index: len(str.Blocks), Offset: -1}, str.PointerSize, str.Endian, str.Alignment)
	str.Blocks = append(str.Blocks, root)
	str.Current = root
	str.Stack = append(str.Stack, str.Current)
	return str
}

func (str *Stream) SetVerbose(verbose bool) {
	str.context.SetVerbose(verbose)
}

func (str *Stream) Issues() []Issue {
	return str.context.Issues()
}

func (str *Stream) HasErrors() bool {
	return str.context.HasErrors()
}

func (str *Stream) HasWarnings() bool {
	return str.context.HasWarnings()
}

func (str *Stream) activeBlock(operation string) *Block {
	if str.Current == nil || str.finalized {
		str.context.AddError("%s after stream finalization", operation)
		return nil
	}
	return str.Current
}

func (str *Stream) OpenBlock() Pointer {
	if str.activeBlock("opening a block") == nil {
		return NilPtr()
	}
	b := NewBlock(Pointer{Index: len(str.Blocks), Offset: -1}, str.PointerSize, str.Endian, str.Alignment)
	str.Blocks = append(str.Blocks, b)
	str.Stack = append(str.Stack, str.Current)
	str.Current = b
	str.context.AddInformation("opened block %d", b.This.Index)
	return b.This
}

func (str *Stream) CloseBlock() {
	if str.activeBlock("closing a block") == nil {
		return
	}
	if len(str.Stack) <= 1 {
		str.context.AddError("cannot close the root block")
		return
	}
	str.Current.close(str.context)
	str.Current = str.Stack[len(str.Stack)-1]
	str.Stack = str.Stack[:len(str.Stack)-1]
}

func (str *Stream) AlignStruct() {
	str.Align(int(str.PointerSize))
}

func (str *Stream) Align(alignment int) {
	block := str.activeBlock("aligning data")
	if block == nil {
		return
	}
	if alignment <= 0 || alignment&(alignment-1) != 0 {
		str.context.AddError("alignment %d is not a positive power of two", alignment)
		return
	}
	block.align(str.context, alignment)
}

func (str *Stream) Align2() {
	str.Align(2)
}

func (str *Stream) Align4() {
	str.Align(4)
}

func (str *Stream) Align8() {
	str.Align(8)
}

func (str *Stream) WriteBytes(data []byte) {
	if block := str.activeBlock("writing bytes"); block != nil {
		block.writeBytes(data)
	}
}

func (str *Stream) WriteI8(i int8) {
	if block := str.activeBlock("writing int8"); block != nil {
		block.writeI8(i)
	}
}

func (str *Stream) WriteI16(i int16) {
	if block := str.activeBlock("writing int16"); block != nil {
		block.writeI16(i)
	}
}

func (str *Stream) WriteI32(i int32) {
	if block := str.activeBlock("writing int32"); block != nil {
		block.writeI32(i)
	}
}

func (str *Stream) WriteI64(i int64) {
	if block := str.activeBlock("writing int64"); block != nil {
		block.writeI64(i)
	}
}

func (str *Stream) WriteU8(i uint8) {
	if block := str.activeBlock("writing uint8"); block != nil {
		block.writeU8(i)
	}
}

func (str *Stream) WriteU16(i uint16) {
	if block := str.activeBlock("writing uint16"); block != nil {
		block.writeU16(i)
	}
}

func (str *Stream) WriteU32(i uint32) {
	if block := str.activeBlock("writing uint32"); block != nil {
		block.writeU32(i)
	}
}

func (str *Stream) WriteU64(i uint64) {
	if block := str.activeBlock("writing uint64"); block != nil {
		block.writeU64(i)
	}
}

func (str *Stream) WriteFloat(f float32) {
	if block := str.activeBlock("writing float32"); block != nil {
		block.writeF32(f)
	}
}

func (str *Stream) WriteDouble(f float64) {
	if block := str.activeBlock("writing float64"); block != nil {
		block.writeF64(f)
	}
}

func (str *Stream) WritePtr(ptr Pointer) {
	block := str.activeBlock("writing a pointer")
	if block == nil {
		return
	}
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}
	block.writePtr(ptr, sourceLocation{file: file, line: line})
}

func (str *Stream) Finalize() {
	if len(str.Stack) == 0 && str.Current == nil {
		return
	}
	if len(str.Stack) > 1 {
		str.context.AddError("cannot finalize stream with %d open child blocks", len(str.Stack)-1)
		return
	}
	str.Current.close(str.context)
	str.Stack = str.Stack[:0]
	str.Current = nil
	str.finalized = true
}

// Finalize will write out a finalized stream to disk
func (str *Stream) Write(w io.Writer) []int64 {
	if str.written {
		str.context.AddError("stream has already been written")
		return nil
	}
	if w == nil {
		str.context.AddError("cannot write stream to a nil writer")
		return nil
	}
	// Make sure the stream is finalized
	str.Finalize()
	if str.HasErrors() {
		return nil
	}

	// Compute the total number of pointer offsets we may use, so we can preallocate the slice
	countPtrOffsets := 0
	for _, block := range str.Blocks {
		countPtrOffsets += len(block.Pointers)
	}
	pointerOffsets := make([]int64, 0, countPtrOffsets)

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
			if block.Alignment <= 0 || block.Alignment&(block.Alignment-1) != 0 {
				str.context.AddError("block %d alignment %d is not a positive power of two", block.This.Index, block.Alignment)
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
		for _, block := range str.Blocks {
			if block.Canonical >= 0 {
				if canonicalOffset, ok := pointers[block.Canonical]; ok {
					pointers[block.This.Index] = canonicalOffset
				}
			}
		}
		if str.HasErrors() {
			return nil
		}

		// Patch pointers for hashing without emitting diagnostics for intermediate layouts.
		for _, block := range str.Blocks {
			if block.Canonical >= 0 {
				// skip blocks that are inactive due to deduplication
				continue
			}
			block.finalize(str.context, block.This.Offset, pointers, nil, false)
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
				str.context.AddInformation("deduplicated block %d to block %d", block.This.Index, existing)
			} else {
				dedupMap[hash] = block.This.Index
			}
		}
	}

	// Resolve the converged layout once more and emit final pointer diagnostics.
	pointers := make(map[int]int64, len(str.Blocks))
	for _, block := range str.Blocks {
		if block.Canonical < 0 {
			pointers[block.This.Index] = block.This.Offset
		}
	}
	for _, block := range str.Blocks {
		if block.Canonical >= 0 {
			if canonicalOffset, ok := pointers[block.Canonical]; ok {
				pointers[block.This.Index] = canonicalOffset
			}
		}
	}
	pointerOffsets = pointerOffsets[:0]
	for _, block := range str.Blocks {
		if block.Canonical < 0 {
			pointerOffsets = block.finalize(str.context, block.This.Offset, pointers, pointerOffsets, true)
		}
	}
	if str.HasErrors() {
		return nil
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

		if !block.writeTo(str.context, &buffer) {
			return nil
		}
	}

	// write the full buffer to the output writer
	written, err := w.Write(buffer.Bytes())
	if err == nil && written != buffer.Len() {
		err = io.ErrShortWrite
	}
	if err != nil {
		str.context.AddError("writing stream output: %v", err)
		return nil
	}
	str.written = true
	str.context.AddInformation("wrote %d bytes with %d pointer offsets", buffer.Len(), len(pointerOffsets))

	// sort the ptroffsets
	sort.Slice(pointerOffsets, func(i, j int) bool {
		return pointerOffsets[i] < pointerOffsets[j]
	})

	return pointerOffsets
}
