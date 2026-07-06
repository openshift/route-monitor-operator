# Contributing to Route Monitor Operator

Thank you for your interest in contributing to the Route Monitor Operator project.

## Quick Start

1. **Setup**: Install Go 1.25.9+, operator-sdk v3 (kubebuilder v3)
2. **Install hooks**: `prek install`
3. **Build**: `make go-build`
4. **Test**: `make test`
5. **Lint**: `make lint`

See [DEVELOPMENT.md](./DEVELOPMENT.md) for detailed setup instructions.

## Before Submitting a PR

All contributions must pass:

1. **Formatting & linting**: `prek run --all`
2. **Unit tests**: `make test`
3. **Build verification**: `make go-build`
4. **Security scan**: Automatic via prek (gitleaks)

## Development Workflow

### Human Contributors

```bash
# Create a feature branch
git checkout -b feature/my-change

# Make changes, following existing code patterns
# Add/update tests for your changes

# Run validation locally
prek run --all
make test

# Commit with descriptive message
git commit -m "feat: add support for X"

# Push and create PR
git push origin feature/my-change
```

### AI-Assisted Development

When using AI coding agents (Claude Code, GitHub Copilot, Cursor, etc.):

**Agents MUST:**
- Run `prek run` on changed files before committing
- Execute relevant tests after code changes: `make test`
- Preserve existing code style and patterns
- Avoid editing generated files (`**/zz_generated.*.go`, `go.sum` without `go.mod`)
- Never bypass hooks with `--no-verify`
- Never commit secrets, tokens, or credentials
- Reuse existing utilities and abstractions
- Make incremental, focused changes

**Validation expectations:**
1. Format check: `go fmt ./...`
2. Lint: `make lint` (gosec only per `.golangci.yaml`)
3. Type safety: Verified by `go build ./...` in prek
4. Tests: `make test` for affected packages
5. Secret scan: Automatic via prek gitleaks hook

**Required checks before PR:**
- [ ] All prek hooks pass
- [ ] Unit tests pass for modified packages
- [ ] No new gosec warnings introduced
- [ ] No secrets or credentials in diff
- [ ] Generated code regenerated if API types changed: `make generate`

## Code Style

Follow existing patterns:
- Standard Go formatting (`gofmt`)
- golangci-lint rules in `.golangci.yaml` (gosec enabled)
- Ginkgo/Gomega for tests (v1 in unit tests, v2 in E2E)
- Standard Go testing patterns alongside Ginkgo

## Testing Requirements

- **Unit tests required** for all new functionality
- Use Ginkgo BDD style: `Describe`, `Context`, `It`
- Generated mocks live in `pkg/util/test/generated/mocks/` (via `//go:generate mockgen`); tests may use real, fake, or generated mock implementations
- E2E tests live in `test/e2e/` and require a deployed operator

See [TESTING.md](./TESTING.md) for testing guidelines.

## Regenerating Code

After modifying API types:

```bash
# Regenerate deepcopy, OpenAPI, CRD manifests
make generate
```

## Security

**Never commit:**
- API keys, tokens, passwords
- AWS credentials, kubeconfig files
- Private keys, certificates
- `.env` files with secrets
- Debug statements printing sensitive data

The prek gitleaks hook will block commits containing secrets.

**High-risk changes** (requiring extra review):
- Authentication/authorization logic (`pkg/rhobs/`, OIDC handling)
- RBAC manifests with wildcard permissions
- CI/CD pipeline modifications
- Dockerfile changes

## Commit Message Format

Use conventional commits style:

```text
<type>: <short summary>

<optional body>

<optional footer>
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`

Examples:
- `feat: add support for HostedControlPlane domain resolution`
- `fix: correct RBAC permissions for ServiceMonitor`
- `test: add unit tests for ClusterUrlMonitor reconciler`

## Pull Request Process

1. **Title**: Clear, descriptive summary
2. **Description**: Explain what changed and why
3. **Testing**: Describe how you tested the changes
4. **CI**: All Tekton pipeline checks must pass
5. **Review**: Address review feedback promptly

## Questions?

- Review existing issues and PRs for context
- Ask in PR comments for clarification
- Check `AGENTS.md` for agent routing guidance

## License

All contributions are licensed under Apache 2.0. See [LICENSE](./LICENSE).
