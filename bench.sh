#!/bin/bash
# =============================================================================
#  @@-COPYRIGHT-START-@@
#
#  Copyright (c) 2024, Qualcomm Innovation Center, Inc. All rights reserved.
#
#  Redistribution and use in source and binary forms, with or without
#  modification, are permitted provided that the following conditions are met:
#
#  1. Redistributions of source code must retain the above copyright notice,
#     this list of conditions and the following disclaimer.
#
#  2. Redistributions in binary form must reproduce the above copyright notice,
#     this list of conditions and the following disclaimer in the documentation
#     and/or other materials provided with the distribution.
#
#  3. Neither the name of the copyright holder nor the names of its contributors
#     may be used to endorse or promote products derived from this software
#     without specific prior written permission.
#
#  THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
#  AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
#  IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
#  ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
#  LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
#  CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
#  SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
#  INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
#  CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
#  ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
#  POSSIBILITY OF SUCH DAMAGE.
#
#  SPDX-License-Identifier: BSD-3-Clause
#
#  @@-COPYRIGHT-END-@@
# =============================================================================

set -e

sudo apt install -y git flex bison make cpio bc gcc dpkg-dev rsync kmod libelf-dev libssl-dev linux-libc-dev 1>/dev/null

if [ ! -d linux ]; then
  git clone https://github.com/torvalds/linux
else
  pushd linux
  git reset --hard
  git pull
  popd
fi

if [ ! -f linux/vmlinux ]; then
  pushd linux
  export ARCH=x86_64
  export SRCARCH=x86
  export LLVM=1
  make defconfig
  echo "CONFIG_KCOV=y" >> .config
  echo "CONFIG_DEBUG_INFO=y" >> .config
  echo "CONFIG_DEBUG_INFO_DWARF4=y" >> .config
  make olddefconfig
  make -j$(nproc)
  popd
fi

echo 
echo "Parse all trace-pc in vmlinux by using external addr2line"
time bin/addr2line --profile legacy.profile.gz --legacy -a -f -i --all-trace-pc -e linux/vmlinux 1>/dev/null

echo "Parse all trace-pc in vmlinux by using golang impl addr2line"
time bin/addr2line --profile cpu.profile.gz -a -f -i --all-trace-pc -e linux/vmlinux 1>/dev/null

echo "Parse all trace-pc in xt_nat.ko by using external addr2line"
time bin/addr2line --legacy -a -f -i --all-trace-pc -e linux/net/netfilter/xt_nat.ko 1>/dev/null

echo "Parse all trace-pc in xt_nat.ko by using golang impl addr2line"
time bin/addr2line -a -f -i --all-trace-pc -e linux/net/netfilter/xt_nat.ko 1>/dev/null
