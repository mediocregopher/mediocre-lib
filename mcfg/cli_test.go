package mcfg

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	. "testing"
	"time"

	"github.com/mediocregopher/mediocre-go-lib/mrand"
	"github.com/mediocregopher/mediocre-go-lib/mtest/massert"
	"github.com/mediocregopher/mediocre-go-lib/mtest/mchk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceCLIHelp(t *T) {
	assertHelp := func(ctx context.Context, subCmdPrefix []string, exp string) {
		buf := new(bytes.Buffer)
		src := &SourceCLI{}
		pM, err := src.cliParams(CollectParams(ctx))
		require.NoError(t, err)
		subCmdM, _ := ctx.Value(cliKeySubCmdM).(map[string]subCmd)
		src.printHelp(buf, subCmdPrefix, subCmdM, pM)

		out := buf.String()
		ok := regexp.MustCompile(exp).MatchString(out)
		assert.True(t, ok, "exp:%s (%q)\ngot:%s (%q)", exp, exp, out, out)
	}

	ctx := context.Background()
	assertHelp(ctx, nil, `^Usage: \S+

$`)
	assertHelp(ctx, []string{"foo", "bar"}, `^Usage: \S+ foo bar

$`)

	ctx, _ = WithInt(ctx, "foo", 5, "Test int param  ") // trailing space should be trimmed
	ctx, _ = WithBool(ctx, "bar", "Test bool param.")
	ctx, _ = WithString(ctx, "baz", "baz", "Test string param")
	ctx, _ = WithRequiredString(ctx, "baz2", "")
	ctx, _ = WithRequiredString(ctx, "baz3", "")

	assertHelp(ctx, nil, `^Usage: \S+ \[options\]

Options:

	--baz2 \(Required\)

	--baz3 \(Required\)

	--bar \(Flag\)
		Test bool param.

	--baz \(Default: "baz"\)
		Test string param.

	--foo \(Default: 5\)
		Test int param.

$`)

	assertHelp(ctx, []string{"foo", "bar"}, `^Usage: \S+ foo bar \[options\]

Options:

	--baz2 \(Required\)

	--baz3 \(Required\)

	--bar \(Flag\)
		Test bool param.

	--baz \(Default: "baz"\)
		Test string param.

	--foo \(Default: 5\)
		Test int param.

$`)

	ctx, _ = WithCLISubCommand(ctx, "first", "First sub-command", nil)
	ctx, _ = WithCLISubCommand(ctx, "second", "Second sub-command", nil)
	assertHelp(ctx, []string{"foo", "bar"}, `^Usage: \S+ foo bar <sub-command> \[options\]

Sub-commands:

	first	First sub-command
	second	Second sub-command

Options:

	--baz2 \(Required\)

	--baz3 \(Required\)

	--bar \(Flag\)
		Test bool param.

	--baz \(Default: "baz"\)
		Test string param.

	--foo \(Default: 5\)
		Test int param.

$`)

	ctx, _ = WithCLISubCommand(ctx, "", "No sub-command", nil)
	assertHelp(ctx, []string{"foo", "bar"}, `^Usage: \S+ foo bar \[sub-command\] \[options\]

Sub-commands:

	<None>	No sub-command
	first	First sub-command
	second	Second sub-command

Options:

	--baz2 \(Required\)

	--baz3 \(Required\)

	--bar \(Flag\)
		Test bool param.

	--baz \(Default: "baz"\)
		Test string param.

	--foo \(Default: 5\)
		Test int param.

$`)
}

func TestSourceCLI(t *T) {
	type state struct {
		srcCommonState
		*SourceCLI
	}

	type params struct {
		srcCommonParams
		nonBoolWEq bool // use equal sign when setting value
	}

	chk := mchk.Checker{
		Init: func() mchk.State {
			var s state
			s.srcCommonState = newSrcCommonState()
			s.SourceCLI = &SourceCLI{
				Args: make([]string, 0, 16),
			}
			return s
		},
		Next: func(ss mchk.State) mchk.Action {
			s := ss.(state)
			var p params
			p.srcCommonParams = s.srcCommonState.next()
			// if the param is a bool or unset this won't get used, but w/e
			p.nonBoolWEq = mrand.Intn(2) == 0
			return mchk.Action{Params: p}
		},
		Apply: func(ss mchk.State, a mchk.Action) (mchk.State, error) {
			s := ss.(state)
			p := a.Params.(params)

			s.srcCommonState = s.srcCommonState.applyCtxAndPV(p.srcCommonParams)
			if !p.unset {
				arg := cliKeyPrefix
				if len(p.path) > 0 {
					arg += strings.Join(p.path, cliKeyJoin) + cliKeyJoin
				}
				arg += p.name
				if !p.isBool {
					if p.nonBoolWEq {
						arg += "="
					} else {
						s.SourceCLI.Args = append(s.SourceCLI.Args, arg)
						arg = ""
					}
					arg += p.nonBoolVal
				}
				s.SourceCLI.Args = append(s.SourceCLI.Args, arg)
			}

			err := s.srcCommonState.assert(s.SourceCLI)
			return s, err
		},
	}

	if err := chk.RunFor(2 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWithCLITail(t *T) {
	ctx := context.Background()
	ctx, _ = WithInt(ctx, "foo", 5, "")
	ctx, _ = WithBool(ctx, "bar", "")

	type testCase struct {
		args    []string
		expTail []string
	}

	cases := []testCase{
		{
			args:    []string{"--foo", "5"},
			expTail: nil,
		},
		{
			args:    []string{"--foo", "5", "a", "b", "c"},
			expTail: []string{"a", "b", "c"},
		},
		{
			args:    []string{"--foo=5", "a", "b", "c"},
			expTail: []string{"a", "b", "c"},
		},
		{
			args:    []string{"--foo", "5", "--bar"},
			expTail: nil,
		},
		{
			args:    []string{"--foo", "5", "--bar", "a", "b", "c"},
			expTail: []string{"a", "b", "c"},
		},
	}

	for _, tc := range cases {
		ctx, tail := WithCLITail(ctx)
		_, err := Populate(ctx, &SourceCLI{Args: tc.args})
		massert.Require(t, massert.Comment(massert.All(
			massert.Nil(err),
			massert.Equal(tc.expTail, *tail),
		), "tc: %#v", tc))
	}
}

func ExampleWithCLITail() {
	ctx := context.Background()
	ctx, foo := WithInt(ctx, "foo", 1, "Description of foo.")
	ctx, tail := WithCLITail(ctx)
	ctx, bar := WithString(ctx, "bar", "defaultVal", "Description of bar.")

	_, err := Populate(ctx, &SourceCLI{
		Args: []string{"--foo=100", "BADARG", "--bar", "BAR"},
	})

	fmt.Printf("err:%v foo:%v bar:%v tail:%#v\n", err, *foo, *bar, *tail)
	// Output: err:<nil> foo:100 bar:defaultVal tail:[]string{"BADARG", "--bar", "BAR"}
}

func TestWithCLISubCommand(t *T) {
	var (
		ctx         context.Context
		foo         *int
		bar         *int
		baz         *int
		aFlag       *bool
		defaultFlag *bool
	)
	reset := func() {
		foo, bar, baz, aFlag, defaultFlag = nil, nil, nil, nil, nil
		ctx = context.Background()
		ctx, foo = WithInt(ctx, "foo", 0, "Description of foo.")
		ctx, aFlag = WithCLISubCommand(ctx, "a", "Description of a.",
			func(ctx context.Context) context.Context {
				ctx, bar = WithInt(ctx, "bar", 0, "Description of bar.")
				return ctx
			})
		ctx, defaultFlag = WithCLISubCommand(ctx, "", "Description of default.",
			func(ctx context.Context) context.Context {
				ctx, baz = WithInt(ctx, "baz", 0, "Description of baz.")
				return ctx
			})
	}

	reset()
	_, err := Populate(ctx, &SourceCLI{
		Args: []string{"a", "--foo=1", "--bar=2"},
	})
	massert.Require(t,
		massert.Comment(massert.Nil(err), "%v", err),
		massert.Equal(1, *foo),
		massert.Equal(2, *bar),
		massert.Nil(baz),
		massert.Equal(true, *aFlag),
		massert.Equal(false, *defaultFlag),
	)

	reset()
	_, err = Populate(ctx, &SourceCLI{
		Args: []string{"--foo=1", "--baz=3"},
	})
	massert.Require(t,
		massert.Comment(massert.Nil(err), "%v", err),
		massert.Equal(1, *foo),
		massert.Nil(bar),
		massert.Equal(3, *baz),
		massert.Equal(false, *aFlag),
		massert.Equal(true, *defaultFlag),
	)
}

func ExampleWithCLISubCommand() {
	ctx := context.Background()
	ctx, foo := WithInt(ctx, "foo", 0, "Description of foo.")

	var bar *int
	ctx, aFlag := WithCLISubCommand(ctx, "a", "Description of a.",
		func(ctx context.Context) context.Context {
			ctx, bar = WithInt(ctx, "bar", 0, "Description of bar.")
			return ctx
		})

	var baz *int
	ctx, defaultFlag := WithCLISubCommand(ctx, "", "Description of default.",
		func(ctx context.Context) context.Context {
			ctx, baz = WithInt(ctx, "baz", 0, "Description of baz.")
			return ctx
		})

	args := []string{"a", "--foo=1", "--bar=2"}
	if _, err := Populate(ctx, &SourceCLI{Args: args}); err != nil {
		panic(err)
	}
	fmt.Printf("foo:%d bar:%d aFlag:%v defaultFlag:%v\n", *foo, *bar, *aFlag, *defaultFlag)

	// reset output for another Populate
	*aFlag = false
	args = []string{"--foo=1", "--baz=3"}
	if _, err := Populate(ctx, &SourceCLI{Args: args}); err != nil {
		panic(err)
	}
	fmt.Printf("foo:%d baz:%d aFlag:%v defaultFlag:%v\n", *foo, *baz, *aFlag, *defaultFlag)

	// Output: foo:1 bar:2 aFlag:true defaultFlag:false
	// foo:1 baz:3 aFlag:false defaultFlag:true
}
