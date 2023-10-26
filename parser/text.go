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
	"fmt"
)

// ReadCoverPoints finds all coverage points (calls of __sanitizer_cov_trace_*) in the object file.
// Currently it is [amd64|arm64]-specific: looks for opcode and correct offset.
// Running objdump on the whole object file is too slow.
func FindAllCoverPoints(path string) ([2][]uint64, error) {
	var pcs [2][]uint64
	info, err := GetTracePCInfo(path)
	if err != nil {
		return pcs, err
	}
	if info.tracePC == 0 {
		return pcs, fmt.Errorf("no __sanitizer_cov_trace_pc symbol in the object file")
	}
	f, err := elf.Open(path)
	if err != nil {
		return pcs, err
	}
	defer f.Close()
	s, err := GetSectionByName(f, ".text")
	if err != nil {
		return pcs, err
	}
	if s == nil {
		return pcs, fmt.Errorf("no .text section in the object file")
	}
	data, err := s.Data()
	if err != nil {
		return pcs, err
	}
	// Loop that's checking each instruction for the current architectures call
	// opcode. When found, it compares the call target address with those of the
	// __sanitizer_cov_trace_* functions we previously collected. When found,
	// we collect the pc as a coverage point.
	arch := arches[f.FileHeader.Machine]
	for i, opcode := range data {
		if opcode != arch.opcodes[0] && opcode != arch.opcodes[1] {
			continue
		}
		i -= arch.opcodeOffset
		if i < 0 || i+arch.callLen > len(data) {
			continue
		}
		pc := info.textAddr + uint64(i)
		target := arch.target(&arch, data[i:], pc, opcode)
		if target == info.tracePC {
			pcs[0] = append(pcs[0], pc)
		} else if info.traceCmp[target] {
			pcs[1] = append(pcs[1], pc)
		}
	}
	return pcs, nil
}
