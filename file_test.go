package mybase

import (
	"testing"
)

func getParsedFile(cfg *Config, ignoreUnknownOptions bool, contents string, ignoredOpts ...string) (*File, error) {
	var err error
	file := NewFile("/tmp/fake.cnf")
	file.IgnoreUnknownOptions = ignoreUnknownOptions
	file.IgnoreOptions(ignoredOpts...)
	if len(contents) > 0 {
		file.contents = contents
		file.read = true
		err = file.Parse(cfg)
	}
	return file, err
}

func TestParse(t *testing.T) {
	assertFileParsed := func(f *File, err error, expectedSections ...string) {
		t.Helper()
		if err != nil {
			t.Errorf("Expected file to parse without error, but instead found %s", err)
		} else if len(f.sections) != len(expectedSections) {
			t.Errorf("Expected file to have %d sections, but instead found %d", len(expectedSections), len(f.sections))
		} else {
			for _, name := range expectedSections {
				if _, ok := f.sectionIndex[name]; !ok {
					t.Errorf("Expected section \"%s\" to exist, but it does not", name)
				}
			}
		}
	}
	assertFileValue := func(f *File, sectionName, optionName, value string) {
		t.Helper()
		if section := f.sectionIndex[sectionName]; section == nil {
			t.Errorf("Expected section \"%s\" to exist, but it does not", sectionName)
		} else if actualValue, ok := section.Values[optionName]; !ok || actualValue != value {
			t.Errorf("Expected section \"%s\" value of %s to be \"%s\", instead found \"%s\" (set=%t)", sectionName, optionName, value, actualValue, ok)
		}
	}

	cmd := NewCommand("test", "1.0", "this is for testing", nil)
	cmd.AddOption(StringOption("mystring", 0, "", ""))
	cmd.AddOption(BoolOption("mybool", 0, false, ""))
	cli := &CommandLine{
		Command: cmd,
	}
	cfg := NewConfig(cli)

	f, err := getParsedFile(cfg, false, "mystring=hello\nmybool")
	assertFileParsed(f, err, "")
	assertFileValue(f, "", "mystring", "hello")
	assertFileValue(f, "", "mybool", "1")

	f, err = getParsedFile(cfg, false, "skip-mybool\n mystring =  whatever \n\n\t[one] #yay\nmybool=1\n[two]\nloose-mystring=overridden\n\n\n")
	assertFileParsed(f, err, "", "one", "two")
	assertFileValue(f, "", "mystring", "whatever")
	assertFileValue(f, "", "mybool", "")
	assertFileValue(f, "one", "mybool", "1")
	assertFileValue(f, "two", "mystring", "overridden")

	f, err = getParsedFile(cfg, false, "loose-doesntexist=foo\n\n\nmystring=`ok`  ")
	assertFileParsed(f, err, "")
	assertFileValue(f, "", "mystring", "`ok`")

	f, err = getParsedFile(cfg, true, "errors-dont-matter=1")
	assertFileParsed(f, err, "")
	f, err = getParsedFile(cfg, false, "errors-dont-matter=1", "errors-dont-matter")
	assertFileParsed(f, err, "")

	badContents := []string{
		"mystring=hello\ninvalid=fail",
		"mybool=true\nmystring\n",
		"[foo\nmybool\n",
	}
	for _, contents := range badContents {
		f, err = getParsedFile(cfg, false, contents)
		if err == nil {
			t.Errorf("Expected file parsing to generate error, but it did not. Contents:\n%s\n\n", f.contents)
		}
	}

	// Test Config.LooseFileOptions
	cfg.LooseFileOptions = true
	f, err = getParsedFile(cfg, false, "[one]\nerrors-dont-matter=1\nmystring=hello")
	assertFileParsed(f, err, "", "one")
	assertFileValue(f, "one", "mystring", "hello")
}

func TestParseLine(t *testing.T) {
	assertLine := func(line, sectionName, key, value, comment string, kind lineType, isLoose bool) {
		result, err := parseLine(line)
		if err != nil {
			t.Errorf("Unexpected error result from parsing line \"%s\": %s", line, err)
			return
		}
		expect := parsedLine{
			sectionName: sectionName,
			key:         key,
			value:       value,
			comment:     comment,
			kind:        kind,
			isLoose:     isLoose,
		}
		if *result != expect {
			t.Errorf("Result %v does not match expectation %v", *result, expect)
		}
	}
	assertLineHasErr := func(line string) {
		_, err := parseLine(line)
		if err == nil {
			t.Errorf("Expected error result from parsing line \"%s\", but no error returned", line)
		}
	}

	assertLine("", "", "", "", "", lineTypeBlank, false)
	assertLine("; comments are cool right", "", "", "", " comments are cool right", lineTypeComment, false)
	assertLine("#so are these", "", "", "", "so are these", lineTypeComment, false)
	assertLine("  [awesome]  # very nice section", "awesome", "", "", " very nice section", lineTypeSectionHeader, false)
	assertLine("[]", "", "", "", "", lineTypeSectionHeader, false)
	assertLine("   [cool beans]   # awesome section", "cool beans", "", "", " awesome section", lineTypeSectionHeader, false)
	assertLine("  foo", "", "foo", "", "", lineTypeKeyOnly, false)
	assertLine(" loose-foo#sup=dup'whatever'", "", "foo", "", "sup=dup'whatever'", lineTypeKeyOnly, true)
	assertLine("this  =  that  =  whatever  # okie dokie", "", "this", "that  =  whatever", " okie dokie", lineTypeKeyValue, false)
	assertLine("loose_something=\"quoted value # ignores value's # comments\" # until after value's \"quotes\"", "", "something", "\"quoted value # ignores value's # comments\"", " until after value's \"quotes\"", lineTypeKeyValue, true)
	assertLine("  backticks-work = `yep working fine`   ", "", "backticks-work", "`yep working fine`", "", lineTypeKeyValue, false)
	assertLine("foo='first' part of value only is quoted", "", "foo", "'first' part of value only is quoted", "", lineTypeKeyValue, false)
	assertLine("foo='first' and last parts of value are 'quoted'", "", "foo", "'first' and last parts of value are 'quoted'", "", lineTypeKeyValue, false)

	assertLineHasErr("[section")
	assertLineHasErr("[section   # hmmm")
	assertLineHasErr("[section] lol # lolol")
	assertLineHasErr(`"key"="value"`)
	assertLineHasErr("key\\=still-key = value")
	assertLineHasErr(`no-terminator = "this quote does not end`)
	assertLineHasErr(`foo=bar\`)
	assertLineHasErr("foo=\"mismatched quotes`")
	assertLineHasErr("foo=`unbalanced`quotes`")
}
