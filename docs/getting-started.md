# Getting Started (Local Development)

This guide helps you set up the Kube Forensics Controller locally for testing and development using [Kind](https://kind.sigs.k8s.io/).

## Prerequisites
- **Go 1.25+**
- **Docker**
- **Kind**
- **kubectl**

## Step-by-Step Guide

### 1. Create a Kind Cluster
If you don't have a cluster running:
```bash
kind create cluster
```

### 2. Build the Docker Image
Build the controller manager image locally:
```bash
make docker-build
```

### 3. Load Image into Kind
Kind nodes cannot pull local docker images by default. You must load it:
```bash
make kind-load IMG=controller:v0.2.2
```

### 4. Deploy the Controller
Apply the RBAC and Deployment manifests:
```bash
make deploy
```

### 5. Verify Deployment
Check if the controller is running in the default namespace:
```bash
kubectl get pods -l control-plane=controller-manager
```

### 6. Run a Test Crash
Deploy a sample "crashing" pod to verify the controller reacts:
```bash
kubectl apply -f example/crashing-pod.yaml
```

Wait a few seconds, then check the `debug-forensics` namespace:
```bash
kubectl get pods -n debug-forensics
```
You should see a `crash-app-forensic-...` pod running.

## Cleaning Up
To remove the controller:
```bash
make undeploy
```
To delete the cluster:
```bash
kind delete cluster
```
