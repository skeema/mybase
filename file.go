package mycli

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"unicode"
)

type Section struct {
	Name   string
	Values map[string]string
}

type File struct {
	Dir                  string
	Name                 string
	Sections             []*Section
	SectionIndex         map[string]*Section
	IgnoreUnknownOptions bool
	read                 bool
	parsed               bool
	contents             string
	selected             []string
}

func NewFile(pathAndName string) *File {
	return &File{
		Dir:          path.Dir(pathAndName),
		Name:         path.Base(pathAndName),
		Sections:     make([]*Section, 0),
		SectionIndex: make(map[string]*Section),
	}
}

func (f *File) Path() string {
	return path.Join(f.Dir, f.Name)
}

func (f *File) Write(overwrite bool) error {
	lines := make([]string, 0)
	for n, section := range f.Sections {
		if section.Name != "" {
			lines = append(lines, fmt.Sprintf("[%s]", section.Name))
		}
		for k, v := range section.Values {
			lines = append(lines, fmt.Sprintf("%s=%s", k, v))
		}
		if n < len(f.Sections)-1 {
			lines = append(lines, "")
		}
	}

	if len(lines) == 0 {
		log.Printf("Skipping write to %s due to empty configuration", f.Path())
		return nil
	}
	f.contents = fmt.Sprintf("%s\n", strings.Join(lines, "\n"))

	flag := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flag |= os.O_TRUNC
	} else {
		flag |= os.O_EXCL
	}
	osFile, err := os.OpenFile(f.Path(), flag, 0666)
	if err != nil {
		return err
	}
	n, err := osFile.Write([]byte(f.contents))
	if err == nil && n < len(f.contents) {
		err = io.ErrShortWrite
	}
	if err1 := osFile.Close(); err == nil {
		err = err1
	}
	return err
}

// Read loads the contents of the option file, but does not parse it.
func (f *File) Read() error {
	file, err := os.Open(f.Path())
	if err != nil {
		return err
	}
	defer file.Close()
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	f.contents = string(bytes)
	f.read = true
	return nil
}

func (f *File) Parse(cfg *Config) error {
	if !f.read {
		if err := f.Read(); err != nil {
			return err
		}
	}

	section := &Section{
		Name:   "",
		Values: make(map[string]string),
	}
	f.Sections = append(f.Sections, section)
	f.SectionIndex[""] = section

	var lineNumber int
	scanner := bufio.NewScanner(strings.NewReader(f.contents))
	for scanner.Scan() {
		line := scanner.Text()
		lineNumber++
		line = strings.TrimLeftFunc(line, unicode.IsSpace)
		if line == "" {
			continue
		}
		if line[0] == '[' {
			name := line[1 : len(line)-1]
			if s, exists := f.SectionIndex[name]; exists {
				section = s
			} else {
				section = &Section{
					Name:   name,
					Values: make(map[string]string),
				}
				f.Sections = append(f.Sections, section)
				f.SectionIndex[name] = section
			}
			continue
		}

		tokens := strings.SplitN(line, "#", 2)
		key, value, loose := NormalizeOptionToken(tokens[0])
		source := fmt.Sprintf("%s line %d", f.Path(), lineNumber)
		opt := cfg.FindOption(key)
		if opt == nil {
			if loose || f.IgnoreUnknownOptions {
				continue
			} else {
				return OptionNotDefinedError{key, source}
			}
		}
		if value == "" {
			if opt.RequireValue {
				return OptionMissingValueError{opt.Name, source}
			} else if opt.Type == OptionTypeBool {
				// Option without value indicates option is being enabled if boolean
				value = "1"
			}
		}

		section.Values[key] = value
	}

	f.parsed = true
	f.selected = []string{""}
	return scanner.Err()
}

// UseSection changes which section(s) of the file are used when calling
// OptionValue. If multiple section names are supplied, multiple sections will
// be checked by OptionValue, with sections listed first taking precedence over
// subsequent ones.
// Note that the default nameless section "" (i.e. lines at the top of the file
// prior to a section header) is automatically appended to the end of the list.
// So this section is always checked, at lowest priority, need not be
// passed to this function.
func (f *File) UseSection(names ...string) error {
	notFound := make([]string, 0)
	already := make(map[string]bool, len(names))
	f.selected = make([]string, 0, len(names)+1)

	for _, name := range names {
		if already[name] {
			continue
		}
		already[name] = true
		if _, ok := f.SectionIndex[name]; ok {
			f.selected = append(f.selected, name)
		} else {
			notFound = append(notFound, name)
		}
	}
	if !already[""] {
		f.selected = append(names, "")
	}

	if len(notFound) == 0 {
		return nil
	}
	return fmt.Errorf("File %s missing section: %s", f.Path(), strings.Join(notFound, ", "))
}

// OptionValue returns the value for the requested option from the option file.
// Only the previously-selected section(s) of the file will be used, or the
// default section "" if no section has been selected via UseSection.
// Panics if the file has not yet been parsed, as this would indicate a bug.
// This is satisfies the OptionValuer interface, allowing Files to be used as
// an option source in Config.
func (f *File) OptionValue(optionName string) (string, bool) {
	if !f.parsed {
		panic(fmt.Errorf("Call to OptionValue(\"%s\") on unparsed file %s", optionName, f.Path()))
	}
	for _, sectionName := range f.selected {
		section := f.SectionIndex[sectionName]
		if value, ok := section.Values[optionName]; ok {
			return value, true
		}
	}
	return "", false
}
