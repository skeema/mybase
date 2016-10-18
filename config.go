package mycli

import (
	"fmt"
	"strconv"
	"strings"
)

// OptionValuer should be implemented by anything that can parse and return
// user-supplied values for options. If the struct has a value corresponding
// to the given optionName, it should return the value along with a true value
// for ok. If the struct does not have a value for the given optionName, it
// should return "", false.
type OptionValuer interface {
	OptionValue(optionName string) (value string, ok bool)
}

// Config represents a list of sources for option values -- the command-line
// plus zero or more option files, or any other source implementing the
// OptionValuer interface.
type Config struct {
	CLI            *CommandLine            // Parsed command-line
	sources        []OptionValuer          // Sources of option values, excluding CLI or Command; higher indexes override lower indexes
	unifiedValues  map[string]string       // Precomputed cache of option name => value
	unifiedSources map[string]OptionValuer // Precomputed cache of option name => which source supplied it
	dirty          bool                    // true if source list has changed, meaning next access needs to recompute caches
}

// NewConfig creates a Config object, given a CommandLine and any arbitrary
// number of other OptionValuer option sources. The order of sources matters:
// in case of conflicts (multiple sources providing the same option value),
// later sources override earlier sources. The CommandLine always overrides
// other sources, and should not be supplied redundantly via sources.
func NewConfig(cli *CommandLine, sources ...OptionValuer) *Config {
	return &Config{
		CLI:     cli,
		sources: sources,
		dirty:   true,
	}
}

// Clone returns a shallow copy of a Config. The copy will point to the same
// CLI value and sources values, but the sources slice itself will be a new
// slice, meaning that a caller can add sources without impacting the original
// Config's source list.
func (cfg *Config) Clone() *Config {
	sourcesCopy := make([]OptionValuer, len(cfg.sources))
	copy(sourcesCopy, cfg.sources)
	return &Config{
		CLI:     cfg.CLI,
		sources: sourcesCopy,
		dirty:   true,
	}
}

// AddSource adds a new OptionValuer to cfg. It will override previously-added
// sources, with the exception of the CommandLine, which always takes
// precedence.
func (cfg *Config) AddSource(source OptionValuer) {
	cfg.sources = append(cfg.sources, source)
	cfg.dirty = true
}

// HandleCommand executes the CommandHandler callback associated with the
// Command that was parsed on the CommandLine.
func (cfg *Config) HandleCommand() error {
	return cfg.CLI.Command.Handler(cfg)
}

// rebuild iterates over all sources, to construct a single cached key-value
// lookup map. This improves performance of subsequent option value lookups.
func (cfg *Config) rebuild() {
	allSources := make([]OptionValuer, 1, len(cfg.sources)+2)

	// Lowest-priority source is the current command, which returns default values
	// for any valid option
	allSources[0] = cfg.CLI.Command

	// Next come cfg.sources, which are already ordered from lowest priority to highest priority
	allSources = append(allSources, cfg.sources...)

	// Finally, at highest priority is options provided on the command-line
	allSources = append(allSources, cfg.CLI)

	options := cfg.CLI.Command.Options()
	cfg.unifiedValues = make(map[string]string, len(options)+len(cfg.CLI.Command.args))
	cfg.unifiedSources = make(map[string]OptionValuer, len(options)+len(cfg.CLI.Command.args))

	// Iterate over positional CLI args. These have highest precedence of all, and
	// are treated as a special-case (not placed in sources and work differently
	// than normal options, since they cannot appear in option files)
	for pos, arg := range cfg.CLI.Command.args {
		cfg.unifiedSources[arg.Name] = cfg.CLI
		if pos < len(cfg.CLI.ArgValues) {
			cfg.unifiedValues[arg.Name] = cfg.CLI.ArgValues[pos]
			delete(options, arg.Name) // shadow any normal option that has same name
		} else {
			// If the arg was optional and not supplied on CLI, use its default value.
			// In this case we intentionally DON'T shadow any normal option with same
			// name, since a supplied option should override an unsupplied arg default.
			cfg.unifiedValues[arg.Name] = arg.Default
		}
	}

	// Iterate over all options, and set them in our maps for tracking values and sources.
	// We go in reverse order to start at highest priority and break early when a value is found.
	for name := range options {
		var found bool
		for n := len(allSources) - 1; n >= 0 && !found; n-- {
			source := allSources[n]
			if value, ok := source.OptionValue(name); ok {
				cfg.unifiedValues[name] = value
				cfg.unifiedSources[name] = source
				found = true
			}
		}
		if !found {
			// If not even the Command provides a value, something is horribly wrong.
			panic(fmt.Errorf("Assertion failed: Iterated over option %s not provided by command %s", name, cfg.CLI.Command.Name))
		}
	}

	cfg.dirty = false
}

func (cfg *Config) rebuildIfDirty() {
	if cfg.dirty {
		cfg.rebuild()
	}
}

// Changed returns true if the specified option name has been set somewhere, or
// false if not (meaning it is still equal to its default value)
func (cfg *Config) Changed(name string) bool {
	cfg.rebuildIfDirty()
	source, ok := cfg.unifiedSources[name]
	if !ok {
		panic(fmt.Errorf("Assertion failed: called Changed on unknown option %s", name))
	}
	switch source.(type) {
	case *Command:
		return false
	default:
		return true
	}
}

// OnCLI returns true if the specified option name was set on the command-line,
// or false otherwise.
func (cfg *Config) OnCLI(name string) bool {
	cfg.rebuildIfDirty()
	source, ok := cfg.unifiedSources[name]
	if !ok {
		panic(fmt.Errorf("Assertion failed: called OnCLI on unknown option %s", name))
	}
	return source == cfg.CLI
}

// FindOption returns an Option by name. It first searches the current command
// hierarchy, but if it fails to find the option there, it then searches all
// other command hierarchies as well. This makes it suitable for use in parsing
// option files, which may refer to options that aren't relevant to the current
// command but exist in some other command.
// Returns nil if no option with that name can be found anywhere.
func (cfg *Config) FindOption(name string) *Option {
	myOptions := cfg.CLI.Command.Options()
	if opt, ok := myOptions[name]; ok {
		return opt
	}

	var helper func(*Command) *Option
	helper = func(cmd *Command) *Option {
		if opt, ok := cmd.options[name]; ok {
			return opt
		}
		for _, sub := range cmd.SubCommands {
			opt := helper(sub)
			if opt != nil {
				return opt
			}
		}
		return nil
	}

	rootCommand := cfg.CLI.Command
	for rootCommand.ParentCommand != nil {
		rootCommand = rootCommand.ParentCommand
	}
	return helper(rootCommand)
}

// Get returns an option's value as a string. If the option is not set, its
// default value will be returned. Panics if the option does not exist, since
// this is indicative of programmer error, not runtime error.
func (cfg *Config) Get(name string) string {
	cfg.rebuildIfDirty()
	value, ok := cfg.unifiedValues[name]
	if !ok {
		panic(fmt.Errorf("Assertion failed: called Get on unknown option %s", name))
	}
	return value
}

// GetBool returns an option's value as a bool. If the option is not set, its
// default value will be returned. Panics if the flag does not exist.
func (cfg *Config) GetBool(name string) bool {
	switch strings.ToLower(cfg.Get(name)) {
	case "false", "off", "0", "":
		return false
	default:
		return true
	}
}

// GetInt returns an option's value as an int. If an error occurs in parsing
// the value as an int, it is returned as the second return value.
func (cfg *Config) GetInt(name string) (int, error) {
	return strconv.Atoi(cfg.Get(name))
}

// GetIntOrDefault is like GetInt, but returns the option's default value if
// parsing the supplied value as an int fails.
func (cfg *Config) GetIntOrDefault(name string) int {
	value, err := cfg.GetInt(name)
	if err != nil {
		defaultValue, _ := cfg.CLI.Command.OptionValue(name)
		value, err = strconv.Atoi(defaultValue)
		if err != nil {
			panic(fmt.Errorf("Assertion failed: default value for option %s is %s, which fails int parsing", name, defaultValue))
		}
	}
	return value
}
