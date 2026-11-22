// Command cmd prints ANX source-build metadata without modifying Git.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/arturonavax/anx/internal/buildversion"
)

func main() {
	next := flag.Bool("next", false, "print the planned stable version; require complete local history")
	ldflags := flag.Bool("ldflags", false, "print linker flags for an ANX source build")
	flag.Parse()
	if flag.NArg() != 0 || *next && *ldflags {
		flag.Usage()
		os.Exit(2)
	}
	info, err := buildversion.Resolve(".")
	switch {
	case *next:
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(info.Next)
	case *ldflags:
		flags := []string{"-s", "-w"}
		safeValue := regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
		for _, item := range []struct{ symbol, env, value string }{
			{"Version", "ANX_BUILD_VERSION", info.Version},
			{"Commit", "ANX_BUILD_COMMIT", info.Commit},
			{"Date", "ANX_BUILD_DATE", info.Date},
		} {
			if override := os.Getenv(item.env); override != "" {
				item.value = override
			}
			if item.value == "" {
				continue
			}
			if !safeValue.MatchString(item.value) {
				fmt.Fprintln(os.Stderr, "invalid linker metadata:", item.env)
				os.Exit(2)
			}
			flags = append(flags, "-X", "main."+item.symbol+"="+item.value)
		}
		fmt.Println(strings.Join(flags, " "))
	default:
		fmt.Println(info.Version)
	}
}
