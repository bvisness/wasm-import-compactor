package compactor

import (
	"cmp"
	"encoding/csv"
	"errors"
	"io"
	"slices"
	"strconv"

	"github.com/bvisness/wasm-import-compactor/leb128"
	"github.com/bvisness/wasm-import-compactor/parser"
	"github.com/bvisness/wasm-import-compactor/utils"
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

func CompactImports(
	fileName string,
	wasm io.Reader,
	enableEncoding2 bool,
	out io.Writer,
	countsOut io.Writer,
	minPossibleOut io.Writer,
) error {
	p := parser.NewParser(wasm)
	importCounts := map[string]int{}
	numImportsTotal := 0
	minPossible := 0

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
				importCounts[modName]++
				numImportsTotal++
			}

			// Emit new import section
			groups := rleImports(imports, enableEncoding2)
			out.Write([]byte{0x02})
			var outBody []byte
			outBody = appendU32(outBody, uint32(len(groups)))
			for _, group := range groups {
				outBody = append(outBody, group.Encode()...)
			}
			out.Write(leb128.EncodeU64(uint64(len(outBody))))
			out.Write(outBody)

			// Estimate the min possible import section size by sorting imports first
			// by module name and externtype, then re-encoding.
			slices.SortStableFunc(imports, func(a, b Import) int {
				return cmp.Or(
					cmp.Compare(a.ModName, b.ModName),
					slices.Compare(a.Externtype, b.Externtype),
				)
			})
			sortedGroups := rleImports(imports, enableEncoding2)
			minPossible += lebLen(len(sortedGroups))
			for _, group := range sortedGroups {
				minPossible += len(group.Encode())
			}

		// Pass through all other sections
		default:
			out.Write([]byte{sectionId})
			out.Write(leb128.EncodeU64(uint64(len(body))))
			out.Write(body)
		}
	}

	// Write counts
	{
		w := csv.NewWriter(countsOut)
		for modName, count := range importCounts {
			w.Write([]string{fileName, modName, strconv.Itoa(count), strconv.Itoa(numImportsTotal)})
		}
		w.Flush()
		utils.Must(w.Error())
	}

	// Write min possible import section size (est.)
	{
		w := csv.NewWriter(minPossibleOut)
		w.Write([]string{strconv.Itoa(minPossible)})
		w.Flush()
		utils.Must(w.Error())
	}

	return nil
}

// rleImports groups the imports into blocks for the new encodings, always
// keeping them in their original order. No encoding can span module names, so
// each run of imports from the same module is laid out on its own.
func rleImports(imports []Import, enableEncoding2 bool) []ImportEncoder {
	var groups []ImportEncoder
	for _, module := range maximalRuns(imports, func(a, b Import) bool {
		return a.ModName == b.ModName
	}) {
		groups = append(groups, groupOneModule(module, enableEncoding2)...)
	}
	return groups
}

// What to do with one stretch of same-typed imports.
type stretchPlan int

const (
	planGroupWithNeighbors stretchPlan = iota // share a GroupSameModule with the stretches beside it
	planOwnGroup                              // a GroupSameModuleAndType of its own
	planPlain                                 // one plain Import per item
)

// groupOneModule lays out a run of imports that all share a module name,
// choosing whichever encodings actually cost the fewest bytes.
//
// Only whole stretches of same-typed imports are worth considering as a unit: a
// GroupSameModuleAndType cannot span externtypes, and growing one to cover its
// entire stretch always pays for itself, because every import it absorbs saves
// an externtype and costs at most a byte of item count. So the layout is decided
// one stretch at a time.
//
// The catch is that pulling a stretch out of the middle of a module's imports
// splits the GroupSameModule around it in two, paying for the module name an
// extra time. The choices are therefore not independent, and we run a small
// dynamic program over the stretches, tracking the cheapest layout both with
// and without a GroupSameModule left open for the next stretch to join.
//
// The one wrinkle is that a group's item count is charged along the cheapest
// path only, so where a group's count would cross a LEB128 boundary (128 items,
// then 16384, ...) the layout can be a byte or two off the true optimum.
func groupOneModule(imports []Import, enableEncoding2 bool) []ImportEncoder {
	modName := imports[0].ModName
	// Module name, empty item name, and the encoding byte.
	blockOverhead := nameLen(modName) + nameLen("") + 1

	stretches := maximalRuns(imports, func(a, b Import) bool {
		return slices.Equal(a.Externtype, b.Externtype)
	})

	// closed[t] is the cost of the cheapest layout of stretches[:t+1] that ends
	// with no GroupSameModule open; open[t] is the cheapest that does, and
	// openLen[t] counts the imports in that group so far. The other slices
	// remember the decisions so we can walk them back afterwards.
	closed := make([]int, len(stretches))
	closedPlan := make([]stretchPlan, len(stretches))
	closedAfterOpen := make([]bool, len(stretches))
	open := make([]int, len(stretches))
	openLen := make([]int, len(stretches))
	openExtends := make([]bool, len(stretches))

	for t, stretch := range stretches {
		externtypeLen := len(stretch[0].Externtype)
		nameBytes := 0
		for _, imp := range stretch {
			nameBytes += nameLen(imp.ItemName)
		}

		// Joining a GroupSameModule pays for an externtype per import, but not
		// for that group's own overhead, which is paid when it is opened.
		join := nameBytes + len(stretch)*externtypeLen
		ownGroup := blockOverhead + externtypeLen + lebLen(len(stretch)) + nameBytes
		plainImports := len(stretch) * (nameLen(modName) + externtypeLen)

		// Standing apart from its neighbors, cheapest way.
		apart, plan := plainImports+nameBytes, planPlain
		if enableEncoding2 && ownGroup < apart {
			apart, plan = ownGroup, planOwnGroup
		}

		if t == 0 {
			closed[t], closedPlan[t] = apart, plan
			open[t] = blockOverhead + lebLen(len(stretch)) + join
			openLen[t] = len(stretch)
			continue
		}

		// Standing apart also closes any open group, which costs nothing
		// extra - that group's bytes were paid as it was opened and extended.
		before, afterOpen := closed[t-1], false
		if open[t-1] < before {
			before, afterOpen = open[t-1], true
		}
		closed[t], closedPlan[t], closedAfterOpen[t] = before+apart, plan, afterOpen

		// Extend the open group, or close it and start a new one. Extending
		// also pays for any extra bytes in the group's item count.
		extend := open[t-1] + join + lebLen(openLen[t-1]+len(stretch)) - lebLen(openLen[t-1])
		startNew := closed[t-1] + blockOverhead + lebLen(len(stretch)) + join
		if extend <= startNew {
			open[t], openLen[t], openExtends[t] = extend, openLen[t-1]+len(stretch), true
		} else {
			open[t], openLen[t] = startNew, len(stretch)
		}
	}

	// Walk the decisions back to front to recover the layout.
	plans := make([]stretchPlan, len(stretches))
	inOpenGroup := open[len(stretches)-1] < closed[len(stretches)-1]
	for t := len(stretches) - 1; t >= 0; t-- {
		if inOpenGroup {
			plans[t] = planGroupWithNeighbors
			inOpenGroup = openExtends[t]
		} else {
			plans[t] = closedPlan[t]
			inOpenGroup = closedAfterOpen[t]
		}
	}

	var groups []ImportEncoder
	for t := 0; t < len(stretches); {
		switch plans[t] {
		case planGroupWithNeighbors:
			// Consecutive stretches always belong to the same group; the
			// layout never puts two GroupSameModules next to each other,
			// since merging them would save the module name.
			var items []GroupSameModuleItem
			for ; t < len(stretches) && plans[t] == planGroupWithNeighbors; t++ {
				for _, imp := range stretches[t] {
					items = append(items, GroupSameModuleItem{
						Name:       imp.ItemName,
						Externtype: imp.Externtype,
					})
				}
			}
			groups = append(groups, GroupSameModule{ModName: modName, Items: items})
		case planOwnGroup:
			var items []string
			for _, imp := range stretches[t] {
				items = append(items, imp.ItemName)
			}
			groups = append(groups, GroupSameModuleAndType{
				ModName:    modName,
				Externtype: stretches[t][0].Externtype,
				Items:      items,
			})
			t++
		case planPlain:
			for _, imp := range stretches[t] {
				groups = append(groups, imp)
			}
			t++
		}
	}
	return groups
}

// maximalRuns splits imports into maximal runs of consecutive imports that same
// reports as belonging together. Each import is compared against the first
// import of the run it might join.
func maximalRuns(imports []Import, same func(a, b Import) bool) [][]Import {
	var runs [][]Import
	for i := 0; i < len(imports); {
		j := i + 1
		for j < len(imports) && same(imports[i], imports[j]) {
			j++
		}
		runs = append(runs, imports[i:j])
		i = j
	}
	return runs
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

func lebLen(n int) int {
	return len(leb128.EncodeU64(uint64(n)))
}

func nameLen(name string) int {
	return lebLen(len(name)) + len(name)
}
