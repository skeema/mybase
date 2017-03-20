package mybase

import (
	"reflect"
	"strings"
	"testing"
)

type dummySource map[string]string

func (source dummySource) OptionValue(optionName string) (string, bool) {
	val, ok := source[optionName]
	return val, ok
}

// getConfig returns a stub config based on a single map of key->value string
// pairs. All keys in the map will automatically be considered valid options.
func getConfig(values map[string]string) *Config {
	cmd := NewCommand("test", "1.0", "this is for testing", nil)
	for key := range values {
		cmd.AddOption(StringOption(key, 0, "", key))
	}
	cli := &CommandLine{
		Command: cmd,
	}
	return NewConfig(cli, dummySource(values))
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
	cfg := getConfig(optionValues)

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
		cfg := getConfig(optionValues)
		if actual := cfg.Get(name); actual != value {
			t.Errorf("Expected Get(%s) to return %s, instead found %s", name, value, actual)
		}
	}
	assertQuotedGet := func(name, value, expected string) {
		optionValues := map[string]string{
			name: value,
		}
		cfg := getConfig(optionValues)
		if actual := cfg.Get(name); actual != expected {
			t.Errorf("Expected Get(%s) to return %s, instead found %s", name, expected, actual)
		}
	}

	basicValues := map[string]string{
		"basic":      "foo",
		"nothing":    "",
		"uni-start":  "☃snowperson",
		"uni-end":    "snowperson☃",
		"uni-both":   "☃snowperson☃",
		"middle":     "something 'something' something",
		"beginning":  `"something" something`,
		"end":        "something `something`",
		"no-escape1": `something\'s still backslashed`,
		"no-escape2": `'even this\'s still backslashed', they said`,
	}
	for name, value := range basicValues {
		assertBasicGet(name, value)
	}

	quotedValues := [][3]string{
		{"single", "'quoted'", "quoted"},
		{"double", `"quoted"`, "quoted"},
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

func TestGetSlice(t *testing.T) {
	assertGetSlice := func(optionValue string, delimiter rune, unwrapFull bool, expected ...string) {
		if expected == nil {
			expected = make([]string, 0)
		}
		cfg := getConfig(map[string]string{"option-name": optionValue})
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
	cfg := getConfig(optionValues)

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
	cfg := getConfig(optionValues)

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
