# create-go-cms

The official command-line tool to create and scaffold **Go CMS** projects.

Go CMS is an open-source, high-performance content management system inspired by Strapi and WordPress Gutenberg, powered by Go and React.

## Quickstart

Create a new project using `npx`:

```bash
npx create-go-cms@latest my-project
cd my-project
npm run dev
```

Or using `yarn`:

```bash
yarn create go-cms my-project
```

Or using `pnpm`:

```bash
pnpm create go-cms my-project
```

## Options

```text
npx create-go-cms [project-name] [options]

Options:
  --mode <type>       CMS mode: 'templated' (default) or 'headless'
  --template <type>   Starter template: 'blog' (default) or 'blank'
  --quickstart, -y    Skip prompts and scaffold with defaults
  -h, --help          Show CLI help
  -v, --version       Show version
```

## Available Scripts in Scaffolded Projects

* `npm run dev`: Starts the development server with auto-migrations.
* `npm run build`: Validates configuration, schemas, and templates for production.
* `npm run start`: Runs the hardened server in production mode.
* `npm run migrate`: Applies pending database migrations.
* `npm run doctor`: Runs health checks on your database and system environment.

## License

MIT © [Go CMS Contributors](https://github.com/go-cms/cms)

