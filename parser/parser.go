package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"

	"github.com/bvisness/wasm-import-compactor/leb128"
	"github.com/bvisness/wasm-import-compactor/utils"
)

type Parser struct {
	r   readPeeker
	Cur int

	// It is kind of jank that there can only be one recording at a time, but it is satisfactory for now.
	record         bool
	originalReader readPeeker
}

func NewParser(r io.Reader) Parser {
	return Parser{
		r:   bufio.NewReader(r),
		Cur: 0,
	}
}

func NewParserFromBytes(b []byte, at int) Parser {
	return Parser{
		r:   bufio.NewReader(bytes.NewBuffer(b)),
		Cur: at,
	}
}

func (p *Parser) StartRecording() {
	if p.record {
		panic("already recording")
	}
	p.record = true

	p.originalReader = p.r
	p.r = &recordingReader{r: p.r}
}

func (p *Parser) StopRecording() []byte {
	if !p.record {
		panic("not recording")
	}
	p.record = false
	res := p.r.(*recordingReader).buf
	p.r = p.originalReader
	p.originalReader = nil
	return res
}

func (p *Parser) ReadN(thing string, n int) ([]byte, error) {
	at := p.Cur
	bytes := make([]byte, n)
	nRead, err := io.ReadFull(p.r, bytes)
	if err != nil {
		return nil, fmt.Errorf("%s at offset %d: %w", thing, at, err)
	}
	p.Cur += nRead
	return bytes, nil
}

func (p *Parser) PeekByte(thing string) (byte, error) {
	at := p.Cur
	bytes, err := p.r.Peek(1)
	if err != nil {
		return 0, fmt.Errorf("%s at offset %d: %w", thing, at, err)
	}
	return bytes[0], nil
}

func (p *Parser) ReadByte(thing string) (byte, error) {
	at := p.Cur
	var b [1]byte
	_, err := io.ReadFull(p.r, b[:])
	if err != nil {
		return 0, fmt.Errorf("%s at offset %d: %w", thing, at, err)
	}
	p.Cur += 1
	return b[0], nil
}

// Reads a byte and interprets it as a signed LEB128 integer.
func (p *Parser) ReadByteAsS64(thing string) (int64, error) {
	at := p.Cur
	b, err := p.ReadByte(thing)
	if err != nil {
		return 0, err
	}
	v, _, err := leb128.DecodeS64(bytes.NewReader([]byte{b}))
	if err != nil {
		return 0, fmt.Errorf("%s at offset %d: %w", thing, at, err)
	}
	return v, nil
}

func (p *Parser) ReadU32(thing string) (uint32, int, error) {
	v, n, err := p.ReadU64(thing)
	return uint32(v), n, err
}

func (p *Parser) ReadU64(thing string) (uint64, int, error) {
	at := p.Cur
	v, n, err := leb128.DecodeU64(p.r)
	if err != nil {
		return 0, n, fmt.Errorf("%s at offset %d: %w", thing, at, err)
	}
	p.Cur += n
	return v, n, nil
}

func (p *Parser) ReadS32(thing string) (int32, int, error) {
	v, n, err := p.ReadS64(thing)
	return int32(v), n, err
}

func (p *Parser) ReadS64(thing string) (int64, int, error) {
	at := p.Cur
	v, n, err := leb128.DecodeS64(p.r)
	if err != nil {
		return 0, n, fmt.Errorf("%s at offset %d: %w", thing, at, err)
	}
	p.Cur += n
	return v, n, nil
}

func (p *Parser) ReadF32(thing string) (float32, error) {
	b, err := p.ReadN(thing, 4)
	if err != nil {
		return 0, err
	}
	bits := uint32(b[0])<<0 |
		uint32(b[1])<<8 |
		uint32(b[2])<<16 |
		uint32(b[3])<<24
	return math.Float32frombits(bits), nil
}

func (p *Parser) ReadF64(thing string) (float64, error) {
	b, err := p.ReadN(thing, 8)
	if err != nil {
		return 0, err
	}
	bits := uint64(b[0])<<0 |
		uint64(b[1])<<8 |
		uint64(b[2])<<16 |
		uint64(b[3])<<24 |
		uint64(b[4])<<32 |
		uint64(b[5])<<40 |
		uint64(b[6])<<48 |
		uint64(b[7])<<56
	return math.Float64frombits(bits), nil
}

func (p *Parser) ReadName(thing string) (string, error) {
	n, _, err := p.ReadU32(thing)
	if err != nil {
		return "", err
	}
	name, err := p.ReadN(thing, int(n))
	if err != nil {
		return "", err
	}
	return string(name), nil
}

func (p *Parser) ReadTableType(thing string) (TableType, error) {
	et, err := p.ReadRefType(fmt.Sprintf("element type for %s", thing))
	if err != nil {
		return TableType{}, err
	}
	lim, err := p.ReadLimits(fmt.Sprintf("limits for %s", thing))
	if err != nil {
		return TableType{}, err
	}
	return TableType{
		ET:  et,
		Lim: lim,
	}, nil
}

func (p *Parser) ReadMemType(thing string) (MemType, error) {
	lim, err := p.ReadLimits(fmt.Sprintf("limits for %s", thing))
	if err != nil {
		return MemType{}, err
	}
	return MemType{lim}, nil
}

func (p *Parser) ReadGlobalType(thing string) (GlobalType, error) {
	t, err := p.ReadValType(thing)
	if err != nil {
		return GlobalType{}, err
	}
	mut, err := p.ReadByte(thing)
	if err != nil {
		return GlobalType{}, err
	}

	return GlobalType{
		Mut: mut == 0x01,
		T:   t,
	}, nil
}

func (p *Parser) ReadTagType(thing string) (uint32, error) {
	_, err := p.ReadByte(thing)
	if err != nil {
		return 0, err
	}
	idx, _, err := p.ReadU32(thing)
	return idx, err
}

func (p *Parser) ReadValType(thing string) (ValType, error) {
	at := p.Cur

	// The encoding for value types is carefully constructed so that the numbers
	// are interpretable as negative signed LEB128 integers. But, the spec also
	// is clear that they are one byte. We therefore parse one byte but interpret
	// it as SLEB128 for the sake of our logic.
	t, err := p.ReadByteAsS64(thing)
	if err != nil {
		return ValType{}, err
	}

	switch tc := TypeCode(t); tc {
	case RTNonNull, RTNull:
		ht, err := p.ReadHeapType(thing)
		if err != nil {
			return ValType{}, err
		}
		return ValType{
			isRef: true,
			refType: RefType{
				Null: tc == RTNull,
				HT:   ht,
			},
		}, nil
	default:
		tc := TypeCode(t)
		if tc.IsNumType() || tc.IsVecType() {
			return ValType{
				numOrVecType: tc,
			}, nil
		} else if tc.IsHeapType() {
			return ValType{
				isRef: true,
				refType: RefType{
					Null: true,
					HT:   tc,
				},
			}, nil
		} else {
			return ValType{}, fmt.Errorf("%s at offset %d: invalid valtype", thing, at)
		}
	}
}

func (p *Parser) ReadRefType(thing string) (RefType, error) {
	kind, err := p.PeekByte(thing)
	if err != nil {
		return RefType{}, err
	}

	null := false
	if kind == 0x64 || kind == 0x63 {
		utils.Must1(p.ReadByte(thing))
		null = kind == 0x63
	}

	ht, err := p.ReadHeapType(thing)
	if err != nil {
		return RefType{}, err
	}

	return RefType{
		Null: null,
		HT:   ht,
	}, nil
}

func (p *Parser) ReadHeapType(thing string) (TypeCode, error) {
	at := p.Cur
	kind, n, err := p.ReadS64(thing)
	if err != nil {
		return 0, err
	}
	if kind < 0 && n != 1 {
		return 0, fmt.Errorf("%s at offset %d: invalid abstract heap type", thing, at)
	}
	ht := TypeCode(kind)
	if !ht.IsHeapType() {
		return 0, fmt.Errorf("%s at offset %d: invalid heap type", thing, at)
	}
	return ht, nil
}

func (p *Parser) ReadLimits(thing string) (Limits, error) {
	flags, err := p.ReadByte("limits flags")
	if err != nil {
		return Limits{}, err
	}

	min, _, err := p.ReadU64("limits min")
	if err != nil {
		return Limits{}, err
	}

	lim := Limits{Min: min}
	if flags&0b001 > 0 {
		max, _, err := p.ReadU64("limits max")
		if err != nil {
			return Limits{}, err
		}
		lim.HasMax = true
		lim.Max = max
	}
	if flags&0b100 > 0 {
		lim.AT = ATI64
	}

	return lim, nil
}

func (p *Parser) ReadExpr(thing string) (res []byte, err error) {
	p.StartRecording()
	defer func() {
		res = p.StopRecording()
	}()

	depth := 0

instrs:
	for {
		b1, err := p.ReadByte(thing)
		if err != nil {
			return nil, err
		}

		switch b1 {
		case 0x0B: // end
			if depth == 0 {
				break instrs
			}
			depth -= 1
		case 0x41: // i32.const n
			_, _, err := p.ReadU32(fmt.Sprintf("i32.const in %s", thing))
			if err != nil {
				return nil, err
			}
		case 0x42: // i64.const n
			_, _, err := p.ReadU64(fmt.Sprintf("i64.const in %s", thing))
			if err != nil {
				return nil, err
			}
		case 0x43: // f32.const z
			_, err := p.ReadF32(fmt.Sprintf("f32.const in %s", thing))
			if err != nil {
				return nil, err
			}
		case 0x44: // f64.const z
			_, err := p.ReadF64(fmt.Sprintf("f64.const in %s", thing))
			if err != nil {
				return nil, err
			}

		case 0x6A: // i32.add
		case 0x6B: // i32.sub
		case 0x6C: // i32.mul

		case 0x7C: // i64.add
		case 0x7D: // i64.sub
		case 0x7E: // i64.mul

		// case 0xD0: // ref.null

		default:
			return nil, fmt.Errorf("%s at offset %d: unknown opcode %x", thing, p.Cur-1, b1)
		}
	}

	return nil, nil
}

func (p *Parser) Expect(thing string, bytes []byte) error {
	at := p.Cur
	actual, err := p.ReadN(thing, len(bytes))
	if err != nil {
		return err
	}
	if err := p.AssertBytesEqual(at, actual, bytes); err != nil {
		return fmt.Errorf("reading %s: %w", thing, err)
	}
	return nil
}

func (p *Parser) AssertBytesEqual(at int, actual, expected []byte) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("at offset %d: expected bytes %+v but got %+v", at, expected, actual)
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return fmt.Errorf("at offset %d: expected bytes %+v but got %+v", at, expected, actual)
		}
	}
	return nil
}
