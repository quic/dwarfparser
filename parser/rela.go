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
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
)

func IsSectionExist(path, sec string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	s, err := GetSectionByName(f, sec)
	if err != nil {
		return false
	}
	return s != nil
}

func FindAllCoverPointsInRelaSec(path string) ([2][]uint64, error) {
	var pcs [2][]uint64
	info, err := GetTracePCInfo(path)
	if err != nil {
		return pcs, err
	}
	f, err := elf.Open(path)
	if err != nil {
		return pcs, err
	}
	defer f.Close()
	s, err := GetSectionByName(f, ".rela.text")
	if err != nil {
		return pcs, err
	}
	if s == nil {
		return pcs, fmt.Errorf("no .rela.text section")
	}
	callRelocType := arches[f.FileHeader.Machine].callRelocType
	relaOffset := arches[f.FileHeader.Machine].relaOffset
	di, err := DWARF(path)
	if err != nil {
		return pcs, err
	}
	rel := new(elf.Rela64)
	for r := s.Open(); ; {
		if err := binary.Read(r, di.Reader().ByteOrder(), rel); err != nil {
			if err == io.EOF {
				break
			}
			return pcs, err
		}
		if (rel.Info & 0xffffffff) != callRelocType {
			continue
		}
		pc := rel.Off - relaOffset
		index := int(elf.R_SYM64(rel.Info)) - 1
		if info != nil {
			if info.tracePCIdx[index] {
				pcs[0] = append(pcs[0], pc)
			} else if info.traceCmpIdx[index] {
				pcs[1] = append(pcs[1], pc)
			}
		} else {
			pcs[0] = append(pcs[0], pc)
		}
	}
	return pcs, err
}
