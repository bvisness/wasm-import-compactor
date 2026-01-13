package parser

type MemType struct {
	Lim Limits
}

type TableType struct {
	ET  RefType
	Lim Limits
}

type GlobalType struct {
	Mut bool
	T   ValType
}

type AddressType int

const (
	ATI32 AddressType = iota
	ATI64
)

type Limits struct {
	AT       AddressType
	Min, Max uint64
	HasMax   bool
}

type ValType struct {
	isRef        bool
	numOrVecType TypeCode
	refType      RefType
}

func (vt ValType) IsNumType() bool {
	return !vt.isRef && vt.numOrVecType.IsNumType()
}

func (vt ValType) IsVecType() bool {
	return !vt.isRef && vt.numOrVecType.IsVecType()
}

func (vt ValType) IsRefType() bool {
	return vt.isRef
}

func (vt ValType) NumType() TypeCode {
	if !vt.IsNumType() {
		panic("valtype was not a numtype")
	}
	return vt.numOrVecType
}

func (vt ValType) VecType() TypeCode {
	if !vt.IsVecType() {
		panic("valtype was not a vectype")
	}
	return vt.numOrVecType
}

func (vt ValType) RefType() RefType {
	if !vt.IsRefType() {
		panic("valtype was not a reftype")
	}
	return vt.refType
}

type RefType struct {
	Null bool
	HT   TypeCode // may be an abstract heap type or a concrete one, depending on sign
}

type TypeCode int

const (
	// The hex bytes in here refer to the number's encoding in SLEB128.

	// numtype
	NT__last  TypeCode = NTI32
	NTI32     TypeCode = -1 // 0x7F
	NTI64     TypeCode = -2 // 0x7E
	NTF32     TypeCode = -3 // 0x7D
	NTF64     TypeCode = -4 // 0x7C
	NT__first TypeCode = NTF64

	// vectype
	VT__last  TypeCode = VTV128
	VTV128    TypeCode = -5 // 0x7B
	VT__first TypeCode = VTV128

	// heaptype (abstract, because positive values mean concrete type index)
	HT__last   TypeCode = HTNoExn
	HTNoExn    TypeCode = -12 // 0x74
	HTNoFunc   TypeCode = -13 // 0x73
	HTNoExtern TypeCode = -14 // 0x72
	HTNone     TypeCode = -15 // 0x71
	HTFunc     TypeCode = -16 // 0x70
	HTExtern   TypeCode = -17 // 0x6F
	HTAny      TypeCode = -18 // 0x6E
	HTEq       TypeCode = -19 // 0x6D
	HTI31      TypeCode = -20 // 0x6C
	HTStruct   TypeCode = -21 // 0x6B
	HTArray    TypeCode = -22 // 0x6A
	HTExn      TypeCode = -23 // 0x69
	HT__first  TypeCode = HTExn

	// Sentinel bytes indicating that a ref type's heap type follows.
	RTNonNull TypeCode = -28 // 0x64
	RTNull    TypeCode = -29 // 0x63
)

func (tc TypeCode) IsNumType() bool {
	return NT__first <= tc && tc <= NT__last
}

func (tc TypeCode) IsVecType() bool {
	return VT__first <= tc && tc <= VT__last
}

func (tc TypeCode) IsHeapType() bool {
	return tc.IsAbstractHeapType() || tc.IsConcreteHeapType()
}

func (tc TypeCode) IsAbstractHeapType() bool {
	return HT__first <= tc && tc <= HT__last
}

func (tc TypeCode) IsConcreteHeapType() bool {
	return tc > 0
}
