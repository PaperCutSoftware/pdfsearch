// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.
package doclib

import (
	"flag"
	"fmt"
	"os"
)

// MakeUsage updates flag.Usage to include usage message `msg`.
func MakeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
}
