package mybase

import (
	"io/ioutil"
	"os"
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

func TestFileReadWrite(t *testing.T) {
	f := NewFile(os.TempDir(), "mybasetest.cnf")
	if f.Exists() {
		t.Fatalf("File at path %s unexpectedly already exists", f.Path())
	}
	if err := f.Read(); err == nil {
		t.Fatal("Expected File.Read() to fail on nonexistent file, but err is nil")
	}

	contents := "foo=bar\noptional-string\n\n[mysection]\nskip-foo\nskip-safeties\nsimple-bool\n"
	err := ioutil.WriteFile(f.Path(), []byte(contents), 0777)
	if err != nil {
		t.Fatalf("Unable to directly write %s to set up test: %s", f.Path(), err)
	}
	defer os.Remove(f.Path())

	if !f.Exists() {
		t.Error("Expected File.Exists() to return true, but it did not")
	}
	if err := f.Read(); err != nil {
		t.Fatalf("Unexpected error from File.Read(): %v", err)
	}
	if f.contents != contents {
		t.Errorf("Unexpected f.contents: %q", f.contents)
	}
	if !f.read {
		t.Error("Expected f.read to be true after calling Read(), but it is false")
	}

	cmd := NewCommand("test", "1.0", "this is for testing", nil)
	cmd.AddOption(StringOption("foo", 0, "", ""))
	cmd.AddOption(StringOption("optional-string", 0, "", "").ValueOptional())
	cmd.AddOption(BoolOption("safeties", 0, true, ""))
	cmd.AddOption(BoolOption("simple-bool", 0, false, ""))
	cli := &CommandLine{
		Command: cmd,
	}
	cfg := NewConfig(cli)
	if err := f.Parse(cfg); err != nil {
		t.Fatalf("Unexpected error from Parse(): %v", err)
	}

	// Non-overwrite Write should fail, but overwrite-friendly should be fine
	if err := f.Write(false); err == nil {
		t.Error("Expected Write(false) to fail, but it did not")
	}
	if err := f.Write(true); err != nil {
		t.Errorf("Unexpected error from Write(true): %v", err)
	}
	newContentsBytes, err := ioutil.ReadFile(f.Path())
	if err != nil {
		t.Fatalf("Unexpected error directly re-reading file: %v", err)
	}
	if string(newContentsBytes) != contents {
		t.Errorf("Unexpected file contents: %q", string(newContentsBytes))
	}
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

	f, err = getParsedFile(cfg, false, "mystring = \nskip-mybool")
	assertFileParsed(f, err, "")
	assertFileValue(f, "", "mystring", "''")

	f, err = getParsedFile(cfg, false, "skip-mybool\n mystring =  whatever \n\n\t[one] #yay\nmybool=1\n[two]\nloose-mystring=overridden\n\n\n")
	assertFileParsed(f, err, "", "one", "two")
	assertFileValue(f, "", "mystring", "whatever")
	assertFileValue(f, "", "mybool", "")
	assertFileValue(f, "one", "mybool", "1")
	assertFileValue(f, "two", "mystring", "overridden")
	AssertFileSetsOptions(t, f, "mybool", "mystring")
	AssertFileMissingOptions(t, f, "yay")

	// Test with utf8 BOM in front of contents
	f, err = getParsedFile(cfg, false, "\uFEFFskip-mybool\n mystring =  whatever \n\n\t[one] #yay\nmybool=1\n[two]\nloose-mystring=overridden\n\n\n")
	assertFileParsed(f, err, "", "one", "two")
	assertFileValue(f, "", "mystring", "whatever")

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

func TestFileSameContents(t *testing.T) {
	cmd := NewCommand("test", "1.0", "this is for testing", nil)
	cmd.AddOption(StringOption("mystring", 0, "", ""))
	cmd.AddOption(BoolOption("mybool", 0, false, ""))
	cli := &CommandLine{
		Command: cmd,
	}
	cfg := NewConfig(cli)

	f1, err1 := getParsedFile(cfg, false, "skip-mybool\n mystring =  whatever \n\n\t[one] #yay\nmybool=1\n[two]\nloose-mystring=overridden\n\n\n")
	f2, err2 := getParsedFile(cfg, false, "mystring = whatever\nskip-mybool\n[one]\nmybool=1\n[two]\nmystring=overridden\n")
	if err1 != nil || err2 != nil {
		t.Fatalf("Unexpected errors in getting parsed test files: %v / %v", err1, err2)
	}
	if !f1.SameContents(f2) {
		t.Error("Expected f1 and f2 to have the same contents, but they did not")
	}

	f3, err3 := getParsedFile(cfg, false, "skip-mybool\n[one]\nmybool=1\n[two]\nmystring=overridden\n")
	if err3 != nil {
		t.Fatalf("Unexpected error in getting parsed test file: %v", err3)
	}
	if f1.SameContents(f3) {
		t.Error("Expected f1 and f3 to have different contents, but SameContents returned true")
	}
	f3.SetOptionValue("", "mystring", "some other value")
	if f1.SameContents(f3) {
		t.Error("Expected f1 and f3 to have different contents, but SameContents returned true")
	}
	f3.SetOptionValue("", "mystring", "whatever")
	if !f1.SameContents(f3) {
		t.Error("Expected f1 and f3 to now have the same contents, but they did not")
	}
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
