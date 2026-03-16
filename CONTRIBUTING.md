# Contributing

Thanks for contributing.

## Development Workflow

1. Install tooling:

```bash
just tools
```

2. Start local dependencies when needed:

```bash
just env-init
just compose-up
```

3. Make your changes.
4. Regenerate code when specs, SQL, or schema change:

```bash
just sqlc-generate
just ogen-generate
```

5. Run the full quality gate before opening a PR:

```bash
just check
just security
```

## Coding Expectations

- Prefer small, explicit packages with clear boundaries.
- Keep code idiomatic and standard-library first.
- Add or update tests for every behavior change.
- Use table-driven tests where they fit.
- Use `cmp.Diff` for readable test failures.
- Keep package documentation in `doc.go` files.
- Do not edit generated files by hand.

## Database Changes

- Add schema changes in `db/migrations`.
- Keep SQL queries in `db/queries`.
- Regenerate `sqlc` output after schema or query changes.
- If startup behavior depends on the schema change, cover it with tests.

## API Changes

- Update the OpenAPI spec in `api/` first.
- Regenerate `ogen` code.
- Keep error behavior aligned with the shared error schema.
- Preserve request ID propagation across logs, errors, and audit records.

## Pull Requests

- Keep PRs focused.
- Explain the reason for the change.
- Mention any migration, config, or rollout concerns.
- Include verification steps when behavior is not obvious from tests alone.

## Commit Messages

- Use Conventional Commits because release tags and GitHub Release notes are generated automatically from commit history.
- Common prefixes are `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `ci:`, and `chore:`.
- Mark breaking changes with `!` in the subject, such as `feat!: remove legacy auth`, or add a `BREAKING CHANGE:` footer.
- Keep the subject line focused on the user-visible change so release notes stay readable.
