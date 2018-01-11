package mcfg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
)

// SourceCLI is a Source which will parse configuration from the CLI.
//
// Possible CLI options are generated by joining the Path to a Param, and its
// name, with dashes. For example:
//
//	cfg := mcfg.New().Child("foo").Child("bar")
//	addr := cfg.ParamString("addr", "", "Some address")
//	// the CLI option to fill addr will be "--foo-bar-addr"
//
// If the "-h" option is seen then a help page will be printed to
// stdout and the process will exit. Since all normally-defined parameters must
// being with double-dash ("--") they won't ever conflict with the help option.
//
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
func (cli SourceCLI) Parse(cfg *Cfg) ([]ParamValue, error) {
	args := cli.Args
	if cli.Args == nil {
		args = os.Args[1:]
	}

	pvM, err := cli.cliParamVals(cfg)
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
			cli.printHelp(os.Stdout, pvM)
			os.Stdout.Sync()
			os.Exit(1)
		} else {
			for key, pv = range pvM {
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
				return nil, fmt.Errorf("unexpected config parameter %q", arg)
			}
		}

		// pvOk is always true at this point, and so pv is filled in

		if pv.IsBool {
			// if it's a boolean we don't expect there to be a following value,
			// it's just a flag
			if pvStrValOk {
				return nil, fmt.Errorf("param %q is a boolean and cannot have a value", arg)
			}
			pv.Value = json.RawMessage("true")

		} else if !pvStrValOk {
			// everything else should have a value. if pvStrVal isn't filled it
			// means the next arg should be one. Continue the loop, it'll get
			// filled with the next one (hopefully)
			continue

		} else if pv.IsString && (pvStrVal == "" || pvStrVal[0] != '"') {
			pv.Value = json.RawMessage(`"` + pvStrVal + `"`)

		} else {
			pv.Value = json.RawMessage(pvStrVal)
		}

		pvs = append(pvs, pv)
		key = ""
		pv = ParamValue{}
		pvOk = false
		pvStrVal = ""
		pvStrValOk = false
	}
	if pvOk && !pvStrValOk {
		return nil, fmt.Errorf("param %q expected a value", key)
	}
	return pvs, nil
}

func (cli SourceCLI) cliParamVals(cfg *Cfg) (map[string]ParamValue, error) {
	m := map[string]ParamValue{}
	for _, param := range cfg.Params {
		key := cliKeyPrefix
		if len(cfg.Path) > 0 {
			key += strings.Join(cfg.Path, cliKeyJoin) + cliKeyJoin
		}
		key += param.Name
		m[key] = ParamValue{
			Param: param,
			Path:  cfg.Path,
		}
	}

	for _, child := range cfg.Children {
		childM, err := cli.cliParamVals(child)
		if err != nil {
			return nil, err
		}
		for key, pv := range childM {
			if _, ok := m[key]; ok {
				return nil, fmt.Errorf("multiple params use the same CLI arg %q", key)
			}
			m[key] = pv
		}
	}

	return m, nil
}

func (cli SourceCLI) printHelp(w io.Writer, pvM map[string]ParamValue) {
	type pvEntry struct {
		arg string
		ParamValue
	}

	pvA := make([]pvEntry, 0, len(pvM))
	for arg, pv := range pvM {
		pvA = append(pvA, pvEntry{arg: arg, ParamValue: pv})
	}

	sort.Slice(pvA, func(i, j int) bool {
		return pvA[i].arg < pvA[j].arg
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

	for _, pvE := range pvA {
		fmt.Fprintf(w, "\n%s", pvE.arg)
		if pvE.IsBool {
			fmt.Fprintf(w, " (Flag)")
		} else if defVal := fmtDefaultVal(pvE.Into); defVal != "" {
			fmt.Fprintf(w, " (Default: %s)", defVal)
		}
		fmt.Fprintf(w, "\n")
		if pvE.Usage != "" {
			fmt.Fprintln(w, "\t"+pvE.Usage)
		}
	}
	fmt.Fprintf(w, "\n")
}
