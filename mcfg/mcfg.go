// Package mcfg provides a simple foundation for complex service/binary
// configuration, initialization, and destruction
package mcfg

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mediocregopher/mediocre-go-lib/mctx"
	"github.com/mediocregopher/mediocre-go-lib/merr"
)

// TODO Sources:
// - JSON file
// - YAML file

func sortParams(params []Param) {
	sort.Slice(params, func(i, j int) bool {
		a, b := params[i], params[j]
		aPath, bPath := mctx.Path(a.Context), mctx.Path(b.Context)
		for {
			switch {
			case len(aPath) == 0 && len(bPath) == 0:
				return a.Name < b.Name
			case len(aPath) == 0 && len(bPath) > 0:
				return false
			case len(aPath) > 0 && len(bPath) == 0:
				return true
			case aPath[0] != bPath[0]:
				return aPath[0] < bPath[0]
			default:
				aPath, bPath = aPath[1:], bPath[1:]
			}
		}
	})
}

// returns all Params gathered by recursively retrieving them from this Context
// and its children. Returned Params are sorted according to their Path and
// Name.
func collectParams(ctx context.Context) []Param {
	var params []Param

	var visit func(context.Context)
	visit = func(ctx context.Context) {
		for _, param := range getLocalParams(ctx) {
			params = append(params, param)
		}

		for _, childCtx := range mctx.Children(ctx) {
			visit(childCtx)
		}
	}
	visit(ctx)

	sortParams(params)
	return params
}

func paramHash(path []string, name string) string {
	h := md5.New()
	for _, pathEl := range path {
		fmt.Fprintf(h, "pathEl:%q\n", pathEl)
	}
	fmt.Fprintf(h, "name:%q\n", name)
	hStr := hex.EncodeToString(h.Sum(nil))
	// we add the displayName to it to make debugging easier
	return paramFullName(path, name) + "/" + hStr
}

func populate(params []Param, src Source) error {
	if src == nil {
		src = ParamValues(nil)
	}

	// map Params to their hash, so we can match them to their ParamValues
	// later. There should not be any duplicates here.
	pM := map[string]Param{}
	for _, p := range params {
		path := mctx.Path(p.Context)
		hash := paramHash(path, p.Name)
		if _, ok := pM[hash]; ok {
			panic("duplicate Param: " + paramFullName(path, p.Name))
		}
		pM[hash] = p
	}

	pvs, err := src.Parse(params)
	if err != nil {
		return err
	}

	// dedupe the ParamValues based on their hashes, with the last ParamValue
	// taking precedence. Also filter out those with no corresponding Param.
	pvM := map[string]ParamValue{}
	for _, pv := range pvs {
		hash := paramHash(pv.Path, pv.Name)
		if _, ok := pM[hash]; !ok {
			continue
		}
		pvM[hash] = pv
	}

	// check for required params
	for hash, p := range pM {
		if !p.Required {
			continue
		} else if _, ok := pvM[hash]; !ok {
			ctx := mctx.Annotate(p.Context,
				"param", paramFullName(mctx.Path(p.Context), p.Name))
			return merr.New("required parameter is not set", ctx)
		}
	}

	// do the actual populating
	for hash, pv := range pvM {
		// at this point, all ParamValues in pvM have a corresponding pM Param
		p := pM[hash]
		if err := json.Unmarshal(pv.Value, p.Into); err != nil {
			return err
		}
	}

	return nil
}

// Populate uses the Source to populate the values of all Params which were
// added to the given Context, and all of its children. Populate may be called
// multiple times with the same Context, each time will only affect the values
// of the Params which were provided by the respective Source.
//
// Source may be nil to indicate that no configuration is provided. Only default
// values will be used, and if any parameters are required this will error.
func Populate(ctx context.Context, src Source) error {
	return populate(collectParams(ctx), src)
}
