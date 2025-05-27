package mybase

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestOptionStatus(t *testing.T) {
	assertOptionStatus := func(cfg *Config, name string, expectChanged, expectSupplied, expectOnCLI bool) {
		t.Helper()
		if cfg.Changed(name) != expectChanged {
			t.Errorf("Expected cfg.Changed(%s)==%t, but instead returned %t", name, expectChanged, !expectChanged)
		}
		if cfg.Supplied(name) != expectSupplied {
			t.Errorf("Expected cfg.Supplied(%s)==%t, but instead returned %t", name, expectSupplied, !expectSupplied)
		}
		if cfg.OnCLI(name) != expectOnCLI {
			t.Errorf("Expected cfg.OnCLI(%s)==%t, but instead returned %t", name, expectOnCLI, !expectOnCLI)
		}
	}

	fakeFileOptions := SimpleSource(map[string]string{
		"hidden": "set off cli",
		"bool1":  "1",
	})
	cmd := simpleCommand()
	cfg := ParseFakeCLI(t, cmd, "mycommand -s 'hello world' --skip-truthybool --hidden=\"somedefault\" -B arg1", fakeFileOptions)
	assertOptionStatus(cfg, "visible", false, false, false)
	assertOptionStatus(cfg, "hidden", false, true, true)
	assertOptionStatus(cfg, "hasshort", true, true, true)
	assertOptionStatus(cfg, "bool1", true, true, false)
	assertOptionStatus(cfg, "bool2", true, true, true)
	assertOptionStatus(cfg, "truthybool", true, true, true)
	assertOptionStatus(cfg, "required", true, true, true)
	assertOptionStatus(cfg, "optional", false, false, false)

	// Among other things, confirm behavior of string option set to empty string
	cfg = ParseFakeCLI(t, cmd, "mycommand --skip-bool1 --hidden=\"\" --bool2 arg1", fakeFileOptions)
	assertOptionStatus(cfg, "bool1", false, true, true)
	assertOptionStatus(cfg, "hidden", true, true, true)
	assertOptionStatus(cfg, "bool2", true, true, true)
	if cfg.GetRaw("hidden") != "''" || cfg.Get("hidden") != "" {
		t.Errorf("Unexpected behavior of stringy options with empty value: GetRaw=%q, Get=%q", cfg.GetRaw("hidden"), cfg.Get("hidden"))
	}
}

func TestDeprecationWarnings(t *testing.T) {
	cmd := NewCommand("mycommand", "summary", "description", nil)
	cmd.AddOption(StringOption("name", 'n', "", "dummy description"))
	cmd.AddOption(StringOption("price", 'p', "", "dummy description").MarkDeprecated("details1"))
	cmd.AddOption(StringOption("category", 'c', "default", "dummy description"))
	cmd.AddOption(StringOption("seats", 's', "8", "dummy description").MarkDeprecated("details2"))

	fileContents := `
name="myproduct"
category=misc
price=100

[beta]
price=5
`

	cliPrice := "Option --price is deprecated. details1"
	cliSeats := "Option --seats is deprecated. details2"
	filePrice := "/tmp/fake.cnf: Option price is deprecated. details1"

	// map of cli flags -> expected warnings
	cases := map[string][]string{
		"":                               {filePrice, filePrice},
		"-p 100 -n foo":                  {cliPrice, filePrice, filePrice},
		"--name=foo --seats=4":           {cliSeats, filePrice, filePrice},
		"--seats 123 -c books":           {cliSeats, filePrice, filePrice},
		"-s 111 -c 444 -p 222 -n myname": {cliSeats, cliPrice, filePrice, filePrice},
	}

	for cliFlags, expected := range cases {
		cfg := ParseFakeCLI(t, cmd, "mycommand "+cliFlags)
		f, err := getParsedFile(cfg, false, fileContents)
		if err != nil {
			t.Fatalf("Unexpected error getting fake parsed file: %v", err)
		}
		cfg.AddSource(f)
		deprWarnings := cfg.DeprecatedOptionUsage()
		if len(deprWarnings) != len(expected) {
			t.Errorf("For command `mycommand %s`, expected %d warnings, instead found %d: %v", cliFlags, len(expected), len(deprWarnings), deprWarnings)
			continue
		}
		slices.Sort(deprWarnings)
		slices.Sort(expected)
		for n := range deprWarnings {
			if deprWarnings[n] != expected[n] {
				t.Errorf("For command `mycommand %s`, expected warning[%d] to be %q, but instead found %q", cliFlags, n, expected[n], deprWarnings[n])
			}
		}
	}
}

func TestSuppliedWithValue(t *testing.T) {
	assertSuppliedWithValue := func(cfg *Config, name string, expected bool) {
		t.Helper()
		if cfg.SuppliedWithValue(name) != expected {
			t.Errorf("Unexpected return from SuppliedWithValue(%q): expected %t, found %t", name, expected, !expected)
		}
	}
	assertPanic := func(cfg *Config, name string) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("Expected SuppliedWithValue(%q) to panic, but it did not", name)
			}
		}()
		cfg.SuppliedWithValue(name)
	}

	cmd := simpleCommand()
	cmd.AddOption(StringOption("optional1", 'y', "", "dummy description").ValueOptional())
	cmd.AddOption(StringOption("optional2", 'z', "default", "dummy description").ValueOptional())

	cfg := ParseFakeCLI(t, cmd, "mycommand -s 'hello world' --skip-truthybool arg1")
	assertPanic(cfg, "doesntexist") // panics if option does not exist
	assertPanic(cfg, "truthybool")  // panics if option isn't string typed
	assertPanic(cfg, "hasshort")    // panics if option value isn't optional
	assertSuppliedWithValue(cfg, "optional1", false)
	assertSuppliedWithValue(cfg, "optional2", false)

	cfg = ParseFakeCLI(t, cmd, "mycommand -y -z arg1")
	assertSuppliedWithValue(cfg, "optional1", false)
	assertSuppliedWithValue(cfg, "optional2", false)

	cfg = ParseFakeCLI(t, cmd, "mycommand -yhello --optional2 arg1")
	assertSuppliedWithValue(cfg, "optional1", true)
	assertSuppliedWithValue(cfg, "optional2", false)

	cfg = ParseFakeCLI(t, cmd, "mycommand --optional2= --optional1='' arg1")
	assertSuppliedWithValue(cfg, "optional1", true)
	assertSuppliedWithValue(cfg, "optional2", true)
}

func TestRuntimeOverride(t *testing.T) {
	assertOptionValue := func(c *Config, name, expected string) {
		t.Helper()
		if actual := c.Get(name); actual != expected {
			t.Errorf("Expected config.Get(%q) = %q, instead found %q", name, expected, actual)
		}
	}
	assertOnCLI := func(c *Config, name string, expected bool) {
		t.Helper()
		if actual := c.OnCLI(name); actual != expected {
			t.Errorf("Expected config.OnCLI(%q) = %t, instead found %t", name, expected, actual)
		}
	}
	assertSetRuntimeOverridePanic := func(c *Config, name string) {
		t.Helper()
		defer func() {
			t.Helper()
			if iface := recover(); iface == nil {
				t.Errorf("Expected SetRuntimeOverride(%q) to panic, but it did not", name)
			}
		}()
		c.SetRuntimeOverride(name, "foo")
	}

	cmd := simpleCommand()
	cmd.AddOption(StringOption("optional1", 'y', "", "dummy description").ValueOptional())
	cmd.AddOption(StringOption("optional2", 'z', "default", "dummy description").ValueOptional())
	cfg := ParseFakeCLI(t, cmd, "mycommand -s 'hello world' --skip-truthybool arg1")

	// Confirm results prior to overrides
	assertOptionValue(cfg, "optional1", "")
	assertOptionValue(cfg, "optional2", "default")
	assertOptionValue(cfg, "hasshort", "hello world")
	assertOnCLI(cfg, "hasshort", true)
	assertOnCLI(cfg, "optional2", false)

	// Confirm behavior of overrides
	cfg.SetRuntimeOverride("hasshort", "overridden1")
	cfg.SetRuntimeOverride("optional2", "overridden2")
	assertSetRuntimeOverridePanic(cfg, "doesnt-exist")
	assertOptionValue(cfg, "hasshort", "overridden1")
	assertOptionValue(cfg, "optional2", "overridden2")
	assertOnCLI(cfg, "hasshort", false)
	assertOnCLI(cfg, "optional2", false)

	// Confirm behaviors of clone, including use of a deep copy of the overrides
	// map, rather than a shared reference
	clone := cfg.Clone()
	assertOptionValue(clone, "optional1", "")
	assertOptionValue(clone, "hasshort", "overridden1")
	assertOptionValue(clone, "optional2", "overridden2")
	assertOnCLI(clone, "hasshort", false)
	assertOnCLI(clone, "optional2", false)
	assertOnCLI(clone, "truthybool", true)
	cfg.SetRuntimeOverride("optional2", "newval")
	assertOptionValue(cfg, "optional2", "newval")
	assertOptionValue(clone, "optional2", "overridden2")
	clone.SetRuntimeOverride("hasshort", "alsonew")
	assertOptionValue(cfg, "hasshort", "overridden1")
	assertOptionValue(clone, "hasshort", "alsonew")

	// Confirm behavior on a dirty config
	clone = cfg.Clone()
	assertSetRuntimeOverridePanic(clone, "doesnt-exist2")
	clone.SetRuntimeOverride("hasshort", "alsonew")
	assertOptionValue(clone, "hasshort", "alsonew")
}

func TestGetRaw(t *testing.T) {
	optionValues := map[string]string{
		"basic":     "foo",
		"nothing":   "",
		"single":    "'quoted'",
		"double":    `"quoted"`,
		"backtick":  "`quoted`",
		"middle":    "something 'something' something",
		"beginning": `"something" something`,
		"end":       "something `something`",
	}
	cfg := simpleConfig(optionValues)

	for name, expected := range optionValues {
		if found := cfg.GetRaw(name); found != expected {
			t.Errorf("Expected GetRaw(%s) to be %s, instead found %s", name, expected, found)
		}
	}
}

func TestGet(t *testing.T) {
	assertBasicGet := func(name, value string) {
		optionValues := map[string]string{
			name: value,
		}
		cfg := simpleConfig(optionValues)
		if actual := cfg.Get(name); actual != value {
			t.Errorf("Expected Get(%s) to return %s, instead found %s", name, value, actual)
		}
	}
	assertQuotedGet := func(name, value, expected string) {
		optionValues := map[string]string{
			name: value,
		}
		cfg := simpleConfig(optionValues)
		if actual := cfg.Get(name); actual != expected {
			t.Errorf("Expected Get(%s) to return %s, instead found %s", name, expected, actual)
		}
	}

	basicValues := map[string]string{
		"basic":            "foo",
		"nothing":          "",
		"uni-start":        "☃snowperson",
		"uni-end":          "snowperson☃",
		"uni-both":         "☃snowperson☃",
		"middle":           "something 'something' something",
		"beginning":        `"something" something`,
		"end":              "something `something`",
		"no-escape1":       `something\'s still backslashed`,
		"no-escape2":       `'even this\'s still backslashed', they said`,
		"not-fully-quoted": `"hello world", I say, "more quoted text but not fully quote wrapped"`,
	}
	for name, value := range basicValues {
		assertBasicGet(name, value)
	}

	quotedValues := [][3]string{
		{"single", "'quoted'", "quoted"},
		{"double", `"quoted"`, "quoted"},
		{"empty", "''", ""},
		{"backtick", "`quoted`", "quoted"},
		{"uni-middle", `"yay ☃ snowpeople"`, `yay ☃ snowpeople`},
		{"esc-quote", `'something\'s escaped'`, `something's escaped`},
		{"esc-esc", `"c:\\tacotown"`, `c:\tacotown`},
		{"esc-rando", `'why\ whatevs'`, `why whatevs`},
		{"esc-uni", `'escaped snowpeople \☃ oh noes'`, `escaped snowpeople ☃ oh noes`},
	}
	for _, tuple := range quotedValues {
		assertQuotedGet(tuple[0], tuple[1], tuple[2])
	}
}

func TestGetAllowEnvVar(t *testing.T) {
	t.Setenv("SOME_VAR", "some value")
	cfg := simpleConfig(map[string]string{
		"int":                       "1",
		"blank":                     "",
		"working-env":               "$SOME_VAR",
		"non-env":                   "SOME_VAR",
		"dollar-literal":            "$",
		"unset-env-blank":           "$OTHER_VAR",
		"quoted-working-env":        `"$SOME_VAR"`,
		"singlequote-no-env":        "'$SOME_VAR'",
		"backtick-no-env":           "`$SOME_VAR`",
		"quoted-dollar-literal":     `"$"`,
		"spaces-working-env":        "  $SOME_VAR\n",
		"spaces-quoted-working-env": ` "$SOME_VAR"   `,
	})

	testCases := map[string]string{
		"int":                       "1",
		"blank":                     "",
		"working-env":               "some value",
		"non-env":                   "SOME_VAR",
		"dollar-literal":            "$",
		"unset-env-blank":           "",
		"quoted-working-env":        "some value",
		"singlequote-no-env":        "$SOME_VAR",
		"backtick-no-env":           "$SOME_VAR",
		"quoted-dollar-literal":     "$",
		"spaces-working-env":        "some value",
		"spaces-quoted-working-env": "some value",
	}
	for input, expected := range testCases {
		if actual := cfg.GetAllowEnvVar(input); actual != expected {
			t.Errorf("Expected cfg.Get(%q) to return %q, instead found %q", input, expected, actual)
		}
	}
}

func TestGetSlice(t *testing.T) {
	assertGetSlice := func(optionValue string, delimiter rune, unwrapFull bool, expected ...string) {
		t.Helper()
		if expected == nil {
			expected = make([]string, 0)
		}
		cfg := simpleConfig(map[string]string{"option-name": optionValue})
		if actual := cfg.GetSlice("option-name", delimiter, unwrapFull); !reflect.DeepEqual(actual, expected) {
			t.Errorf("Expected GetSlice(\"...\", '%c', %t) on %#v to return %#v, instead found %#v", delimiter, unwrapFull, optionValue, expected, actual)
		}
	}

	assertGetSlice("hello", ',', false, "hello")
	assertGetSlice(`hello\`, ',', false, `hello\`)
	assertGetSlice("hello, world", ',', false, "hello", "world")
	assertGetSlice(`outside,"inside, ok?",   also outside`, ',', false, "outside", "inside, ok?", "also outside")
	assertGetSlice(`escaped\,delimiter doesn\'t split, ok?`, ',', false, `escaped\,delimiter doesn\'t split`, "ok?")
	assertGetSlice(`quoted "mid, value" doesn\'t split, either, duh`, ',', false, `quoted "mid, value" doesn\'t split`, "either", "duh")
	assertGetSlice(`'escaping\'s ok to prevent early quote end', yay," ok "`, ',', false, "escaping's ok to prevent early quote end", "yay", "ok")
	assertGetSlice(" space   delimiter", ' ', false, "space", "delimiter")
	assertGetSlice(`'fully wrapped in single quotes, commas still split tho, "nested\'s ok"'`, ',', false, "fully wrapped in single quotes, commas still split tho, \"nested's ok\"")
	assertGetSlice(`'fully wrapped in single quotes, commas still split tho, "nested\'s ok"'`, ',', true, "fully wrapped in single quotes", "commas still split tho", "nested's ok")
	assertGetSlice(`"'quotes',get \"tricky\", right, 'especially \\\' nested'"`, ',', true, "quotes", `get "tricky"`, "right", "especially ' nested")
	assertGetSlice("", ',', false)
	assertGetSlice("   ", ',', false)
	assertGetSlice("   ", ' ', false)
	assertGetSlice("``", ',', true)
	assertGetSlice(" `  `  ", ',', true)
	assertGetSlice(" `  `  ", ' ', true)
}

func TestGetSliceAllowEnvVar(t *testing.T) {
	assertGetSlice := func(viaEnv bool, optionValue string, unwrapFull bool, expected ...string) {
		t.Helper()
		if expected == nil {
			expected = make([]string, 0)
		}
		var configVal string
		if viaEnv {
			configVal = "$FOO"
			t.Setenv("FOO", optionValue)
		} else {
			configVal = optionValue
		}
		cfg := simpleConfig(map[string]string{"option-name": configVal})
		if actual := cfg.GetSliceAllowEnvVar("option-name", ',', unwrapFull); !reflect.DeepEqual(actual, expected) {
			t.Errorf("Expected GetSliceAllowEnv(\"...\", ',', %t) on %#v to return %#v, instead found %#v", unwrapFull, optionValue, expected, actual)
		}
	}

	assertGetSlice(true, "hello", false, "hello")
	assertGetSlice(true, `hello\`, false, `hello\`)
	assertGetSlice(false, "hello", false, "hello")
	assertGetSlice(false, `hello\`, false, `hello\`)
	assertGetSlice(true, "hello, world", false, "hello", "world")
	assertGetSlice(false, "hello, world", false, "hello", "world")
	assertGetSlice(true, "'hello, world'", true, "hello, world")
	assertGetSlice(false, "'hello, world'", false, "hello, world")
	assertGetSlice(false, "'hello, world'", true, "hello", "world")
}

func TestGetEnum(t *testing.T) {
	optionValues := map[string]string{
		"foo":   "bar",
		"caps":  "SHOUTING",
		"blank": "",
	}
	cfg := simpleConfig(optionValues)

	value, err := cfg.GetEnum("foo", "baw", "bar", "bat")
	if value != "bar" || err != nil {
		t.Errorf("Expected bar,nil; found %s,%s", value, err)
	}
	value, err = cfg.GetEnum("foo", "BAW", "BaR", "baT")
	if value != "BaR" || err != nil {
		t.Errorf("Expected BaR,nil; found %s,%s", value, err)
	}
	value, err = cfg.GetEnum("foo", "nope", "dope")
	if value != "" || err == nil {
		t.Errorf("Expected error, found %s,%s", value, err)
	}
	value, err = cfg.GetEnum("caps", "yelling", "shouting")
	if value != "shouting" || err != nil {
		t.Errorf("Expected shouting,nil; found %s,%s", value, err)
	}
	value, err = cfg.GetEnum("blank", "nonblank1", "nonblank2")
	if value != "" || err != nil {
		t.Errorf("Expected empty string to be allowed since it is the default value, but instead found %s,%s", value, err)
	}
}

func TestGetBytes(t *testing.T) {
	optionValues := map[string]string{
		"simple-ok":     "1234",
		"negative-fail": "-3",
		"float-fail":    "4.5",
		"kilo1-ok":      "123k",
		"kilo2-ok":      "234K",
		"megs1-ok":      "12M",
		"megs2-ok":      "440mB",
		"gigs-ok":       "4GB",
		"tera-fail":     "55t",
		"blank-ok":      "",
	}
	cfg := simpleConfig(optionValues)

	assertBytes := func(name string, expect uint64) {
		value, err := cfg.GetBytes(name)
		if err == nil && strings.HasSuffix(name, "_bad") {
			t.Errorf("Expected error for GetBytes(%s) but didn't find one", name)
		} else if err != nil && strings.HasSuffix(name, "-ok") {
			t.Errorf("Unexpected error for GetBytes(%s): %s", name, err)
		}
		if value != expect {
			t.Errorf("Expected GetBytes(%s) to return %d, instead found %d", name, expect, value)
		}
	}

	expected := map[string]uint64{
		"simple-ok":     1234,
		"negative-fail": 0,
		"float-fail":    0,
		"kilo1-ok":      123 * 1024,
		"kilo2-ok":      234 * 1024,
		"megs1-ok":      12 * 1024 * 1024,
		"megs2-ok":      440 * 1024 * 1024,
		"gigs-ok":       4 * 1024 * 1024 * 1024,
		"tera-fail":     0,
		"blank-ok":      0,
	}
	for name, expect := range expected {
		assertBytes(name, expect)
	}
}

func TestGetRegexp(t *testing.T) {
	optionValues := map[string]string{
		"valid":   "^test",
		"invalid": "+++",
		"blank":   "",
	}
	cfg := simpleConfig(optionValues)

	re, err := cfg.GetRegexp("valid")
	if err != nil {
		t.Errorf("Unexpected error for GetRegexp(\"valid\"): %s", err)
	}
	if re == nil || !re.MatchString("testing") {
		t.Error("Regexp returned by GetRegexp(\"valid\") not working as expected")
	}

	re, err = cfg.GetRegexp("invalid")
	if re != nil || err == nil {
		t.Errorf("Expected invalid regexp to return nil and err, instead returned %v, %v", re, err)
	}

	re, err = cfg.GetRegexp("blank")
	if re != nil || err != nil {
		t.Errorf("Expected blank regexp to return nil, nil; instead returned %v, %v", re, err)
	}
}

func TestGetAbsPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Unexpected error getting working directory: %v", err)
	}
	defaultAbs := fmt.Sprintf("%s%cfoobar", filepath.VolumeName(wd), os.PathSeparator)
	cmd := NewCommand("mycommand", "summary", "description", nil)
	cmd.AddOption(StringOption("file1", 'x', "", "dummy description"))
	cmd.AddOption(StringOption("file2", 'y', "default", "dummy description"))
	cmd.AddOption(StringOption("file3", 'z', defaultAbs, "dummy description"))
	cfg := ParseFakeCLI(t, cmd, "mycommand")

	// Test cases for default values
	cases := map[string]string{
		"file1": "",                           // Option with blank default --> return empty string
		"file2": filepath.Join(wd, "default"), // Option with relative default --> base off of wd
		"file3": defaultAbs,                   // Option with absolute default --> return as-is
	}
	for optionName, expected := range cases {
		if actual, err := cfg.GetAbsPath(optionName); actual != expected {
			t.Errorf("Expected GetAbsPath(%q) to return %q, instead got %q with err=%v", optionName, expected, actual, err)
		}
	}

	// Test cases for command-line values
	cfg = ParseFakeCLI(t, cmd, "mycommand --file1=foo/bar --file2="+strings.ReplaceAll(defaultAbs, `\`, `\\`))
	cases = map[string]string{
		"file1": filepath.Join(wd, "foo/bar"),
		"file2": defaultAbs,
	}
	for optionName, expected := range cases {
		if actual, err := cfg.GetAbsPath(optionName); actual != expected {
			t.Errorf("Expected GetAbsPath(%q) to return %q, instead got %q with err=%v", optionName, expected, actual, err)
		}
	}

	// Test cases for relative to option file
	cfg = ParseFakeCLI(t, cmd, "mycommand")
	f, err := getParsedFile(cfg, false, "file1="+defaultAbs+"\nfile2=aaa/bbb\n")
	if err != nil {
		t.Fatalf("Unexpected error getting fake parsed file: %v", err)
	}
	f.Dir = fmt.Sprintf("%s%ctmp", filepath.VolumeName(wd), os.PathSeparator)
	cfg.AddSource(f)
	cases = map[string]string{
		"file1": defaultAbs,
		"file2": filepath.Join(f.Dir, "aaa", "bbb"),
	}
	for optionName, expected := range cases {
		if actual, err := cfg.GetAbsPath(optionName); actual != expected {
			t.Errorf("Expected GetAbsPath(%q) to return %q, instead got %q with err=%v", optionName, expected, actual, err)
		}
	}
}

// simpleConfig returns a stub config based on a single map of key->value string
// pairs. All keys in the map will automatically be considered valid options.
func simpleConfig(values map[string]string) *Config {
	cmd := NewCommand("test", "1.0", "this is for testing", nil)
	for key := range values {
		cmd.AddOption(StringOption(key, 0, "", key))
	}
	cli := &CommandLine{
		Command: cmd,
	}
	return NewConfig(cli, SimpleSource(values))
}
