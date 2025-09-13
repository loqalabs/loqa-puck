# Development Guidance

See [/loqalabs/CLAUDE.md](../CLAUDE.md) for complete development workflow guidance.

## Service Context

**loqa-relay** - Audio capture client and future embedded firmware foundation (Go)

- **Role**: Client-side audio capture and streaming to hub via gRPC
- **Quality Gates**: `make quality-check` (includes go fmt, go vet, golangci-lint, tests)
- **Development**: `go run ./test-go/cmd` (test client), `make build` (production build)
- **Testing**: `./tools/run-test-relay.sh` for voice pipeline testing

All workflow rules and development guidance are provided automatically by the MCP server based on repository detection.