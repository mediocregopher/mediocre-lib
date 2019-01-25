package mcfg

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/mediocregopher/mediocre-go-lib/merr"
)

// SourceCLI is a Source which will parse configuration from the CLI.
//
// Possible CLI options are generated by joining a Param's Path and Name with
// dashes. For example:
//
//	ctx := mctx.New()
//	ctx = mctx.ChildOf(ctx, "foo")
//	ctx = mctx.ChildOf(ctx, "bar")
//	addr := mcfg.String(ctx, "addr", "", "Some address")
//	// the CLI option to fill addr will be "--foo-bar-addr"
//
// If the "-h" option is seen then a help page will be printed to
// stdout and the process will exit. Since all normally-defined parameters must
// being with double-dash ("--") they won't ever conflict with the help option.
//
// SourceCLI behaves a little differently with boolean parameters. Setting the
// value of a boolean parameter directly _must_ be done with an equals, for
// example: `--boolean-flag=1` or `--boolean-flag=false`. Using the
// space-separated format will not work. If a boolean has no equal-separated
// value it is assumed to be setting the value to `true`, as would be expected.
type SourceCLI struct {
	Args []string // if nil then os.Args[1:] is used

	DisableHelpPage bool
}

const (
	cliKeyJoin   = "-"
	cliKeyPrefix = "--"
	cliValSep    = "="
	cliHelpArg   = "-h"
)

// Parse implements the method for the Source interface
func (cli SourceCLI) Parse(params []Param) ([]ParamValue, error) {
	args := cli.Args
	if cli.Args == nil {
		args = os.Args[1:]
	}

	pM, err := cli.cliParams(params)
	if err != nil {
		return nil, err
	}
	pvs := make([]ParamValue, 0, len(args))
	var (
		key        string
		pv         ParamValue
		pvOk       bool
		pvStrVal   string
		pvStrValOk bool
	)
	for _, arg := range args {
		if pvOk {
			pvStrVal = arg
			pvStrValOk = true
		} else if !cli.DisableHelpPage && arg == cliHelpArg {
			cli.printHelp(os.Stdout, pM)
			os.Stdout.Sync()
			os.Exit(1)
		} else {
			for key, pv.Param = range pM {
				if arg == key {
					pvOk = true
					break
				}

				prefix := key + cliValSep
				if !strings.HasPrefix(arg, prefix) {
					continue
				}
				pvOk = true
				pvStrVal = strings.TrimPrefix(arg, prefix)
				pvStrValOk = true
				break
			}
			if !pvOk {
				err := merr.New("unexpected config parameter")
				return nil, merr.WithValue(err, "param", arg, true)
			}
		}

		// pvOk is always true at this point, and so pv is filled in

		// As a special case for CLI, if a boolean has no value set it means it
		// is true.
		if pv.IsBool && !pvStrValOk {
			pvStrVal = "true"
			pvStrValOk = true
		} else if !pvStrValOk {
			// everything else should have a value. if pvStrVal isn't filled it
			// means the next arg should be one. Continue the loop, it'll get
			// filled with the next one (hopefully)
			continue
		}

		pv.Value = pv.Param.fuzzyParse(pvStrVal)

		pvs = append(pvs, pv)
		key = ""
		pv = ParamValue{}
		pvOk = false
		pvStrVal = ""
		pvStrValOk = false
	}
	if pvOk && !pvStrValOk {
		err := merr.New("param expected a value")
		return nil, merr.WithValue(err, "param", key, true)
	}
	return pvs, nil
}

func (cli SourceCLI) cliParams(params []Param) (map[string]Param, error) {
	m := map[string]Param{}
	for _, p := range params {
		key := strings.Join(append(p.Path, p.Name), cliKeyJoin)
		m[cliKeyPrefix+key] = p
	}
	return m, nil
}

func (cli SourceCLI) printHelp(w io.Writer, pM map[string]Param) {
	type pEntry struct {
		arg string
		Param
	}

	pA := make([]pEntry, 0, len(pM))
	for arg, p := range pM {
		pA = append(pA, pEntry{arg: arg, Param: p})
	}

	sort.Slice(pA, func(i, j int) bool {
		if pA[i].Required != pA[j].Required {
			return pA[i].Required
		}
		return pA[i].arg < pA[j].arg
	})

	fmtDefaultVal := func(ptr interface{}) string {
		if ptr == nil {
			return ""
		}
		val := reflect.Indirect(reflect.ValueOf(ptr))
		zero := reflect.Zero(val.Type())
		if reflect.DeepEqual(val.Interface(), zero.Interface()) {
			return ""
		} else if val.Type().Kind() == reflect.String {
			return fmt.Sprintf("%q", val.Interface())
		}
		return fmt.Sprint(val.Interface())
	}

	for _, p := range pA {
		fmt.Fprintf(w, "\n%s", p.arg)
		if p.IsBool {
			fmt.Fprintf(w, " (Flag)")
		} else if p.Required {
			fmt.Fprintf(w, " (Required)")
		} else if defVal := fmtDefaultVal(p.Into); defVal != "" {
			fmt.Fprintf(w, " (Default: %s)", defVal)
		}
		fmt.Fprintf(w, "\n")
		if p.Usage != "" {
			fmt.Fprintln(w, "\t"+p.Usage)
		}
	}
	fmt.Fprintf(w, "\n")
}
