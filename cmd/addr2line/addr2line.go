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

package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"

	dwarfparser "github.com/quic/dwarfparser/parser"
)

var (
	flagProfile     = flag.Bool("profile", false, "generate profile.")
	flagTrace       = flag.Bool("trace", false, "generate trace.")
	flagLegacy      = flag.Bool("legacy", false, "call extern addr2line instead.")
	flagAll         = flag.Bool("all", false, "addr2line for all pcs in dwarf debug_line.")
	flagAllTracePCs = flag.Bool("all-trace-pc", false, "addr2line for all __sanitizer_cov_trace_ pcs in dwarf debug_line.")
	flagAddress     = flag.Bool("a", false, "Like --addresses in gnu|llvm addr2line.")
	flagFunction    = flag.Bool("f", false, "Like --functions in gnu|llvm addr2line.")
	flagInline      = flag.Bool("i", false, "Like --inlines in gnu|llvm addr2line.")
	flagFileName    = flag.String("e", "a.out", "Like -e in gnu|llvm addr2line. The default file is a.out.")

	logger = log.New(os.Stdout, "", 0)
)

func main() {
	flag.Parse()
	if *flagProfile {
		cpuProfile, err := os.OpenFile("cpu.prof.gz", os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		defer cpuProfile.Close()
		err = pprof.StartCPUProfile(cpuProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}
	if *flagTrace {
		traceFile, err := os.Create("trace.out")
		if err != nil {
			panic(err)
		}
		if err := trace.Start(traceFile); err != nil {
			panic(err)
		}
		defer trace.Stop()
	}

	var pcs []uint64
	var err error
	if !*flagAll && !*flagAllTracePCs && len(flag.Args()) == 0 {
		symb := NewSymbolizer()
		defer symb.Close()
		scanner := bufio.NewScanner(os.Stdin)
		for {
			if !scanner.Scan() {
				err := scanner.Err()
				if err == nil {
					os.Exit(0)
				} else {
					fmt.Fprintln(os.Stderr, err)
				}
			}
			text := scanner.Text()
			if len(text) == 0 {
				continue
			}
			pc, err := strconv.ParseUint(text, 0, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to parse %v, err: %v", text, err)
				continue
			}
			if *flagLegacy {
				frames, err := symb.SymbolizeArray(*flagFileName, pcs)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to symbolize: %v\n", err)
					os.Exit(1)
				}
				for _, frame := range frames {
					if !*flagInline && frame.Inline {
						continue
					}
					if *flagFunction {
						logger.Printf("%v\n", frame.Func)
					}
					logger.Printf("%v:%v\n", frame.File, frame.Line)
				}
			} else {
				frames, err := dwarfparser.Addr2line(*flagFileName, pc)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
				}
				println(frames, *flagAddress, *flagFunction, *flagInline)
			}
		}
	}
	if *flagAll || *flagAllTracePCs {
		pcs, err = dwarfparser.FindAllPCs(*flagFileName, *flagAllTracePCs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	} else {
		for _, text := range flag.Args() {
			if !strings.HasPrefix(text, "0x") && !strings.HasPrefix(text, "0X") {
				text = fmt.Sprintf("0x%v", text)
			}
			pc, err := strconv.ParseUint(text, 0, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to parse %v, err: %v", text, err)
			}
			pcs = append(pcs, pc)
		}
	}
	procs := runtime.GOMAXPROCS(0)
	errC := make(chan error, procs)
	pcchan := make(chan []uint64, procs)
	for p := 0; p < procs; p++ {
		go func() {
			var symb *Symbolizer
			if *flagLegacy {
				symb = NewSymbolizer()
				defer symb.Close()
			}
			for pcs := range pcchan {
				if *flagLegacy {
					frames, err := symb.SymbolizeArray(*flagFileName, pcs)
					if err != nil {
						errC <- fmt.Errorf("failed to symbolize: %w", err)
						return
					}
					for _, frame := range frames {
						if !*flagInline && frame.Inline {
							continue
						}
						var output string
						if *flagFunction {
							output = fmt.Sprintf("%v\n", frame.Func)
						}
						output += fmt.Sprintf("%v:%v\n", frame.File, frame.Line)
						logger.Printf("%v", output)
					}
				} else {
					for _, pc := range pcs {
						frames, err := dwarfparser.Addr2line(*flagFileName, pc)
						if err != nil {
							errC <- fmt.Errorf("failed to symbolize 0x%x: %w", pc, err)
							return
						}
						println(frames, *flagAddress, *flagFunction, *flagInline)
					}
				}
			}
			errC <- nil
		}()
	}
	for i := 0; i < len(pcs); {
		end := i + 100
		if end > len(pcs) {
			end = len(pcs)
		}
		pcchan <- pcs[i:end]
		i = end
	}
	close(pcchan)
	for p := 0; p < procs; p++ {
		if err := <-errC; err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return
		}
	}
	if *flagProfile {
		memProfile, err := os.OpenFile("mem.prof.gz", os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		defer memProfile.Close()
		err = pprof.WriteHeapProfile(memProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}
}

func println(frames []dwarfparser.Frame, flagAddress, flagFunction, flagInline bool) {
	if len(frames) < 1 {
		return
	}
	var output string
	if flagAddress {
		output = fmt.Sprintf("0x%x\n", frames[0].PC)
	}
	for _, frame := range frames {
		if !flagInline && frame.Inline {
			continue
		}
		if flagFunction {
			output += fmt.Sprintf("%v\n", frame.Func)
		}
		output += fmt.Sprintf("%v:%v\n", frame.File, frame.Line)
	}
	logger.Printf("%v", output)
}
