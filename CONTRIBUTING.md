# Contributing to SkyHook

Thank you for your interest in contributing to SkyHook! We welcome bug reports, feature requests, documentation improvements, and code contributions.

## Development Workflow

1. Fork the repository and create your feature branch:
   ```bash
   git checkout -b feature/amazing-feature
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Run unit tests with race detection:
   ```bash
   go test -v -race ./...
   ```

4. Verify Docker build locally:
   ```bash
   docker build -t skyhook:test -f deploy/Dockerfile .
   ```

5. Commit your changes and push to your fork:
   ```bash
   git commit -m "feat: add amazing feature"
   git push origin feature/amazing-feature
   ```

6. Open a Pull Request against `main`.

## License

By contributing to SkyHook, you agree that your contributions will be licensed under the [GNU Affero General Public License v3.0](LICENSE).
