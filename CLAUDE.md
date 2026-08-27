# Money Flow Project Guidelines

## Project Structure

- `go/` - Contains the backend system written in Go. An event sourced architecture is used to handle the intent of money flow, backed by Tigger Beatle database to handing the low level accounting at a Token level. Event log is append-only and immutable, stored in PostgreSQL single table. Protobuf is used to define the domain command and event messages as well as the exposed twirp services.

- `ruby/` - Contains the business backend system written in Ruby. Exposed via GraphQL, services to handle the business logic of money flow with the `go/` backend. The Ruby backend is responsible for handling the business logic and orchestrating the flow of money between different entities. Sequel is used as the ORM to interact with the PostgreSQL database.

- `client/` - Contains the frontend system written in VueJS. The frontend is responsible for providing a user interface for users to interact with the money flow system. It communicates with the Ruby backend via GraphQL to perform various operations related to money flow. VueJS 3 is used along with Apollo Client 4. TailwindCSS is used for styling the frontend components. Apollo Client Composables are used to handle GraphQL queries and mutations in a reactive way and manage the state of the application.

- `proto/` - Contains the Protobuf definitions for the domain command and event messages as well as the exposed twirp services. The Protobuf files are used to generate code for both the Go and Ruby backends.

## Coding

Write tests first, then write code to pass the tests.
After writing code, run linters on your code to ensure it adheres to style guidelines.
Never disable a linter/cop rule to silence a violation — no inline disable comments (e.g. `# rubocop:disable`) and no adding exclusions to linter config files (e.g. `.rubocop.yml`). If a rule's suggestion is genuinely wrong for the code (rename would break behavior, etc.), restructure the code so the rule no longer fires, or ask before overriding it.
In Sorbet-typed Ruby code, best efforts should be taken to avoid `T.untyped` — it opts a value out of static checking entirely. Before reaching for it, look for a real type: a generated RBI (e.g. via `tapioca`), a `T.let`/`T.must` narrowing, a custom shim, or a small typed wrapper around an untyped boundary. Reserve `T.untyped` for boundaries that are genuinely untypeable (e.g. dynamically-defined methods with no RBI, values from a library with no type information) — and leave a short comment explaining why a real type isn't possible there.
Every new Ruby file should start `# typed: strict`. Only drop to a weaker sigil for the same kind of narrow, genuinely-unmodelable case as the existing exceptions — boot/environment setup (`lib/boot.rb`, `lib/environment.rb`, both `# typed: false`) and generated Sequel migrations (`# typed: ignore`, matching the template in `Rakefile`'s `db:new_migration` task) — and leave a comment explaining why `strict` doesn't fit.

- Ruby
- - Testing: Rspec
    Linting/Format: Rubocop
    LSP: ruby-lsp
    Type Checking: Sorbet

- Go
- - Testing: Go test
    Linting/Format: Golangci-lint
    LSP: gopls

- Client (VueJS)
- - Testing: Vue Test Utils
    Linting/Format: ESLint, Prettier
    LSP: TS Server, Vue Language Server
