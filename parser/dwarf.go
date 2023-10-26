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
	"debug/elf"
	"fmt"
	"sort"

	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	dwarfCMap = cmap.New[*dwarf.Data]()
)

func DWARF(path string) (*dwarf.Data, error) {
	if e, ok := dwarfCMap.Get(path); ok {
		return e, nil
	}
	file, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	di, err := file.DWARF()
	if err != nil {
		return nil, err
	}
	if di == nil {
		return nil, fmt.Errorf("not found .debug_info section")
	}
	dwarfCMap.Set(path, di)
	return di, nil
}

func FindAllPCs(path string, filterTracePC bool) ([]uint64, error) {
	var pcs []uint64
	if filterTracePC {
		var coverPoints [2][]uint64
		var err error
		if IsSectionExist(path, ".rela.text") {
			coverPoints, err = FindAllCoverPointsInRelaSec(path)
		} else {
			coverPoints, err = FindAllCoverPoints(path)
		}
		if err != nil {
			return nil, err
		}
		for _, pcs1 := range coverPoints {
			pcs = append(pcs, pcs1...)
		}
	} else {
		err := GenLineEntries(path)
		if err != nil {
			return nil, err
		}
		cus, err := FindAllCompileUnits(path)
		if err != nil {
			return nil, err
		}
		for _, cu := range cus {
			lineEntries, err := cu.getLineEntries()
			if err != nil {
				return nil, err
			}
			for _, ent := range lineEntries {
				pcs = append(pcs, ent.Address)
			}
		}
	}
	sort.Slice(pcs, func(i, j int) bool {
		return pcs[i] < pcs[j]
	})
	return pcs, nil
}

func Addr2line(path string, pc uint64) ([]Frame, error) {
	frames, err := FindAllFramesByAddr(path, pc)
	if err != nil {
		return nil, err
	}
	return frames, nil
}

func FindAllFramesByAddr(path string, pc uint64) ([]Frame, error) {
	var frames []Frame
	cu, err := GetCompileUnitByAddr(path, pc)
	if err != nil {
		return nil, err
	}
	sp, err := cu.GetSubprogramByAddr(pc)
	if err != nil {
		return nil, err
	}
	frames = append(frames, Frame{
		PC:     uint64(sp.Offset),
		Func:   sp.Name,
		File:   sp.DeclFile,
		Line:   sp.DeclLine,
		Inline: sp.Inline,
	})
	k := fmt.Sprintf("%v-%v-%v", path, cu.Entry.Offset, sp.Offset)
	rts, ok := allSubroutinesCMap.Get(k)
	if !ok {
		rts, err = sp.GetSubroutinesBySubprogram()
		if err != nil {
			return nil, err
		}
		allSubroutinesCMap.Set(k, rts)
	}
	for i := len(rts); i > 0; i-- {
		f := rts[i-1]
		for _, r := range f.Ranges {
			if pc >= r[0] && pc < r[1] {
				frames = append(frames, Frame{
					PC:     uint64(f.Offset),
					Func:   f.Name,
					File:   f.CallFile,
					Line:   f.CallLine,
					Inline: f.Inline,
				})
				break
			}
		}
	}
	sort.Slice(frames, func(i int, j int) bool {
		return frames[i].PC > frames[j].PC
	})
	le, err := GetLineEntryByAddr(path, pc)
	if err != nil {
		return nil, err
	}
	frames = append([]Frame{
		{
			PC:   pc,
			File: le.File.Name,
			Line: le.Line,
		},
	}, frames...)
	var next Frame
	for i := 0; i < len(frames)-1; i++ {
		next = frames[i+1]
		frames[i].Func = next.Func
	}
	if len(frames) == 1 {
		frames[0].Func = "??"
		return frames, nil
	}
	return frames[:len(frames)-1], nil
}
