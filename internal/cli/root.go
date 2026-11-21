package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Execute runs the main Go CMS CLI application.
func Execute(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}

	cmd := strings.ToLower(args[0])
	subArgs := args[1:]

	switch cmd {
	case "create", "new", "init":
		return RunCreate(subArgs)

	case "develop", "dev":
		fs := flag.NewFlagSet("develop", flag.ContinueOnError)
		configPath := fs.String("config", "", "Path to configuration file")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		return RunServer(*configPath, false)

	case "start":
		fs := flag.NewFlagSet("start", flag.ContinueOnError)
		configPath := fs.String("config", "", "Path to configuration file")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		return RunServer(*configPath, true)

	case "build":
		fs := flag.NewFlagSet("build", flag.ContinueOnError)
		configPath := fs.String("config", "", "Path to configuration file")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		return RunBuild(*configPath)

	case "migrate":
		fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
		configPath := fs.String("config", "", "Path to configuration file")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		return RunMigrate(*configPath, fs.Args())

	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		configPath := fs.String("config", "", "Path to configuration file")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		return RunDoctor(*configPath)

	case "version", "-v", "--version":
		return RunVersion()

	case "help", "-h", "--help":
		printHelp()
		return nil

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q.\n\n", cmd)
		printHelp()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func printHelp() {
	fmt.Printf(`Go CMS CLI %s
The high-performance, open-source content management system.

Usage:
  go-cms <command> [options]

Commands:
  create   [name]   Create a new Go CMS project (interactive or flags)
  develop           Start development server with auto-migrations
  start             Start production server with caching enabled
  build             Validate themes, schemas, and configuration
  migrate  [action] Run database migrations ('up' or 'status')
  doctor            Diagnose system, database, and configuration health
  version           Print Go CMS CLI version details
  help              Display this help message

Options:
  --config <path>   Path to custom config.yaml (defaults to ./config.yaml)
  -h, --help        Show help for command
  -v, --version     Show version details

Quickstart:
  go-cms create my-project
  cd my-project
  go-cms develop

Learn more at https://github.com/go-cms/cms
`, Version)
}

