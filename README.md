# go-datastream

`go-datastream` is a small Go library for writing deterministic binary data with block-based layout, pointer relocation, alignment, and block deduplication.

The repository is split into two packages:

- [`datastream`](datastream/README.md) is the low-level block writer. It handles alignment, pointers, finalization, and SHA1-based deduplication of identical blocks.
- [`codestream`](codestream/README.md) is a reflection-based codec built on top of `datastream`. It serializes Go structs, pointers, slices, arrays, maps, strings, and primitive values into a binary format that resembles C/C++ data layouts.

## Why This Exists

The goal of the project is to support binary formats that are meant to be read back as structured data with known layouts. Instead of storing type metadata, the format writes data into separate blocks and records offsets to those blocks in the main stream.

That makes the library useful for:

- compact binary storage
- deterministic output for the same input data
- block sharing when repeated values deduplicate cleanly
- data layouts that need to be close to in-memory C-style structures

## Repository Layout

- `datastream/` - core block and pointer stream writer
- `codestream/` - reflection-based higher-level serializer and reader
- `datastream/*.go` - low-level writer implementation and tests
- `codestream/*.go` - codec implementation, design notes, and tests

## Quick Start

Run the full test suite:

```bash
go test ./...
```

If you want to explore the packages individually, start with:

- [`datastream/README.md`](datastream/README.md)
- [`codestream/README.md`](codestream/README.md)

## Package Summary

`datastream` provides the primitives: open a block, write values into it, write pointers to other blocks, then finalize the stream into a single binary output.

`codestream` builds on those primitives and adds reflection-based encoding and decoding for predefined Go types. It is designed for deterministic binary round-tripping, not for self-describing interchange formats.
