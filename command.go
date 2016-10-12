package mycli

import (
	"fmt"
	"sort"
	"strings"
)

type CommandHandler func(*Config) error

type Command struct {
	Name          string              // Command name, as used in CLI
	Summary       string              // Short description text. Ignored if ParentCommand is nil.
	Description   string              // Long (multi-line) description/help text
	SubCommands   map[string]*Command // Index of sub-commands
	ParentCommand *Command            // What command this is a sub-command of, or nil if this is the top level
	Handler       CommandHandler      // Callback for processing command. Ignored if len(SubCommands) > 0.
	MinArgs       int                 // minimum number of positional args. Ignored if len(SubCommands) > 0.
	MaxArgs       int                 // maximum number of positional args, or -1 for infinite. Ignored if len(SubCommands) > 0.
	ArgNames      []string            // names of position args, only used in generating help text
	options       map[string]*Option  // Command-specific options
}

// NewCommand creates a standalone command, ie one that does not take sub-
// commands of its own.
func NewCommand(name, summary, description string, handler CommandHandler, minArgs, maxArgs int, argNames ...string) *Command {
	cmd := &Command{
		Name:        name,
		Summary:     summary,
		Description: description,
		Handler:     handler,
		MinArgs:     minArgs,
		MaxArgs:     maxArgs,
		ArgNames:    argNames,
	}

	helpOption := StringOption("help", '?', "", "Display usage information").ValueOptional()
	cmd.AddOption(helpOption)

	return cmd
}

// NewCommandSuite creates a "top-level" Command, typically representing an
// entire program. Intended for use for command suites, e.g. programs with
// sub-commands.
func NewCommandSuite(name, description string) *Command {
	cmd := &Command{
		Name:        name,
		Description: description,
		SubCommands: make(map[string]*Command),
		options:     make(map[string]*Option),
	}

	helpCmd := &Command{
		Name:        "help",
		Description: "Display usage information",
		Summary:     `Display usage information`,
		Handler:     helpHandler,
		MaxArgs:     1,
		ArgNames:    []string{"command"},
	}
	cmd.AddSubCommand(helpCmd)

	helpOption := StringOption("help", '?', "", "Display usage information").ValueOptional()
	cmd.AddOption(helpOption)

	return cmd
}

func (cmd *Command) AddSubCommand(subCmd *Command) {
	if cmd.SubCommands == nil {
		cmd.SubCommands = make(map[string]*Command)
		cmd.Handler = nil
		cmd.MinArgs = 0
		cmd.MaxArgs = 0
		cmd.ArgNames = nil
		helpCmd := &Command{
			Name:        "help",
			Description: "Display usage information",
			Summary:     `Display usage information`,
			MaxArgs:     1,
			Handler:     helpHandler,
		}
		cmd.AddSubCommand(helpCmd)
	}
	subCmd.ParentCommand = cmd
	cmd.SubCommands[subCmd.Name] = subCmd
}

func (cmd *Command) AddOption(opt *Option) {
	if cmd.options == nil {
		cmd.options = make(map[string]*Option)
	}
	cmd.options[opt.Name] = opt
}

// Returns a map of options for this command, recursively merged with its
// parent command. In cases of conflicts, sub-command options override their
// parents / grandparents / etc. The returned map is always a copy, so
// modifications to the map itself will not affect the original cmd.options.
func (cmd *Command) Options() (optMap map[string]*Option) {
	if cmd.ParentCommand == nil {
		optMap = make(map[string]*Option, len(cmd.options))
	} else {
		optMap = cmd.ParentCommand.Options()
	}
	for name := range cmd.options {
		optMap[name] = cmd.options[name]
	}
	return optMap
}

// OptionValue returns the default value of the option with name optionName.
// This is satisfies the OptionValuer interface, and allows a Config to use
// a Command as the lowest-priority option provider in order to return an
// option's default value.
func (cmd *Command) OptionValue(optionName string) (string, bool) {
	options := cmd.Options()
	opt, ok := options[optionName]
	if !ok {
		return "", false
	}
	return opt.Default, true
}

func (cmd *Command) Usage() {
	invocation := cmd.Name
	current := cmd
	for current.ParentCommand != nil {
		current = current.ParentCommand
		invocation = fmt.Sprintf("%s %s", current.Name, invocation)
	}

	fmt.Println(cmd.Description)
	fmt.Println("\nUsage:")
	fmt.Printf("      %s [<options>]%s\n", invocation, cmd.argUsage())

	if len(cmd.SubCommands) > 0 {
		fmt.Println("\nCommands:")
		var maxLen int
		names := make([]string, 0, len(cmd.SubCommands))
		for name := range cmd.SubCommands {
			names = append(names, name)
			if len(name) > maxLen {
				maxLen = len(name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("      %*s  %s\n", -1*maxLen, name, cmd.SubCommands[name].Summary)
		}

	}

	allOptions := cmd.Options()
	if len(allOptions) > 0 {
		fmt.Println("\nOptions:")
		var maxLen int
		names := make([]string, 0, len(allOptions))
		for name := range allOptions {
			names = append(names, name)
			if len(name) > maxLen {
				maxLen = len(name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			opt := allOptions[name]
			fmt.Printf(opt.Usage(maxLen))
		}
	}
}

func (cmd *Command) argUsage() string {
	if len(cmd.SubCommands) > 0 {
		return " <command>"
	}

	var usage string
	var done bool
	var optionalArgs int
	for n := 0; (n < cmd.MaxArgs || cmd.MaxArgs == -1) && !done; n++ {
		var arg string
		if n < len(cmd.ArgNames) {
			arg = fmt.Sprintf("<%s>", cmd.ArgNames[n])
		} else {
			arg = "<arg>"
		}

		// Special case: display multiple optional unnamed args as "..."
		if n+1 >= len(cmd.ArgNames) && n+1 < cmd.MaxArgs && n+1 >= cmd.MinArgs {
			arg = fmt.Sprintf("%s...", arg)
			done = true
		}

		if n < cmd.MinArgs {
			arg = fmt.Sprintf(" %s", arg)
		} else {
			arg = fmt.Sprintf(" [%s", arg)
			optionalArgs++
		}

		usage += arg
	}
	return usage + strings.Repeat("]", optionalArgs)
}

func helpHandler(cfg *Config) error {
	forCommand := cfg.CLI.Command
	if forCommand.ParentCommand != nil {
		forCommand = forCommand.ParentCommand
	}
	if len(cfg.CLI.Args) > 0 && len(forCommand.SubCommands) > 0 {
		forCommandName := cfg.CLI.Args[0]
		var ok bool
		if forCommand, ok = forCommand.SubCommands[forCommandName]; !ok {
			return fmt.Errorf("Unknown command \"%s\"", forCommandName)
		}
	}
	forCommand.Usage()
	return nil
}
