package util

import "fmt"

const Debug = true

func DPrinter(format string, a ...interface{}) {
	if Debug {
		fmt.Printf(format, a...)
	}
}
