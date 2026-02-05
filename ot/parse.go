// Package ot provides OpenType font table parsing.
package ot

import (
	"encoding/binary"
	"errors"
)

// Common errors
var (
	ErrInvalidFont    = errors.New("invalid font data")
	ErrTableNotFound  = errors.New("table not found")
	ErrInvalidTable   = errors.New("invalid table data")
	ErrInvalidOffset  = errors.New("offset out of bounds")
	ErrInvalidFormat  = errors.New("unsupported format")
	ErrInvalidFeature = errors.New("invalid feature string")
)

// Tag is a 4-byte OpenType tag.
type Tag uint32

// MakeTag creates a Tag from 4 bytes.
func MakeTag(a, b, c, d byte) Tag {
	return Tag(uint32(a)<<24 | uint32(b)<<16 | uint32(c)<<8 | uint32(d))
}

// String returns the tag as a 4-character string.
func (t Tag) String() string {
	return string([]byte{
		byte(t >> 24),
		byte(t >> 16),
		byte(t >> 8),
		byte(t),
	})
}

// Common OpenType tags
var (
	TagCmap = MakeTag('c', 'm', 'a', 'p')
	TagHead = MakeTag('h', 'e', 'a', 'd')
	TagHhea = MakeTag('h', 'h', 'e', 'a')
	TagHmtx = MakeTag('h', 'm', 't', 'x')
	TagMaxp = MakeTag('m', 'a', 'x', 'p')
	TagName = MakeTag('n', 'a', 'm', 'e')
	TagOS2  = MakeTag('O', 'S', '/', '2')
	TagPost = MakeTag('p', 'o', 's', 't')
	TagGlyf = MakeTag('g', 'l', 'y', 'f')
	TagLoca = MakeTag('l', 'o', 'c', 'a')
	TagGDEF = MakeTag('G', 'D', 'E', 'F')
	TagGSUB = MakeTag('G', 'S', 'U', 'B')
	TagGPOS = MakeTag('G', 'P', 'O', 'S')
	TagCvt  = MakeTag('c', 'v', 't', ' ')
	TagFpgm = MakeTag('f', 'p', 'g', 'm')
	TagPrep = MakeTag('p', 'r', 'e', 'p')
	TagGasp = MakeTag('g', 'a', 's', 'p')
	TagVhea = MakeTag('v', 'h', 'e', 'a')
	TagVmtx = MakeTag('v', 'm', 't', 'x')
	TagVORG = MakeTag('V', 'O', 'R', 'G')
)

// Parser provides methods for reading binary OpenType data.
type Parser struct {
	data []byte
	off  int
}

// NewParser creates a parser for the given data.
func NewParser(data []byte) *Parser {
	return &Parser{data: data}
}

// Data returns the underlying byte slice.
func (p *Parser) Data() []byte {
	return p.data
}

// Offset returns the current offset.
func (p *Parser) Offset() int {
	return p.off
}

// SetOffset sets the current offset.
func (p *Parser) SetOffset(off int) error {
	if off < 0 || off > len(p.data) {
		return ErrInvalidOffset
	}
	p.off = off
	return nil
}

// Remaining returns the number of bytes remaining.
func (p *Parser) Remaining() int {
	return len(p.data) - p.off
}

// Skip advances the offset by n bytes.
func (p *Parser) Skip(n int) error {
	if p.off+n > len(p.data) {
		return ErrInvalidOffset
	}
	p.off += n
	return nil
}

// Bytes returns n bytes at the current offset and advances.
func (p *Parser) Bytes(n int) ([]byte, error) {
	if p.off+n > len(p.data) {
		return nil, ErrInvalidOffset
	}
	b := p.data[p.off : p.off+n]
	p.off += n
	return b, nil
}

// U8 reads a uint8 and advances.
func (p *Parser) U8() (uint8, error) {
	if p.off >= len(p.data) {
		return 0, ErrInvalidOffset
	}
	v := p.data[p.off]
	p.off++
	return v, nil
}

// U16 reads a big-endian uint16 and advances.
func (p *Parser) U16() (uint16, error) {
	if p.off+2 > len(p.data) {
		return 0, ErrInvalidOffset
	}
	v := binary.BigEndian.Uint16(p.data[p.off:])
	p.off += 2
	return v, nil
}

// I16 reads a big-endian int16 and advances.
func (p *Parser) I16() (int16, error) {
	v, err := p.U16()
	return int16(v), err
}

// U32 reads a big-endian uint32 and advances.
func (p *Parser) U32() (uint32, error) {
	if p.off+4 > len(p.data) {
		return 0, ErrInvalidOffset
	}
	v := binary.BigEndian.Uint32(p.data[p.off:])
	p.off += 4
	return v, nil
}

// I32 reads a big-endian int32 and advances.
func (p *Parser) I32() (int32, error) {
	v, err := p.U32()
	return int32(v), err
}

// Tag reads a 4-byte tag and advances.
func (p *Parser) Tag() (Tag, error) {
	v, err := p.U32()
	return Tag(v), err
}

// U16At reads a big-endian uint16 at the given offset (doesn't advance).
func (p *Parser) U16At(off int) (uint16, error) {
	if off+2 > len(p.data) {
		return 0, ErrInvalidOffset
	}
	return binary.BigEndian.Uint16(p.data[off:]), nil
}

// I16At reads a big-endian int16 at the given offset (doesn't advance).
func (p *Parser) I16At(off int) (int16, error) {
	v, err := p.U16At(off)
	return int16(v), err
}

// U32At reads a big-endian uint32 at the given offset (doesn't advance).
func (p *Parser) U32At(off int) (uint32, error) {
	if off+4 > len(p.data) {
		return 0, ErrInvalidOffset
	}
	return binary.BigEndian.Uint32(p.data[off:]), nil
}

// SubParser returns a parser for a sub-range of the data.
func (p *Parser) SubParser(off, length int) (*Parser, error) {
	if off < 0 || length < 0 || off+length > len(p.data) {
		return nil, ErrInvalidOffset
	}
	return &Parser{data: p.data[off : off+length]}, nil
}

// SubParserFromOffset returns a parser starting at off to end of data.
func (p *Parser) SubParserFromOffset(off int) (*Parser, error) {
	if off < 0 || off > len(p.data) {
		return nil, ErrInvalidOffset
	}
	return &Parser{data: p.data[off:]}, nil
}
