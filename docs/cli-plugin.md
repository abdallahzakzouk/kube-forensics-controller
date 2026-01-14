# Kubectl Forensic Plugin

A CLI tool to simplify interacting with forensic pods.

## Installation

```bash
make plugin
sudo cp bin/kubectl-forensic /usr/local/bin/kubectl-forensic
```

## Commands

### `list`
Lists all available forensic pods in the target namespace.
```bash
kubectl forensic list
```

### `access <pod-name>`
Automatically finds the injected toolkit shell (`/usr/local/bin/toolkit/sh`) and execs into the pod.
It also displays the **Original Command** as a hint, so you know how to restart the application manually.

```bash
kubectl forensic access my-app-forensic-xyz
# Output:
# 💡 Suggested Start Command: ./run.sh
# / #
```

### `logs <pod-name>`
Prints the original crash logs captured by the controller.
```bash
kubectl forensic logs my-app-forensic-xyz
```

### `export <pod-name>`
Downloads the logs and performs a **Chain of Custody Integrity Check**.
```bash
kubectl forensic export my-app-forensic-xyz > crash.log
# Output (stderr):
# Integrity Check Passed: a1b2c3...
```

### `cleanup`
Deletes all forensic pods in the namespace.
```bash
kubectl forensic cleanup
```

## Autocompletion
To enable tab completion:

**Zsh:**
```bash
kubectl forensic completion zsh > ~/.kubectl_forensic_completion.zsh
echo "source ~/.kubectl_forensic_completion.zsh" >> ~/.zshrc
echo "compdef _kubectl_forensic kubectl-forensic" >> ~/.zshrc
```

**Bash:**
```bash
kubectl forensic completion bash > ~/.kubectl_forensic_completion.bash
echo "source ~/.kubectl_forensic_completion.bash" >> ~/.bashrc
```
