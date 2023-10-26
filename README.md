Dwarf Parser
============
Package dwarfparser provides higher level functions to parse dwarf.
The purpose is to avoid using third party tools (ex. llvm-addr2line) in go pkg.

# Installing
```
go get github.com/quic/dwarfparser
```

# Usage
Importing the package:
```
import (
     dwarfparser "github.com/quic/dwarfparser/parser"
)
```

Call export functions, for example:
```
frames := dwarfparser.Addr2line(elf_path, pc)
```

# Why do we need another addr2line?
There are several addr2line tools to decode function/file/line number from dwarf.
But none can give me correct result for some corner case.
And the toolchain like llvm keeps changing while DWARF format keeps much steady.

Corner case example:
* gnu addr2line v2.30 gives only `?`
```
$ addr2line -afie qca_cld3_kiwi_v2.ko 0x3482c4
0x00000000003482c4
lim_process_sme_req_messages
??:?
```

* llvm-addr2line seems correct, but function of L9013 is not global_dfs.
`global_dfs` is a variable, addr2line should show a function name like `lim_process_sme_req_messages`.
Provided https://github.com/llvm/llvm-project/pull/71008 to fix the issue.

```
android-ndk-r25c/toolchains/llvm/prebuilt/linux-x86_64/bin/llvm-addr2line -afie qca_cld3_kiwi_v2.ko 0x3482c4
warning: failed to compute relocation: R_AARCH64_NONE, Invalid data was encountered while parsing the file
0x3482c4
__lim_process_sme_set_ht2040_mode
out/android14-6.1/msm-kernel/../vendor/qcom/opensource/wlan/qcacld-3.0/core/mac/src/pe/lim/lim_process_sme_req_messages.c:7981
global_dfs
out/android14-6.1/msm-kernel/../vendor/qcom/opensource/wlan/qcacld-3.0/core/mac/src/pe/lim/lim_process_sme_req_messages.c:9013
```

* rust version of addr2line
```
addr2line -afie qca_cld3_kiwi_v2.ko 0x3482c4
$d.55
??:0
```

* golang version of addr2line
```
echo 0x3482c4 | $GOROOT/pkg/tool/linux_amd64/addr2line qca_cld3_kiwi_v2.ko
?
?:0
```

I wrote this golang addr2line which gives more correct info verified by using `llvm-dwarfdump --debug-line` and `llvm-dwarfdump --debug-info`.
It can be an alternative to llvm-addr2line but no depend on third party tools.
```
bin/addr2line -a -f -i -e qca_cld3_kiwi_v2.ko 0x3482c4
0x3482c4
__lim_process_sme_set_ht2040_mode
out/android14-6.1/vendor/qcom/opensource/wlan/qcacld-3.0/core/mac/src/pe/lim/lim_process_sme_req_messages.c:7981
lim_process_sme_req_messages
out/android14-6.1/vendor/qcom/opensource/wlan/qcacld-3.0/core/mac/src/pe/lim/lim_process_sme_req_messages.c:9013
```

# nm doesn't show symbol name correctly
For example, gnu nm shows
```
0000000000000000 t $d.1
0000000000000000 t $d.1
0000000000000000 r $d.1
0000000000000000 r $d.107
0000000000000000 r $d.109
0000000000000000 r $d.13
0000000000000000 r $d.131
0000000000000000 t $d.151
0000000000000000 d $d.152
0000000000000000 r $d.153
0000000000000000 d $d.154
0000000000000000 r $d.155
0000000000000000 r $d.156
0000000000000000 n $d.164
0000000000000000 b $d.27
0000000000000000 N $d.28
0000000000000000 N $d.29
0000000000000000 t $d.3
0000000000000000 d $d.3
0000000000000000 N $d.30
0000000000000000 N $d.31
0000000000000000 r $d.31
0000000000000000 n $d.33
0000000000000000 t $d.34
0000000000000000 N $d.35
0000000000000000 r $d.4
0000000000000000 r $d.53
0000000000000000 d $d.57
0000000000000000 d $d.7
0000000000000000 n $d.8
0000000000000000 r $d.91
0000000000000000 r $d.93
0000000000000000 r $d.95
0000000000000000 r _note_9
0000000000000000 d num_devices
0000000000000000 r __param_num_devices
0000000000000000 r __param_str_num_devices
0000000000000000 D __this_module
0000000000000000 d __UNIQUE_ID___addressable_cleanup_module460
0000000000000000 t __UNIQUE_ID___addressable_init_module459
0000000000000000 r __UNIQUE_ID_num_devicestype461
0000000000000000 r ____versions
0000000000000000 b zcomp_cpu_up_prepare.__key
```
while llvm-nm shows
```
llvm-nm zram.ko | grep 0000000000000000
0000000000000000 d __UNIQUE_ID___addressable_cleanup_module460
0000000000000000 d __UNIQUE_ID___addressable_init_module459
0000000000000000 r __UNIQUE_ID_num_devicestype461
0000000000000000 r ____versions
0000000000000000 r __param_num_devices
0000000000000000 r __param_str_num_devices
0000000000000000 D __this_module
0000000000000000 r _note_9
0000000000000000 d num_devices
0000000000000000 b zcomp_cpu_up_prepare.__key
```

# nm has less symbols than that in .debug_info
For example, llvm-nm doesn't have bit_spin_trylock while in .debug_info we have
```
 llvm-dwarfdump zram.ko | grep -A 3 -B 3 bit_spin_trylock
0x0001ed31:     NULL

0x0001ed32:   DW_TAG_subprogram
                DW_AT_name      ("bit_spin_trylock")
                DW_AT_decl_file ("out/android14-6.1/msm-kernel/include/linux/bit_spinlock.h")
                DW_AT_decl_line (41)
                DW_AT_prototyped        (true)
--
                    DW_AT_abstract_origin       (0x0001ed26 "index")

0x0001efc5:       DW_TAG_inlined_subroutine
                    DW_AT_abstract_origin       (0x0001ed32 "bit_spin_trylock")
                    DW_AT_ranges        (0x00001010
                       [0x00000000000017cc, 0x00000000000017d0)
                       [0x00000000000017d4, 0x00000000000017f4)
```

# License
Please see the [LICENSE file](LICENSE) for details.
