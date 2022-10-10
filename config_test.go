package mybase

import (
	"reflect"
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
