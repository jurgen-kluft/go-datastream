# codestream

`codestream` provides a reflection-driven reader and writer for the binary format used by this repository. It sits on top of the `datastream` package and serializes Go values into a block-based binary layout that mimics C/C++ data structures.

The package is designed for predefined data shapes rather than self-describing data. The writer and reader must agree on the destination type, field order, and layout.

## API

```go
func WriteToStream(w io.Writer, data interface{}) error
func ReadFromStream(r io.Reader, data interface{}) error
```

`WriteToStream` encodes a Go value into the binary format and writes it to `w`. `ReadFromStream` reads the binary format from `r` and populates `data`.

## Supported Types

The current implementation supports:

- structs
- pointers
- slices
- arrays
- maps with supported key and value types
- strings
- primitive numerics and booleans

Unsupported kinds, unexported struct fields, cyclic pointer graphs, and values that exceed format limits return an error.

## Format Overview

The format is little-endian and uses 32-bit offsets so the final file stays below the 2 GB range.

Composite values are written as headers in the parent block, while the referenced payload is stored in a separate block:

- `string_t` uses an 8-byte header: byte length, rune count, and data offset
- `array_t` uses an 8-byte header: length(4), and data offset(4)
- `map_t` uses a 12-byte header: length(4), key array offset(4), and value array offset(4)

Strings are stored as UTF-8 bytes with a trailing null terminator. Arrays and slices are stored as contiguous element data. Maps are stored deterministically by sorting entries by encoded key bytes.

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
err := codestream.WriteToStream(&buf, &MyStruct{ /* ... */ })
if err != nil {
    log.Fatal(err)
}

var decoded MyStruct
err = codestream.ReadFromStream(bytes.NewReader(buf.Bytes()), &decoded)
if err != nil {
    log.Fatal(err)
}
```

## Determinism

The package writes data in a deterministic order so the same input value produces the same bytes across runs, assuming the input map keys encode to the same byte representation.

## Limits

- Maximum file size: under 2 GB
- Maximum string byte length: 65535 bytes
- Maximum string rune count: 65535
- Maximum array or slice length: 65535 elements
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