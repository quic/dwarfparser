// =============================================================================
//  @@-COPYRIGHT-START-@@
//
//  Copyright (c) 2024, Qualcomm Innovation Center, Inc. All rights reserved.
//
//  Redistribution and use in source and binary forms, with or without
//  modification, are permitted provided that the following conditions are met:
//
//  1. Redistributions of source code must retain the above copyright notice,
//     this list of conditions and the following disclaimer.
//
//  2. Redistributions in binary form must reproduce the above copyright notice,
//     this list of conditions and the following disclaimer in the documentation
//     and/or other materials provided with the distribution.
//
//  3. Neither the name of the copyright holder nor the names of its contributors
//     may be used to endorse or promote products derived from this software
//     without specific prior written permission.
//
//  THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
//  AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
//  IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
//  ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
//  LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
//  CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
//  SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
//  INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
//  CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
//  ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
//  POSSIBILITY OF SUCH DAMAGE.
//
//  SPDX-License-Identifier: BSD-3-Clause
//
//  @@-COPYRIGHT-END-@@
// =============================================================================

package dwarfparser

import (
	"debug/dwarf"
	"fmt"
	"sort"

	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	allSubroutinesCMap = cmap.New[[]*DWARFFunction]()
	compileUnitsCMap   = cmap.New[[]*DWARFCompileUnit]()
)

func GetCompileUnitByAddr(path string, pc uint64) (*DWARFCompileUnit, error) {
	cus, err := FindAllCompileUnits(path)
	if err != nil {
		return nil, err
	}
	type Range struct {
		Start uint64
		End   uint64
		CU    *DWARFCompileUnit
	}
	var ranges []*Range
	for _, cu := range cus {
		for _, r := range cu.Ranges {
			ranges = append(ranges, &Range{
				Start: r[0],
				End:   r[1],
				CU:    cu,
			})
		}
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].End < ranges[j].End
	})
	idx := sort.Search(len(ranges), func(i int) bool {
		return pc < ranges[i].Start
	})
	if idx == 0 || idx > len(ranges) {
		return nil, fmt.Errorf("not find CompileUnit for pc 0x%x", pc)
	}
	return ranges[idx-1].CU, nil
}

func FindAllCompileUnits(path string) ([]*DWARFCompileUnit, error) {
	if e, ok := compileUnitsCMap.Get(path); ok {
		return e, nil
	}
	di, err := DWARF(path)
	if err != nil {
		return nil, err
	}
	var cus []*DWARFCompileUnit
	for r := di.Reader(); ; {
		ent, err := r.Next()
		if err != nil {
			return nil, err
		}
		if ent == nil {
			break
		}
		if ent.Tag != dwarf.TagCompileUnit {
			return nil, fmt.Errorf("found unexpected tag %v on top level", ent.Tag)
		}
		attrName := ent.Val(dwarf.AttrName)
		if attrName == nil {
			continue
		}
		attrCompDir := ent.Val(dwarf.AttrCompDir)
		ranges, err := di.Ranges(ent)
		if err != nil {
			return nil, err
		}
		cus = append(cus, &DWARFCompileUnit{
			FilePath: path,
			Dwarf:    di,
			Entry:    ent,
			Name:     attrName.(string),
			CompDir:  attrCompDir.(string),
			Ranges:   ranges,
		})
		r.SkipChildren()
	}
	var finalCUs []*DWARFCompileUnit
	for _, cu := range cus {
		if len(cu.Ranges) == 0 {
			continue
		}
		finalCUs = append(finalCUs, cu)
	}
	sort.Slice(finalCUs, func(i, j int) bool {
		return finalCUs[i].Entry.Offset < finalCUs[j].Entry.Offset
	})
	compileUnitsCMap.Set(path, finalCUs)
	return finalCUs, nil
}

func FindAllFuncsInCUByAddr(path string, pc uint64) ([]*DWARFFunction, error) {
	cu, err := GetCompileUnitByAddr(path, pc)
	if err != nil {
		return nil, err
	}
	k := fmt.Sprintf("%v-%v", path, cu.Entry.Offset)
	if e, ok := allSubroutinesCMap.Get(k); ok {
		return e, nil
	}
	funcs, err := cu.findAllFuncs()
	if err != nil {
		return nil, err
	}
	var finalFuncs []*DWARFFunction
	for _, f := range funcs {
		if len(f.Ranges) == 0 {
			continue
		}
		finalFuncs = append(finalFuncs, f)
	}
	allSubroutinesCMap.Set(k, finalFuncs)
	return finalFuncs, nil
}

func FindAllFuncs(path string) ([]*DWARFFunction, error) {
	if e, ok := allSubroutinesCMap.Get(path); ok {
		return e, nil
	}
	type funcResult struct {
		funcs []*DWARFFunction
		err   error
	}
	var funcs []*DWARFFunction
	cus, err := FindAllCompileUnits(path)
	if err != nil {
		return nil, err
	}
	funcC := make(chan funcResult, len(cus))
	for _, cu := range cus {
		go func(cu *DWARFCompileUnit) {
			var res funcResult
			funcs1, err := cu.findAllFuncs()
			if err != nil {
				res.err = err
			}
			res.funcs = append(res.funcs, funcs1...)
			funcC <- res
		}(cu)
	}
	for _, cu := range cus {
		res := <-funcC
		if err := res.err; err != nil {
			return nil, err
		}
		k := fmt.Sprintf("%v-%v", path, cu.Entry.Offset)
		allSubroutinesCMap.Set(k, res.funcs)
		funcs = append(funcs, res.funcs...)
	}
	allSubroutinesCMap.Set(path, funcs)
	return funcs, nil
}

func (cu *DWARFCompileUnit) GetSubprogramByAddr(pc uint64) (*DWARFFunction, error) {
	var sp *DWARFFunction
	first := true
	depth := 0
	r := cu.Dwarf.Reader()
	r.Seek(cu.Entry.Offset)
	for {
		ent, err := r.Next()
		if err != nil {
			return nil, err
		}
		if ent == nil {
			break
		}
		if ent.Tag == dwarf.TagCompileUnit {
			if first {
				first = false
				depth++
				continue
			}
			break
		} else if ent.Tag == dwarf.TagSubprogram {
			f, err := cu.parseSubprogram(ent, depth)
			if err != nil {
				return nil, err
			}
			if f == nil {
				continue
			}
			if len(f.Ranges) == 0 {
				continue
			}
			if pc >= f.Ranges[0][0] && pc < f.Ranges[len(f.Ranges)-1][1] {
				sp = f
				break
			}
		} else if ent.Tag == 0 {
			depth--
		}
		if ent.Children {
			depth++
		}
	}
	return sp, nil
}

func (sp *DWARFFunction) GetSubroutinesBySubprogram() ([]*DWARFFunction, error) {
	var funcs []*DWARFFunction
	cu := sp.DwarfCompileUnit
	first := true
	depth := 0
	r := cu.Dwarf.Reader()
	r.Seek(sp.Offset)
	for {
		ent, err := r.Next()
		if err != nil {
			return nil, err
		}
		if ent == nil {
			break
		}
		if ent.Tag == dwarf.TagSubprogram {
			if first {
				first = false
				depth = sp.Depth
				continue
			}
			break
		} else if ent.Tag == dwarf.TagInlinedSubroutine {
			attrAbstractOrigin := ent.Val(dwarf.AttrAbstractOrigin)
			if attrAbstractOrigin == nil {
				continue
			}
			f, err := cu.parseSubroutine(ent, depth)
			if err != nil {
				return nil, err
			}
			if f == nil {
				continue
			}
			if len(f.Ranges) == 0 {
				continue
			}
			funcs = append(funcs, f)
		} else if ent.Tag == 0 {
			depth--
		}
		if ent.Children {
			depth++
		}
	}
	return funcs, nil
}

func (cu *DWARFCompileUnit) findAllFuncs() ([]*DWARFFunction, error) {
	var funcs []*DWARFFunction
	first := true
	depth := 0
	r := cu.Dwarf.Reader()
	r.Seek(cu.Entry.Offset)
	for {
		ent, err := r.Next()
		if err != nil {
			return nil, err
		}
		if ent == nil {
			break
		}
		if ent.Tag == dwarf.TagCompileUnit {
			if first {
				first = false
				goto loop
			}
			break
		} else if ent.Tag == dwarf.TagSubprogram {
			f, err := cu.parseSubprogram(ent, depth)
			if err != nil {
				return nil, err
			}
			if f == nil {
				goto loop
			}
			funcs = append(funcs, f)
		} else if ent.Tag == dwarf.TagInlinedSubroutine {
			attrAbstractOrigin := ent.Val(dwarf.AttrAbstractOrigin)
			if attrAbstractOrigin == nil {
				goto loop
			}
			f, err := cu.parseSubroutine(ent, depth)
			if err != nil {
				return nil, err
			}
			if f == nil {
				goto loop
			}
			funcs = append(funcs, f)
		} else if ent.Tag == 0 {
			depth--
		}
	loop:
		if ent.Children {
			depth++
		}
	}
	var finalFuncs []*DWARFFunction
	for _, v := range funcs {
		if len(v.Ranges) == 0 {
			continue
		}
		finalFuncs = append(finalFuncs, v)
	}
	return finalFuncs, nil
}

func (cu *DWARFCompileUnit) parseSubprogram(ent *dwarf.Entry, depth int) (*DWARFFunction, error) {
	attrName := ent.Val(dwarf.AttrName)
	attrAbstractOrigin := ent.Val(dwarf.AttrAbstractOrigin)
	var decFile string
	var decLine int
	var err error

	if attrAbstractOrigin != nil {
		ent1, err := cu.getEntryByOffset(attrAbstractOrigin.(dwarf.Offset))
		if err != nil {
			return nil, err
		}
		attrName = ent1.Val(dwarf.AttrName)
		decFile, err = cu.getFilenameByIndex(int(ent1.Val(dwarf.AttrDeclFile).(int64)))
		if err != nil {
			return nil, err
		}
	}
	if attrName == nil {
		attrName = ent.Val(dwarf.AttrName)
	}
	if attrName == nil {
		return nil, nil
	}
	if decFile == "" && ent.Val(dwarf.AttrDeclFile) != nil {
		decFile, err = cu.getFilenameByIndex(int(ent.Val(dwarf.AttrDeclFile).(int64)))
		if err != nil {
			return nil, err
		}
	}
	attrDecLine := ent.Val(dwarf.AttrDeclLine)
	if attrDecLine != nil {
		decLine = int(attrDecLine.(int64))
	}
	ranges, err := cu.Dwarf.Ranges(ent)
	if err != nil {
		return nil, err
	}
	inline := false
	attrInline := ent.Val(dwarf.AttrInline)
	if attrInline != nil {
		inline = true
	}
	f := &DWARFFunction{
		DwarfCompileUnit: cu,
		Type:             dwarf.TagSubprogram,
		Name:             attrName.(string),
		Ranges:           ranges,
		DeclFile:         decFile,
		DeclLine:         decLine,
		Inline:           inline,
		Offset:           ent.Offset,
		Depth:            depth,
	}

	return f, nil
}

func (cu *DWARFCompileUnit) parseSubroutine(ent *dwarf.Entry, depth int) (*DWARFFunction, error) {
	attrName := ent.Val(dwarf.AttrName)
	attrAbstractOrigin := ent.Val(dwarf.AttrAbstractOrigin)
	var decFile string
	var decLine int
	var err error

	callFile, err := cu.getFilenameByIndex(int(ent.Val(dwarf.AttrCallFile).(int64)))
	if err != nil {
		return nil, err
	}
	callLine := int(ent.Val(dwarf.AttrCallLine).(int64))
	var callColumn int
	cc, ok := ent.Val(dwarf.AttrCallColumn).(int64)
	if !ok {
		callColumn = 0
	} else {
		callColumn = int(cc)
	}
	if attrAbstractOrigin != nil {
		ent1, err := cu.getEntryByOffset(attrAbstractOrigin.(dwarf.Offset))
		if err != nil {
			return nil, err
		}
		attrName = ent1.Val(dwarf.AttrName)
		decFile, err = cu.getFilenameByIndex(int(ent1.Val(dwarf.AttrDeclFile).(int64)))
		if err != nil {
			return nil, err
		}
	}
	if attrName == nil {
		attrName = ent.Val(dwarf.AttrName)
	}
	if attrName == nil {
		return nil, nil
	}
	if decFile == "" && ent.Val(dwarf.AttrDeclFile) != nil {
		decFile, err = cu.getFilenameByIndex(int(ent.Val(dwarf.AttrDeclFile).(int64)))
		if err != nil {
			return nil, err
		}
	}
	attrDecLine := ent.Val(dwarf.AttrDeclLine)
	if attrDecLine != nil {
		decLine = int(attrDecLine.(int64))
	}
	ranges, err := cu.Dwarf.Ranges(ent)
	if err != nil {
		return nil, err
	}
	f := &DWARFFunction{
		DwarfCompileUnit: cu,
		Type:             dwarf.TagInlinedSubroutine,
		Name:             attrName.(string),
		Ranges:           ranges,
		DeclFile:         decFile,
		DeclLine:         decLine,
		CallFile:         callFile,
		CallLine:         callLine,
		CallColumn:       callColumn,
		Inline:           true,
		Offset:           ent.Offset,
		Depth:            depth,
	}

	return f, nil
}

func (cu *DWARFCompileUnit) getEntryByOffset(offset dwarf.Offset) (*dwarf.Entry, error) {
	r := cu.Dwarf.Reader()
	r.Seek(offset)
	return r.Next()
}
