//go:build darwin || linux

package engine

import "syscall"

const soReusePort = syscall.SO_REUSEPORT
