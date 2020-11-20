package mybase

import (
	"fmt"
	"strings"
	"testing"
)

func TestCommandInvocation(t *testing.T) {
	single := simpleCommand()
	expected := "mycommand [<options>] <required> [<optional>]"
	if actual := single.Invocation(); actual != expected {
		t.Errorf("Incorrect result from Invocation() for simple command: expected=%q, actual=%q", expected, actual)
	}

	suite := simpleCommandSuite()
	expected = "mycommand [<options>] <command>"
	if actual := suite.Invocation(); actual != expected {
		t.Errorf("Incorrect result from Invocation() for command suite root: expected=%q, actual=%q", expected, actual)
	}
	subOne := suite.SubCommands["one"]
	expected = "mycommand one [<options>]"
	if actual := subOne.Invocation(); actual != expected {
		t.Errorf("Incorrect result from Invocation() for subcommand one: expected=%q, actual=%q", expected, actual)
	}
	subTwo := suite.SubCommands["two"]
	expected = "mycommand two [<options>] [<optional>]"
	if actual := subTwo.Invocation(); actual != expected {
		t.Errorf("Incorrect result from Invocation() for subcommand two: expected=%q, actual=%q", expected, actual)
	}
}

func TestCommandOptionGroups(t *testing.T) {
	cmd := simpleCommand()
	cmd.AddOptions("global",
		StringOption("another", 0, "", "dummy description"),
	)
	cmd.AddOptions("wontshow",
		StringOption("alsohidden", 0, "", "dummy description").Hidden(),
	)
	cmd.AddOptions("widgets",
		StringOption("weight", 0, "", "dummy description"),
		StringOption("size", 0, "", "dummy description"),
	)
	actual := cmd.OptionGroups()
	expectedGroupNames := []string{"", "widgets", "global"}
	expectedOptionNames := [][]string{
		{"bool1", "bool2", "hasshort", "truthybool", "visible"},
		{"size", "weight"},
		{"another", "help", "version"},
	}
	if len(actual) != len(expectedGroupNames) {
		t.Fatalf("Expected %d option groups; instead found %d", len(expectedGroupNames), len(actual))
	}
	for i, grp := range actual {
		if grp.Name != expectedGroupNames[i] {
			t.Errorf("Expected group[%d] to have name %q; instead found %q", i, expectedGroupNames[i], grp.Name)
		}
		if len(grp.Options) != len(expectedOptionNames[i]) {
			t.Errorf("Expected group[%d] to have %d options; instead found %d", i, len(expectedOptionNames[i]), len(grp.Options))
		} else {
			for j, opt := range grp.Options {
				if opt.Name != expectedOptionNames[i][j] {
					t.Errorf("Expected group[%d].Options[%d] to be %q, instead found %q", i, j, expectedOptionNames[i][j], opt.Name)
				}
			}
		}
	}
}

func TestWebDocText(t *testing.T) {
	single := simpleCommand()
	actual := single.WebDocText()
	if strings.Contains(actual, "command suite") {
		t.Errorf("Unexpected reference to command suite in non-suite web doc text: %q", actual)
	}
	if !strings.HasSuffix(actual, single.WebDocURL) {
		t.Errorf("Expected web doc link to be %q, but full text did not have that suffix: %q", single.WebDocURL, actual)
	}
	single.WebDocURL = ""
	if actual := single.WebDocText(); actual != "" {
		t.Errorf("Expected single command with no web doc to return empty string; instead found %q", actual)
	}

	suite := simpleCommandSuite()
	actual = suite.WebDocText()
	if !strings.Contains(actual, "command suite") {
		t.Errorf("Unexpectedly lacking reference to command suite in web doc text: %q", actual)
	}
	if !strings.HasSuffix(actual, suite.WebDocURL) {
		t.Errorf("Expected web doc link to be %q, but full text did not have that suffix: %q", suite.WebDocURL, actual)
	}
	subOne := suite.SubCommands["one"]
	actual = subOne.WebDocText()
	if strings.Contains(actual, "command suite") {
		t.Errorf("Unexpected reference to command suite in non-suite web doc text: %q", actual)
	}
	if !strings.HasSuffix(actual, fmt.Sprintf("%s/%s", suite.WebDocURL, subOne.Name)) {
		t.Errorf("Expected web doc link to be %q, but full text did not have that suffix: %q", subOne.WebDocURL, actual)
	}
}

// simpleCommand returns a standalone command for testing purposes
func simpleCommand() *Command {
	cmd := NewCommand("mycommand", "summary", "description", nil)
	cmd.AddOption(StringOption("visible", 0, "", "dummy description"))
	cmd.AddOption(StringOption("hidden", 0, "somedefault", "dummy description").Hidden())
	cmd.AddOption(StringOption("hasshort", 's', "", "dummy description"))
	cmd.AddOption(BoolOption("bool1", 'b', false, "dummy description"))
	cmd.AddOption(BoolOption("bool2", 'B', false, "dummy description"))
	cmd.AddOption(BoolOption("truthybool", 0, true, "dummy description"))
	cmd.AddArg("required", "", true)
	cmd.AddArg("optional", "hello", false)
	cmd.WebDocURL = "https://www.indexhint.com/test/cmddoc"
	return cmd
}

// simpleCommandSuite returns a command suite for testing purposes
func simpleCommandSuite() *Command {
	suite := NewCommandSuite("mycommand", "summary", "description")
	suite.AddOption(StringOption("visible", 0, "", "dummy description"))
	suite.AddOption(StringOption("hidden", 0, "somedefault", "dummy description").Hidden())
	suite.AddOption(StringOption("hasshort", 's', "", "dummy description"))
	suite.AddOption(BoolOption("bool1", 'b', false, "dummy description"))
	suite.AddOption(BoolOption("bool2", 'B', false, "dummy description"))
	suite.AddOption(BoolOption("truthybool", 0, true, "dummy description"))
	suite.WebDocURL = "https://www.indexhint.com/test/suitedoc"

	cmd1 := NewCommand("one", "summary", "description", nil)
	cmd1.AddOption(StringOption("visible", 0, "newdefault", "dummy description")) // changed default
	cmd1.AddOption(StringOption("hidden", 0, "somedefault", "dummy description")) // no longer hidden
	cmd1.AddOption(StringOption("newopt", 'n', "", "dummy description"))
	suite.AddSubCommand(cmd1)

	cmd2 := NewCommand("two", "summary", "description", nil)
	cmd2.AddArg("optional", "hello", false)
	suite.AddSubCommand(cmd2)

	return suite
}
