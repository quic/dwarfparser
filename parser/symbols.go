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
	"strings"
)

type TracePCInfo struct {
	textAddr    uint64
	tracePC     uint64
	traceCmp    map[uint64]bool
	tracePCIdx  map[int]bool
	traceCmpIdx map[int]bool
}

func FindAllSymbols(path string) ([]elf.Symbol, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	symbols, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	return symbols, nil
}

func FindAllSymbolsInSec(path, sec string) ([]elf.Symbol, error) {
	var finalSymbols []elf.Symbol
	symbols, err := FindAllSymbols(path)
	if err != nil {
		return nil, err
	}
	idx, err := GetSectionIdx(path, sec)
	if err != nil {
		return nil, err
	}
	for _, s := range symbols {
		if s.Section == elf.SectionIndex(idx) {
			finalSymbols = append(finalSymbols, s)
		}
	}
	return finalSymbols, nil
}

func GetTracePCInfo(path string) (*TracePCInfo, error) {
	symbols, err := FindAllSymbols(path)
	if err != nil {
		return nil, err
	}
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	text, err := GetSectionByName(f, ".text")
	if err != nil {
		return nil, err
	}
	info := &TracePCInfo{
		textAddr:    text.Addr,
		tracePCIdx:  make(map[int]bool),
		traceCmpIdx: make(map[int]bool),
	}
	for i, s := range symbols {
		t := s.Value >= text.Addr && s.Value+s.Size <= text.Addr+text.Size
		if strings.HasPrefix(s.Name, "__sanitizer_cov_trace_") {
			if s.Name == "__sanitizer_cov_trace_pc" {
				info.tracePCIdx[i] = true
				if t {
					info.tracePC = s.Value
				}
			} else {
				info.traceCmpIdx[i] = true
				if t {
					info.traceCmp[s.Value] = true
				}
			}
		}
	}
	return info, nil
}
