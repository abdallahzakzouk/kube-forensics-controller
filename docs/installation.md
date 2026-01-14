# Installation Guide (Production)

This guide covers installing the Kube Forensics Controller on standard Kubernetes clusters (EKS, GKE, AKS, Bare Metal).

## Prerequisites
- **Kubernetes Cluster** (v1.25+)
- **Helm** (v3+)
- **kubectl** configured with cluster admin access.

## Option 1: Helm Chart (Recommended)

We provide a Helm chart for easy installation and configuration.

### 1. Prepare the Image
Ensure the controller image is available in a registry your cluster can pull from (e.g., GHCR, Docker Hub, ECR).
*   If building from source:
    ```bash
    docker build -t my-registry/kube-forensics-controller:v0.2.2 .
    docker push my-registry/kube-forensics-controller:v0.2.2
    ```

### 2. Install the Chart
Clone the repository and install from the `charts/` directory:

```bash
git clone https://github.com/abdallahzakzouk/kube-forensics-controller.git
cd kube-forensics-controller

helm install forensics ./charts/kube-forensics-controller \
  --set image.repository=my-registry/kube-forensics-controller \
  --set image.tag=v0.2.2 \
  --set config.targetNamespace=debug-forensics
```

### 3. Verify
```bash
kubectl get pods -l app.kubernetes.io/name=kube-forensics-controller
```

## Option 2: Plain Manifests

If you prefer raw YAML manifests:

1.  **Generate Manifests:**
    ```bash
    make manifests
    kustomize build config/default > install.yaml
    ```
2.  **Edit Image:** Update the image field in `install.yaml` to point to your registry.
3.  **Apply:**
    ```bash
    kubectl apply -f install.yaml
    ```

## Post-Installation Verification

To verify the installation is working:
1.  Check the logs of the controller pod.
2.  Ensure the CRDs (VolumeSnapshots if used) are present.
3.  Deploy a sample app that crashes.
