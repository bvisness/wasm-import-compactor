package compactor

import (
	"errors"
	"io"
	"slices"

	"github.com/bvisness/wasm-import-compactor/leb128"
	"github.com/bvisness/wasm-import-compactor/parser"
)

type ImportEncoder interface {
	Encode() []byte
}

type Import struct {
	ModName, ItemName string
	Externtype        []byte
}

func (i Import) Encode() []byte {
	var res []byte
	res = appendName(res, i.ModName)
	res = appendName(res, i.ItemName)
	res = append(res, i.Externtype...)
	return res
}

type GroupSameModule struct {
	ModName string
	Items   []GroupSameModuleItem
}

type GroupSameModuleItem struct {
	Name       string
	Externtype []byte
}

func (g GroupSameModule) Encode() []byte {
	var res []byte
	res = appendName(res, g.ModName)
	res = appendName(res, "")
	res = append(res, 0x7F)
	res = appendU32(res, uint32(len(g.Items)))
	for _, item := range g.Items {
		res = appendName(res, item.Name)
		res = append(res, item.Externtype...)
	}
	return res
}

type GroupSameModuleAndType struct {
	ModName    string
	Externtype []byte
	Items      []string
}

func (g GroupSameModuleAndType) Encode() []byte {
	var res []byte
	res = appendName(res, g.ModName)
	res = appendName(res, "")
	res = append(res, 0x7E)
	res = append(res, g.Externtype...)
	res = appendU32(res, uint32(len(g.Items)))
	for _, item := range g.Items {
		res = appendName(res, item)
	}
	return res
}

func CompactImports(wasm io.Reader, out io.Writer) error {
	p := parser.NewParser(wasm)

	if err := p.Expect("magic number", []byte{0, 'a', 's', 'm'}); err != nil {
		return err
	}
	if err := p.Expect("version number", []byte{1, 0, 0, 0}); err != nil {
		return err
	}

	out.Write([]byte{0, 'a', 's', 'm'})
	out.Write([]byte{1, 0, 0, 0})

	for {
		sectionId, err := p.ReadByte("section id")
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		sectionSize, _, err := p.ReadU32("section size")
		if err != nil {
			return err
		}

		bodyStart := p.Cur
		body, err := p.ReadN("section contents", int(sectionSize))
		if err != nil {
			return err
		}

		switch sectionId {
		case 2: // import section
			p := parser.NewParserFromBytes(body, bodyStart)

			var imports []Import

			numImports, _, err := p.ReadU32("num imports")
			if err != nil {
				return err
			}
			for range numImports {
				modName, err := p.ReadName("import module")
				if err != nil {
					return err
				}

				itemName, err := p.ReadName("import name")
				if err != nil {
					return err
				}

				p.StartRecording()
				importType, err := p.ReadByte("import type")
				if err != nil {
					return err
				}
				switch importType {
				case 0x00: // function
					_, _, err := p.ReadU32("type of imported function")
					if err != nil {
						return err
					}
				case 0x01: // table
					_, err := p.ReadTableType("type of imported table")
					if err != nil {
						return err
					}
				case 0x02: // memory
					_, err := p.ReadMemType("type of imported memory")
					if err != nil {
						return err
					}
				case 0x03: // global
					_, err := p.ReadGlobalType("type of imported global")
					if err != nil {
						return err
					}
				case 0x04: // tag
					_, err := p.ReadTagType("type of imported tag")
					if err != nil {
						return err
					}
				}
				externtype := p.StopRecording()

				imports = append(imports, Import{modName, itemName, externtype})
			}

			// "RLE" the imports into chunks for the new encodings
			var groups []ImportEncoder
			for i := 0; i < len(imports); {
				imp := imports[i]

				sameModuleItems := []GroupSameModuleItem{{
					Name:       imp.ItemName,
					Externtype: imp.Externtype,
				}}
				sameModuleAndTypeItems := []string{imp.ItemName}
				doneAddingSameModuleAndType := false
				for j := i + 1; j < len(imports); j++ {
					next := imports[j]
					sameMod := imp.ModName == next.ModName
					sameType := slices.Equal(imp.Externtype, next.Externtype)

					if !sameMod {
						break
					}

					sameModuleItems = append(sameModuleItems, GroupSameModuleItem{
						Name:       next.ItemName,
						Externtype: next.Externtype,
					})
					if sameType {
						if !doneAddingSameModuleAndType {
							sameModuleAndTypeItems = append(sameModuleAndTypeItems, next.ItemName)
						}
					} else {
						doneAddingSameModuleAndType = true
					}
				}

				if len(sameModuleItems) < len(sameModuleAndTypeItems) {
					panic("logic bug - more items with the same module and type, than items with the same module")
				}

				if len(sameModuleAndTypeItems) > 2 {
					groups = append(groups, GroupSameModuleAndType{
						ModName:    imp.ModName,
						Externtype: imp.Externtype,
						Items:      sameModuleAndTypeItems,
					})
					i += len(sameModuleAndTypeItems)
				} else if len(sameModuleItems) > 1 {
					groups = append(groups, GroupSameModule{
						ModName: imp.ModName,
						Items:   sameModuleItems,
					})
					i += len(sameModuleItems)
				} else {
					groups = append(groups, imp)
					i += 1
				}
			}

			// Emit new import section
			out.Write([]byte{0x02})
			var outBody []byte
			outBody = appendU32(outBody, uint32(len(groups)))
			for _, group := range groups {
				outBody = append(outBody, group.Encode()...)
			}
			out.Write(leb128.EncodeU64(uint64(len(outBody))))
			out.Write(outBody)

		// Pass through all other sections
		default:
			out.Write([]byte{sectionId})
			out.Write(leb128.EncodeU64(uint64(len(body))))
			out.Write(body)
		}
	}

	return nil
}

// --------------------------------
// Output

func appendName(s []byte, name string) []byte {
	s = append(s, leb128.EncodeU64(uint64(len(name)))...)
	s = append(s, []byte(name)...)
	return s
}

func appendU32(s []byte, n uint32) []byte {
	return append(s, leb128.EncodeU64(uint64(n))...)
}
