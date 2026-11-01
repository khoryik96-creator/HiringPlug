// Command contracts merges the shared API contract and the per-module contract
// files into contracts/openapi.yaml, the file every code generator reads.
//
//	go run ./cmd/contracts          # (re)write contracts/openapi.yaml
//	go run ./cmd/contracts -check   # exit 1 if contracts/openapi.yaml is out of date
//
// Run it from the backend directory (`task gen` does this for you).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/khoryik96-creator/hiringplug/backend/internal/devtools/contractbundle"
)

func main() {
	base := flag.String("base", "../contracts/openapi.base.yaml", "shared contract file")
	modules := flag.String("modules", "../contracts/modules", "directory of per-module contract files")
	out := flag.String("out", "../contracts/openapi.yaml", "generated output file")
	check := flag.Bool("check", false, "only check that the output file is up to date")
	flag.Parse()

	bundled, err := contractbundle.BundleFiles(*base, *modules)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *check {
		current, err := os.ReadFile(*out)
		if err != nil || !bytes.Equal(current, bundled) {
			fmt.Fprintln(os.Stderr, contractbundle.ErrOutOfDate)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(*out, bundled, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}
