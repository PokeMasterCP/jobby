# Project Overview

This is a personal job application tracker designed for local-only use and run inside a Docker container.

## Technology Stack

- Go backend
- SQLite database
- Goose for database migrations
- `sqlc` for SQL queries and generated database access code
- Server-rendered HTML
- HTMX for client-side interactions
- CSS
- Docker

Prefer the Go standard library and existing project dependencies where practical. Avoid introducing frameworks or additional dependencies unless they provide a clear benefit. Discuss significant new dependencies before adding them.

# Development Approach

Work incrementally. Prefer small, focused changes over implementing multiple features or large architectural changes at once.

Implement the smallest coherent change needed to accomplish the current task. Do not expand the scope beyond what was requested without discussing it first.

When a task requires a larger change, break it into smaller steps and explain the proposed approach before implementing it.

Keep me involved in architectural decisions and changes that materially affect the project's structure or data flow.

# Testing

Do not create new tests unless explicitly requested.

Run existing tests when relevant to verify changes.

If you believe a new test should be added, explain:
- what should be tested,
- why the test is valuable, and
- what behavior it would verify.

Wait for confirmation before implementing the test.

# Coding Style

Prefer simplicity over complexity.

Implement the simplest solution that accomplishes the requirement without unnecessarily sacrificing security, correctness, maintainability, or performance.

Avoid premature abstractions and unnecessary layers. Do not create interfaces, helpers, packages, or abstractions unless they solve an immediate problem or meaningfully improve the code.

Keep comments to a minimum. Prefer clear naming and straightforward code over comments explaining complicated implementations. Add comments when they explain reasoning, constraints, or behavior that is not obvious from the code itself.

Follow standard Go conventions and run `gofmt` on modified Go files.

# Database

Use Goose migrations for database schema changes.

Define application SQL queries for `sqlc` and use the generated database access code. Avoid introducing separate database access patterns unless there is a specific reason to do so.

Do not manually modify generated `sqlc` files.

Keep migrations focused and avoid unrelated schema changes.

# Security

Although the application is intended for local-only use, follow normal secure coding practices.

Treat user-provided and imported data as untrusted. Use appropriate escaping, validation, parameterized SQL, and safe file handling where applicable.

Do not weaken security practices solely because the application runs locally.

# Scope Control

Do not refactor unrelated code while implementing a feature or fix.

If you notice an unrelated issue or potential improvement, mention it rather than changing it automatically.

Before making a significant architectural change, introducing a major dependency, or substantially expanding the scope of a task, explain the proposed change and reasoning first.