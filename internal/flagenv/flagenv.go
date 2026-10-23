// Package flagenv lets every command-line flag also be set through an
// environment variable named FENGHUO_<FLAG_NAME> (dashes become underscores).
// Explicit flags win over the environment.
package flagenv

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func EnvName(flagName string) string {
	return "FENGHUO_" + strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
}

func Parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	var err error
	fs.VisitAll(func(f *flag.Flag) {
		if err != nil || set[f.Name] {
			return
		}
		v, ok := os.LookupEnv(EnvName(f.Name))
		if !ok {
			return
		}
		if e := fs.Set(f.Name, v); e != nil {
			err = fmt.Errorf("invalid %s: %w", EnvName(f.Name), e)
		}
	})
	return err
}

// Explicit reports whether the flag was passed on the command line.
func Explicit(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
