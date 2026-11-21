package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-cms/cms/internal/cli/scaffold"
)

// RunCreate handles project creation and scaffolding.
func RunCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	modeFlag := fs.String("mode", "", "CMS mode: templated or headless")
	templateFlag := fs.String("template", "", "Starter template: blog or blank")
	dbDriverFlag := fs.String("db", "postgres", "Database driver (postgres)")
	dbHostFlag := fs.String("db-host", "localhost", "Database host")
	dbPortFlag := fs.Int("db-port", 5432, "Database port")
	dbUserFlag := fs.String("db-user", "postgres", "Database user")
	dbPassFlag := fs.String("db-pass", "", "Database password")
	dbNameFlag := fs.String("db-name", "", "Database name")
	portFlag := fs.Int("port", 8080, "HTTP server port")
	quickstartFlag := fs.Bool("quickstart", false, "Quickstart with default options")
	fs.BoolVar(quickstartFlag, "y", false, "Quickstart with default options")

	var cleanArgs []string
	var projectName string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && projectName == "" {
			projectName = arg
		} else {
			cleanArgs = append(cleanArgs, arg)
		}
	}

	if err := fs.Parse(cleanArgs); err != nil {
		return err
	}

	if projectName == "" && len(fs.Args()) > 0 {
		projectName = fs.Args()[0]
	}

	reader := bufio.NewReader(os.Stdin)

	// If project name not provided, prompt for it
	if projectName == "" {
		if *quickstartFlag {
			projectName = "my-go-cms"
		} else {
			fmt.Print("? What is your project named? (my-go-cms): ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				projectName = "my-go-cms"
			} else {
				projectName = input
			}
		}
	}

	// Validate project name
	projectName = strings.TrimSpace(projectName)
	projectName = strings.ReplaceAll(projectName, " ", "-")

	mode := scaffold.ModeTemplated
	if *modeFlag != "" {
		if strings.ToLower(*modeFlag) == "headless" {
			mode = scaffold.ModeHeadless
		}
	} else if !*quickstartFlag {
		fmt.Println("\n? Which CMS mode would you like to use?")
		fmt.Println("  1) Templated CMS — Go HTML templates, themes, visual page builder (Recommended)")
		fmt.Println("  2) Headless CMS  — Pure JSON REST APIs, content management, media")
		fmt.Print("Select [1-2] (1): ")
		choice, _ := reader.ReadString('\n')
		if strings.TrimSpace(choice) == "2" {
			mode = scaffold.ModeHeadless
		}
	}

	templateType := scaffold.TemplateBlog
	if *templateFlag != "" {
		if strings.ToLower(*templateFlag) == "blank" {
			templateType = scaffold.TemplateBlank
		}
	} else if !*quickstartFlag {
		fmt.Println("\n? Choose a starter template:")
		fmt.Println("  1) Blog starter — Complete with blog posts, articles, and home page (Recommended)")
		fmt.Println("  2) Blank        — Clean minimal project")
		fmt.Print("Select [1-2] (1): ")
		choice, _ := reader.ReadString('\n')
		if strings.TrimSpace(choice) == "2" {
			templateType = scaffold.TemplateBlank
		}
	}

	dbHost := *dbHostFlag
	dbPort := *dbPortFlag
	dbUser := *dbUserFlag
	dbPass := *dbPassFlag
	dbName := *dbNameFlag
	if dbName == "" {
		dbName = strings.ReplaceAll(projectName, "-", "_") + "_db"
	}

	if !*quickstartFlag && *modeFlag == "" {
		fmt.Printf("\n? Database settings for PostgreSQL (press Enter to accept default):\n")
		fmt.Printf("  Database host (%s): ", dbHost)
		if h, _ := reader.ReadString('\n'); strings.TrimSpace(h) != "" {
			dbHost = strings.TrimSpace(h)
		}
		fmt.Printf("  Database port (%d): ", dbPort)
		if p, _ := reader.ReadString('\n'); strings.TrimSpace(p) != "" {
			if parsed, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				dbPort = parsed
			}
		}
		fmt.Printf("  Database user (%s): ", dbUser)
		if u, _ := reader.ReadString('\n'); strings.TrimSpace(u) != "" {
			dbUser = strings.TrimSpace(u)
		}
		fmt.Print("  Database password: ")
		if pw, _ := reader.ReadString('\n'); strings.TrimSpace(pw) != "" {
			dbPass = strings.TrimSpace(pw)
		}
		fmt.Printf("  Database name (%s): ", dbName)
		if dn, _ := reader.ReadString('\n'); strings.TrimSpace(dn) != "" {
			dbName = strings.TrimSpace(dn)
		}
	}

	fmt.Printf("\n⏳ Creating a new Go CMS app in ./%s...\n", projectName)

	opts := scaffold.Options{
		ProjectName: projectName,
		TargetDir:   projectName,
		Mode:        mode,
		Template:    templateType,
		DBDriver:    *dbDriverFlag,
		DBHost:      dbHost,
		DBPort:      dbPort,
		DBUser:      dbUser,
		DBPassword:  dbPass,
		DBName:      dbName,
		ServerPort:  *portFlag,
	}

	if err := scaffold.Generate(opts); err != nil {
		return fmt.Errorf("project creation failed: %w", err)
	}

	// Print Next.js / Strapi style success banner
	fmt.Printf(`
✨ Success! Created %s at ./%s
Inside that directory, you can run several commands:

  npm run dev
    Starts the development server with auto-migrations.

  npm run build
    Validates themes, schemas, and configuration for production.

  npm run start
    Runs the hardened server in production mode.

  npm run migrate
    Applies pending database migrations.

  npm run doctor
    Runs system diagnostics on your database and configuration.

We suggest that you begin by typing:

  cd %s
  npm run dev

Happy hacking with Go CMS! 🚀
`, projectName, projectName, projectName)

	return nil
}

