# Contributing to Kube Forensics Controller

Thank you for your interest in contributing to the Kube Forensics Controller! We welcome contributions from the community to help make Kubernetes debugging safer and more efficient.

## How to Contribute

### 1. Reporting Bugs
If you find a bug, please create a GitHub Issue with the following details:
- **Description**: What happened?
- **Reproduction Steps**: How can we reproduce it?
- **Expected Behavior**: What should have happened?
- **Logs/Screenshots**: Any relevant output from the controller or `kubectl forensic` plugin.
- **Environment**: Kubernetes version, Controller version, Cloud provider (if applicable).

### 2. Suggesting Enhancements
Have an idea? Open an Issue tagged with `enhancement`. Describe the problem you are solving and your proposed solution.

### 3. Pull Requests
1.  **Fork** the repository.
2.  **Clone** your fork locally.
3.  **Create a branch** for your feature or fix (`git checkout -b feature/amazing-feature`).
4.  **Make your changes**. Ensure code follows the existing style.
5.  **Run Tests**:
    ```bash
    make test
    ```
6.  **Verify Build**:
    ```bash
    make build
    make plugin
    ```
7.  **Commit** your changes with clear messages.
8.  **Push** to your fork.
9.  **Open a Pull Request** to the `main` branch.

## Development Setup

### Prerequisites
- Go 1.25+
- Docker
- Kind (Kubernetes in Docker) or a local cluster
- `kubectl`

### Local Development Loop
To run the controller locally against your current kubecontext:
```bash
make run
```

To build and load the image into Kind:
```bash
make docker-build
make kind-load
```

## Code Standards
- Run `go fmt ./...` and `go vet ./...` before committing.
- Ensure all new features have accompanying unit tests.
- Update `README.md` if you change flags or behavior.

## License
By contributing, you agree that your contributions will be licensed under the MIT License defined in the `LICENSE` file.
