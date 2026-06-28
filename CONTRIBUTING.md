# Contributing to the Self-Healing Platform

We welcome contributions from SREs, Platform Engineers, and Cloud-Native Developers! Please follow these guidelines to keep the codebase clean, stable, and highly performant.

---

## 1. Code of Conduct
By contributing to this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## 2. Getting Started
1. Fork the repository on GitHub.
2. Clone your fork locally:
   ```bash
   git clone https://github.com/your-username/Summer_Project.git
   ```
3. Set up your local environment:
   - Go 1.21+
   - Docker Desktop
   - minikube or kind
   - Helm 3.x

---

## 3. Development Workflow
1. Create a descriptive branch:
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b bugfix/issue-description
   ```
2. Write clean, idiomatic Go code.
3. Make sure code matches formatting standards:
   ```bash
   go fmt ./...
   ```
4. Verify your changes do not introduce linting warnings:
   ```bash
   go vet ./...
   ```
5. Ensure all unit tests pass successfully:
   ```bash
   go test ./...
   ```

---

## 4. Pull Request Guidelines
- Keep pull requests focused on a single responsibility.
- Link your PR to any relevant GitHub Issues.
- Provide a summary of changes, reproduction steps (for bug fixes), or performance metrics (for optimizations).
- A maintainer will review your pull request within 2-3 business days.
