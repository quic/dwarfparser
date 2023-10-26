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
)

type Arch struct {
	callLen       int
	relaOffset    uint64
	opcodeOffset  int
	opcodes       [2]byte
	callRelocType uint64
	target        func(arch *Arch, insn []byte, pc uint64, opcode byte) uint64
}

var arches = map[elf.Machine]Arch{
	elf.EM_X86_64: {
		callLen:       5,
		relaOffset:    1,
		opcodes:       [2]byte{0xe8, 0xe8},
		callRelocType: uint64(elf.R_X86_64_PLT32),
		target: func(arch *Arch, insn []byte, pc uint64, opcode byte) uint64 {
			off := uint64(int64(int32(binary.LittleEndian.Uint32(insn[1:]))))
			return pc + off + uint64(arch.callLen)
		},
	},
	elf.EM_AARCH64: {
		callLen:       4,
		opcodeOffset:  3,
		opcodes:       [2]byte{0x94, 0x97},
		callRelocType: uint64(elf.R_AARCH64_CALL26),
		target: func(arch *Arch, insn []byte, pc uint64, opcode byte) uint64 {
			off := uint64(binary.LittleEndian.Uint32(insn)) & ((1 << 24) - 1)
			if opcode == arch.opcodes[1] {
				off |= 0xffffffffff000000
			}
			return pc + 4*off
		},
	},
}
