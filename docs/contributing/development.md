# Development Guidelines

## Getting Started

### Prerequisites

Before you begin development, ensure you have the following prerequisites:

- A working installation of Go (version 1.23 or later).
- Access to a Kubernetes cluster for testing.
- Familiarity with Git and GitHub workflows.

### Cloning the Repository

To contribute to the project, start by cloning the repository:

```bash
git clone https://github.com/ROCm/k8s-device-plugin.git
cd k8s-device-plugin
```

### Branching Strategy

When working on a new feature or bug fix, create a new branch from the `main` branch:

```bash
git checkout -b feature/my-new-feature
```

Make sure to name your branch descriptively to reflect the changes you are making.

## Development Workflow

1. **Make Changes**: Implement your changes in the codebase.
2. **Testing**: Ensure that your changes are covered by tests. Run existing tests and add new ones as necessary.
3. **Commit Changes**: Commit your changes with a clear and concise commit message:

   ```bash
   git commit -m "Add feature X to improve Y"
   ```

4. **Push Changes**: Push your branch to the remote repository:

   ```bash
   git push origin feature/my-new-feature
   ```

5. **Create a Pull Request**: Navigate to the GitHub repository and create a pull request. Provide a detailed description of your changes and why they are necessary.

## Code Review Process

All contributions will undergo a code review process. Reviewers will assess the quality of the code, adherence to coding standards, and the completeness of tests. Be open to feedback and ready to make adjustments as needed.
