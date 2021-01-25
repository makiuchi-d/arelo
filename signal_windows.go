// +build windows

package main

import (
	"os"
)

func parseSignalOption(str string) (os.Signal, string) {
	return nil, "Signal option (--signal, -s) is not available on Windows."
}
