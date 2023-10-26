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
	"io"
	"sort"

	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	lineFilesCMap   = cmap.New[[]*dwarf.LineFile]()
	lineEntriesCMap = cmap.New[map[uint64]*dwarf.LineEntry]()
)

func GetLineEntryByAddr(path string, pc uint64) (*dwarf.LineEntry, error) {
	cu, err := GetCompileUnitByAddr(path, pc)
	if err != nil {
		return nil, err
	}
	if cu == nil {
		return nil, fmt.Errorf("not found Compile Unit for 0x%x", pc)
	}
	// TODO: don't use r.SeekPC(pc, ent) which is wrong in golang.
	// SeekPC assumes address in .debug_line is sorted from low to high pc,
	// but it's not true.
	les, err := cu.getLineEntries()
	if err != nil {
		return nil, err
	}
	if ent, ok := les[pc]; ok {
		return ent, nil
	}
	// Find nearest LineEntry in case no such LineEntry in .debug_line
	var entries []*dwarf.LineEntry
	for _, ent := range les {
		entries = append(entries, ent)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Address < entries[j].Address
	})
	n := sort.Search(len(entries), func(i int) bool {
		return entries[i].Address > pc
	})
	if n-1 >= 0 && n-1 < len(entries) {
		return entries[n-1], nil
	}
	return nil, fmt.Errorf("not found 0x%x in .debug_line", pc)
}

func GenLineFiles(path string) error {
	cus, err := FindAllCompileUnits(path)
	if err != nil {
		return err
	}
	for _, cu := range cus {
		_, err := cu.getLineFiles()
		if err != nil {
			return err
		}
	}
	return nil
}

func GenLineEntries(path string) error {
	cus, err := FindAllCompileUnits(path)
	if err != nil {
		return err
	}
	for _, cu := range cus {
		_, err := cu.getLineEntries()
		if err != nil {
			return err
		}
	}
	return nil
}

func (cu *DWARFCompileUnit) getLineFiles() ([]*dwarf.LineFile, error) {
	k := fmt.Sprintf("%v-%v", cu.FilePath, cu.Entry.Offset)
	if e, ok := lineFilesCMap.Get(k); ok {
		return e, nil
	}
	r, err := cu.Dwarf.LineReader(cu.Entry)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("failed to get line reader for %v 0x%x", cu.FilePath, cu.Entry.Offset)
	}
	files := r.Files()
	lineFilesCMap.Set(k, files)
	return files, nil
}

func (cu *DWARFCompileUnit) getLineEntries() (map[uint64]*dwarf.LineEntry, error) {
	k := fmt.Sprintf("%v-%v", cu.FilePath, cu.Entry.Offset)
	if e, ok := lineEntriesCMap.Get(k); ok {
		return e, nil
	}
	lineEntries := make(map[uint64]*dwarf.LineEntry)
	r, err := cu.Dwarf.LineReader(cu.Entry)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("failed to get line reader for %v 0x%x", cu.FilePath, cu.Entry.Offset)
	}
	for {
		ent := &dwarf.LineEntry{}
		if r.Next(ent) == io.EOF {
			break
		}
		lineEntries[ent.Address] = ent
	}
	lineEntriesCMap.Set(k, lineEntries)
	return lineEntries, nil
}

func (cu *DWARFCompileUnit) getFilenameByIndex(index int) (string, error) {
	files, err := cu.getLineFiles()
	if err != nil {
		return "", err
	}
	if files == nil {
		return "", fmt.Errorf("files == nil")
	}
	if index >= len(files) {
		return "", fmt.Errorf("index (%v) >= len(files) (%v)", index, len(files))
	}
	if index == 0 {
		return "", nil
	}
	return files[index].Name, nil
}
