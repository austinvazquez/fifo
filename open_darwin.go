//go:build darwin

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package fifo

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// openFifoFile opens fn for the fifo data path, given the flag openFifo
// has already stripped O_CREAT/O_NONBLOCK from.
//
// os.OpenFile deliberately excludes FIFOs from Darwin's kqueue-based
// runtime poller (see golang.org/issue/24164): kqueue can miss the event
// fired when a fifo's last writer closes, so relying on it to detect EOF
// could hang a Read forever. The tradeoff is that Close can't interrupt an
// in-progress Read/Write either: a descriptor that was never registered
// with the poller is just a bare close(2), which a concurrently blocked
// read(2)/write(2) never observes.
//
// An O_RDWR fifo never depends on that EOF path: holding our own fd open
// for both directions guarantees there's always at least one writer, so
// it can never see EOF from someone else's writer closing. For that mode
// only, it's safe to open the descriptor ourselves with O_NONBLOCK
// preserved and hand it to os.NewFile instead of os.OpenFile:
// os.OpenFile's poller exclusion only applies to descriptors it opens
// itself, so this sidesteps it, gets the fd registered with the poller,
// and lets Close interrupt a blocked Read/Write the same way it already
// does on Linux. O_RDONLY/O_WRONLY fifos keep going through os.OpenFile
// unchanged, preserving their existing (correct) EOF-via-blocking-read
// behavior.
func openFifoFile(fn string, flag int) (*os.File, error) {
	if flag&syscall.O_RDWR == 0 {
		return os.OpenFile(fn, flag, 0)
	}

	// O_NONBLOCK is what keeps this fd pollable once wrapped with
	// os.NewFile below, despite openFifo having stripped it from flag
	// for the rest of the fifo's lifetime; see the doc comment above.
	for {
		fd, err := unix.Open(fn, flag|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: fn, Err: err}
		}
		return os.NewFile(uintptr(fd), fn), nil
	}
}
