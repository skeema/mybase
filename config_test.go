package mycli

import (
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
	}
	for name, expect := range expected {
		assertBytes(name, expect)
	}
}
