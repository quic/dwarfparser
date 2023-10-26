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

// LIMITATION:
// limit 1:
// DWARF .debug_info have subprogram and inlined_subroutine,
// but it doesn't know which the call relation if one function is undefined.
// e.x.: zcomp_compress calls crypto_comp_compress,
// but crypto_comp_compress is undefined in zram.ko.
// So, in all funcs returned, func is ignored if len(f.Ranges) == 0
// 0x0000a3f8:   DW_TAG_subprogram
// DW_AT_low_pc    (0x0000000000000480)
// DW_AT_high_pc   (0x00000000000004d4)
// DW_AT_frame_base        (DW_OP_reg29 W29)
// DW_AT_GNU_all_call_sites        (true)
// DW_AT_name      ("zcomp_compress")
// DW_AT_decl_file ("out/android14-6.1/msm-kernel/drivers/block/zram/zcomp.c")
// DW_AT_decl_line (117)
// DW_AT_prototyped        (true)
// DW_AT_type      (0x00000a9a "int")
// DW_AT_external  (true)
// 0x0000a460:   DW_TAG_subprogram
// DW_AT_name      ("crypto_comp_compress")
// DW_AT_decl_file ("out/android14-6.1/msm-kernel/include/linux/crypto.h")
// DW_AT_decl_line (767)
// DW_AT_prototyped        (true)
// DW_AT_type      (0x00000a9a "int")
// DW_AT_declaration       (true)
// DW_AT_external  (true)
//
// limit:2
// DW_TAG_inlined_subroutine can be child of DW_TAG_lexical_block, while this
// DW_TAG_lexical_block can be child of DW_TAG_subprogram
// 0x338eab gen7_process_syncobj_query_work: 1
// 0x338fa0 gen7_syncobj_query_reply: 3
// 0x339039 to_dma_fence_array: 5
// 0x33904e dma_fence_is_array: 6
// 0x33906b fence_is_queried: 5

package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	dwarfparser "github.com/quic/dwarfparser/parser"
)

var (
	flagFileName    = flag.String("f", "a.out", "elf file with debug_info. The default file is a.out.")
	flagCallgraph   = flag.Bool("c", false, "generate callgraph.")
	flagFormat      = flag.String("format", "dot", "output format, default to dot.")
	flagVerbose     = flag.Bool("v", false, "verbose log.")
	flagStat        = flag.Bool("stat", false, "show coverage stats.")
	flagMaxLevel    = flag.Int("l", 0, "limit max function level to show, default 0 means no limit.")
	flagCoveredFile = flag.String("coveredfile", "", "file contains covered function line by line.")

	logger = log.New(os.Stdout, "", 0)
)

func main() {
	flag.Parse()
	funcs, err := dwarfparser.FindAllFuncs(*flagFileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	if *flagVerbose {
		for _, f := range funcs {
			logger.Printf("%v0x%x %v: %v\n", strings.Repeat(" ", f.Depth-1), f.Offset, f.Name, f.Depth)
		}
	}
	if *flagCallgraph {
		err := dot(funcs, *flagMaxLevel, *flagFormat, *flagCoveredFile, *flagStat, *flagVerbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}
}

func dot(funcs []*dwarfparser.DWARFFunction, maxLevel int, format, coveredFile string, showStats, verbose bool) error {
	graph := graphviz.New()
	defer graph.Close()
	digraph, err := graph.Graph()
	if err != nil {
		return err
	}
	digraph.SetLabel("Call Graph")
	digraph.SetRankDir("LR")
	coveredFuncs := make(map[string]bool)
	if coveredFile != "" {
		b, err := os.ReadFile(coveredFile)
		if err != nil {
			return err
		}
		matches := bytes.Split(b, []byte("\n"))
		for _, m := range matches {
			coveredFuncs[string(m)] = true
		}
	}
	type nodeF struct {
		Node     *cgraph.Node
		Function *dwarfparser.DWARFFunction
	}
	nodeWithEdges := make(map[string]bool)
	parentMap := make(map[int]*nodeF)
	uniqFunc := make(map[string]bool)
	type covStats struct {
		Covered int
		Total   int
		Depth   int
	}
	stats := make(map[int]*covStats)
	colors := []string{
		"sienna1", "brown", "green", "cyan", "darkgreen", "tan1", "purple", "red", "yellow", "aquamarine", "bisque", "cadetblue",
	}
	for _, f := range funcs {
		if _, ok := stats[f.Depth]; !ok {
			stats[f.Depth] = &covStats{
				Depth: f.Depth,
			}
		}
		if maxLevel != 0 && f.Depth > maxLevel {
			continue
		}
		if f.Depth == 1 {
			parentMap = make(map[int]*nodeF)
		}
		parent := parentMap[f.Depth-1]
		var n *cgraph.Node
		if _, ok := uniqFunc[f.Name]; !ok {
			stats[f.Depth].Total += 1
			uniqFunc[f.Name] = true
			n, err = digraph.CreateNode(f.Name)
		} else {
			n, err = digraph.Node(f.Name)
		}
		if err != nil {
			return err
		}
		n.SetShape("ellipse")
		if _, ok := coveredFuncs[f.Name]; ok {
			stats[f.Depth].Covered += 1
			n.SetColor("blue")
			n.SetShape("box")
			n.SetLabel(f.Name + " (covered)")
		} else if len(coveredFuncs) > 0 {
			n.SetColor("red")
		}
		if parent == nil {
			// find grandparent if no parent
			parent = parentMap[f.Depth-2]
		}
		if parent != nil {
			edge, err := digraph.CreateEdge(fmt.Sprintf("%v-%v", parent.Function.Name, f.Name), parent.Node, n)
			if err != nil {
				return err
			}
			nodeWithEdges[parent.Function.Name] = true
			nodeWithEdges[f.Name] = true
			edge.SetLabel(fmt.Sprintf("L%v", f.Depth-1))
			if f.Depth > 1 && f.Depth-2 < len(colors) {
				edge.SetColor(colors[f.Depth-2])
			}
		}
		parentMap[f.Depth] = &nodeF{
			Node:     n,
			Function: f,
		}
	}
	f := graphviz.XDOT
	switch format {
	case "svg":
		f = graphviz.SVG
	case "png":
		f = graphviz.PNG
	case "jpg":
		f = graphviz.JPG
	}
	if verbose {
		logger.Printf("NumberNodes:%v, NumberEdges%v\n", digraph.NumberNodes(), digraph.NumberEdges())
	}
	for _, f := range funcs {
		if _, ok := nodeWithEdges[f.Name]; !ok {
			n, err := digraph.Node(f.Name)
			if err != nil {
				return err
			}
			if n != nil {
				digraph.DeleteNode(n)
			}
		}
	}
	if verbose {
		logger.Printf("After deletion, NumberNodes:%v\n", digraph.NumberNodes())
	}
	if err := graph.RenderFilename(digraph, f, "callgraph."+string(f)); err != nil {
		log.Fatal(err)
	}
	if showStats && len(coveredFuncs) > 0 {
		var cv []*covStats
		for _, s := range stats {
			cv = append(cv, s)
		}
		sort.Slice(cv, func(i, j int) bool {
			return cv[i].Depth < cv[j].Depth
		})
		logger.Println("Depth,Covered,Total,Percent")
		for _, c := range cv {
			if maxLevel != 0 && c.Depth > maxLevel {
				continue
			}
			logger.Printf("%v,%v,%v,%v%%\n", c.Depth, c.Covered, c.Total, percent(c.Covered, c.Total))
		}
	}
	return nil
}

func percent(covered, total int) int {
	f := math.Ceil(float64(covered) / float64(total) * 100)
	if f == 100 && covered < total {
		f = 99
	}
	return int(f)
}
