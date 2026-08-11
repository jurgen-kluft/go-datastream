# codestream

`codestream` provides a reflection-driven reader and writer for the binary format used by this repository. It sits on top of the `datastream` package and serializes Go values into a block-based binary layout that mimics C/C++ data structures.

The package is designed for predefined data shapes rather than self-describing data. The writer and reader must agree on the destination type, field order, and layout.

## API

```go
type CodeStream struct
type Options struct {
    PointerIs64Bit bool
    Endian         binary.ByteOrder
    Verbose        bool
}

func NewCodeStream(options Options) *CodeStream
func (cs *CodeStream) WriteStream(w io.Writer, data interface{}) bool
func (cs *CodeStream) ReadStream(r io.Reader, data interface{}) bool
func (cs *CodeStream) Issues() []datastream.Issue
func (cs *CodeStream) HasErrors() bool
func (cs *CodeStream) HasWarnings() bool
func (cs *CodeStream) Report()
```

`WriteStream` encodes a Go value into the binary format and writes it to `w`. `ReadStream` reads the binary format from `r` and populates `data`. Each operation creates a fresh diagnostic Context. The methods return `false` when errors were collected; warnings and information still return `true`.

## Supported Types

The current implementation supports:

- structs
- pointers
- slices
- arrays
- maps with supported key and value types
- strings
- primitive numerics and booleans

Unsupported kinds, unexported struct fields, cyclic pointer graphs, and values that exceed format limits add errors to the current Context.

## Format Overview

The format is little-endian by default. Relative pointers are signed displacements from the pointer field itself to the target block; zero is the null pointer. `Options` can select 64-bit pointers, another byte order, and verbose informational diagnostics.

Composite values are written as headers in the parent block, while the referenced payload is stored in a separate block:

- A 32-bit string uses an 8-byte header: data pointer, `uint16` byte length, and `uint16` rune length.
- A 32-bit slice or fixed array uses an 8-byte header: data pointer and `uint32` length.
- A 32-bit map uses a 12-byte header: key pointer, value pointer, and `uint32` length.
- With 64-bit pointers, strings and arrays use 16-byte, 8-byte-aligned headers; maps use a 24-byte, 8-byte-aligned header. Trailing padding is part of each header.

Strings are stored as UTF-8 bytes with a trailing null terminator. Arrays and slices are stored as contiguous element data. Maps are stored deterministically by sorting entries by encoded key bytes.

Empty slices, arrays, and maps have zero lengths and null data pointers. A non-zero length with a null data pointer is invalid.

## Example

```go
type MyStruct struct {
    Name       string
    Age        int
    Weird      uint8
    Scores     []int32
    Friends    []string
    Bank       *MyBank
    Attributes map[string]int
}

type MyBank struct {
    Name string
    USD  float64
}

var buf bytes.Buffer
stream := codestream.NewCodeStream(codestream.Options{Verbose: true})
if !stream.WriteStream(&buf, &MyStruct{ /* ... */ }) {
    stream.Report()
    return
}

var decoded MyStruct
if !stream.ReadStream(bytes.NewReader(buf.Bytes()), &decoded) {
    stream.Report()
    return
}
```

After each operation, `Issues`, `HasErrors`, `HasWarnings`, and `Report` refer only to that operation. `Report` writes errors to stderr and warnings/information to stdout.

## Determinism

The package writes data in a deterministic order so the same input value produces the same bytes across runs, assuming the input map keys encode to the same byte representation.

## Limits

- Maximum file size: under 2 GB
- Maximum string byte length: 65535 bytes
- Maximum string rune count: 65535
- Maximum array or slice length: 4294967295 elements, subject to memory and file-size limits
- Maximum per-element size for arrays and maps: 65535 bytes

## Testing

The package includes round-trip and determinism tests in `codestream_test.go`.

Run the full repository test suite with:

```bash
go test ./...
```

## Notes

- The binary format does not store type metadata.
- Readers must know the expected Go type ahead of time.
- The format is intended for stable serialization of known schemas, not for generic interchange.
