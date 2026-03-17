# AGENTS.md

Guidelines for AI agents working on this codebase.

## Commands

Use `just` commands for all checks and builds from inside `nix develop`:

```bash
nix develop       # Enter the pinned project toolchain
just check        # Run formatting check, linting, and tests
just security     # Run vulnerability and secret scanning
just pre-pr       # Full pre-PR quality gate (check + security)
```

Canonical verification path: `nix develop -c just check`.

**Important:** Always run `just check` after making code changes to verify formatting, linting, and tests pass.

## Documentation

Every package should contain a `doc.go` file with package-level documentation explaining the package's purpose and contents.

## Testing

All changed or new features must be accompanied by tests. Preferred approach:

- Use table-driven tests with the standard library
- Use `cmp.Diff` from `github.com/google/go-cmp/cmp` for comparisons
- Production code must not contain test hook abstractions or global hooks
