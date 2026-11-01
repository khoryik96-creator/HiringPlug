// Command scaffold creates a new module. Use it through task:
//
//	task new:module -- name=job_ads entity=job_ad [prefix=jobad]
//
// then run `task gen` and `task test`.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/devtools/scaffold"
)

func main() {
	args := map[string]string{}
	for _, a := range os.Args[1:] {
		k, v, _ := strings.Cut(a, "=")
		args[strings.TrimLeft(k, "-")] = v
	}
	names, err := scaffold.NewNames(args["name"], args["entity"], args["prefix"])
	if err != nil {
		fmt.Fprintln(os.Stderr, "scaffold:", err)
		os.Exit(2)
	}
	written, err := scaffold.Generate("..", names)
	for _, f := range written {
		fmt.Println("  wrote", strings.TrimPrefix(f, "../"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "scaffold:", err)
		os.Exit(1)
	}
	fmt.Printf("\nModule %q created. Next:\n  task gen    # regenerate contract, Go, TypeScript and Python code\n  task test   # everything should pass\nThen edit contracts/modules/%s.yaml and the module's files to fit.\n", names.Module, names.Module)
}
