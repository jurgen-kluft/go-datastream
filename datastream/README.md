# datastream

`datastream` is a low-level binary writer for building block-based data files with pointers, alignment, and deduplication.

It is designed for formats that need to lay out structured data in memory-friendly blocks, such as C/C++-style object graphs. Higher-level packages, such as `codestream`, can build on top of it to serialize richer Go values.

## What It Does

- writes binary data into blocks
- emits pointers to other blocks as offsets from the start of the stream
- aligns blocks and field boundaries as needed
- deduplicates identical blocks using SHA1 hashing before writing

## API

```go
type Stream struct
type Block struct
type Pointer struct
type Context struct
type Issue struct
type IssueType int8

func NewStream(pointerSize SizeOfPointer, endian binary.ByteOrder, context *Context) *Stream

func (s *Stream) OpenBlock() Pointer
func (s *Stream) CloseBlock()
func (s *Stream) Align(alignment int)
func (s *Stream) Align2()
func (s *Stream) Align4()
func (s *Stream) Align8()

func (s *Stream) WriteBytes(data []byte)
func (s *Stream) WriteI8(i int8)
func (s *Stream) WriteI16(i int16)
func (s *Stream) WriteI32(i int32)
func (s *Stream) WriteI64(i int64)
func (s *Stream) WriteU8(i uint8)
func (s *Stream) WriteU16(i uint16)
func (s *Stream) WriteU32(i uint32)
func (s *Stream) WriteU64(i uint64)
func (s *Stream) WriteFloat(f float32)
func (s *Stream) WriteDouble(f float64)
func (s *Stream) WritePtr(ptr Pointer)

func (s *Stream) SetVerbose(verbose bool)
func (s *Stream) Issues() []Issue
func (s *Stream) HasErrors() bool
func (s *Stream) HasWarnings() bool
func (c *Context) Report()

func (s *Stream) Finalize()
func (s *Stream) Write(w io.Writer) []int64
```

`Write` finalizes the stream, writes the packed bytes to an `io.Writer`, and returns the sorted list of pointer offsets that were emitted into the final file.

Errors and warnings are collected in the caller-owned Context instead of being returned immediately. Warnings allow serialization to continue. Errors prevent output when they are known before writing. Enable verbose mode on the Context or Stream to collect informational issues for block layout, pointer resolution, deduplication, and output statistics. Context records every issue and does not deduplicate repeated reports.

`Context.Report` prints errors to stderr and warnings/information to stdout.

Each issue has an `IssueType`, a `Description`, and a `String` helper that formats it as `<Severity>: <Description>`. Pointer diagnostics include the file and line where `WritePtr` was called.

## Usage

```go
package main

import (
    "bytes"
    "encoding/binary"
    "fmt"

    "github.com/jurgen-kluft/go-datastream/datastream"
)

func main() {
    context := datastream.NewContext()
    stream := datastream.NewStream(datastream.SizeOfPointer32, binary.LittleEndian, context)

    stream.WriteU8(1)

    child := stream.OpenBlock()
    stream.WriteU16(42)
    stream.CloseBlock()

    stream.Align4()
    stream.WritePtr(child)

    var buf bytes.Buffer
    stream.SetVerbose(true)
    offsets := stream.Write(&buf)
    if stream.HasErrors() {
        for _, issue := range stream.Issues() {
            fmt.Println(issue.String())
        }
        return
    }

    fmt.Println("pointer offsets:", offsets)
    fmt.Printf("bytes: %x\n", buf.Bytes())

    if stream.HasWarnings() {
        for _, issue := range stream.Issues() {
            fmt.Println(issue.String())
        }
    }
}
```

## Block Model

Each call to `OpenBlock` creates a new block. Data written while that block is active is stored separately from the parent block. When `CloseBlock` is called, the block is sealed and its size is recorded.

Pointers are written as placeholders in the current block and later patched to the final offset of their target block during finalization.

## Alignment

Alignment is explicit. Use `Align`, `Align2`, `Align4`, or `Align8` to insert zero padding into the current block.

The stream also aligns whole blocks when writing the final file so that each block begins at its configured alignment boundary.

## Deduplication

Before writing, the stream hashes block contents with SHA1 and removes duplicate blocks. If two blocks contain the same bytes, only one is written and later pointers are redirected to the canonical block.

This is especially useful for repeated strings, arrays, and maps.

## Pointer Size

`SizeOfPointer32` and `SizeOfPointer64` control the number of bytes used when writing pointer placeholders inside blocks.

## Notes

- The package uses endian encoding for all multi-byte values.
- The writer is intended for deterministic binary output.
- The stream must be finalized before writing it out.

## Testing

Run the package tests with:

```bash
go test ./...
```
