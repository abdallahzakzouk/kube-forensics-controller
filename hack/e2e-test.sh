#!/bin/bash
set -e

# E2E Test Script for Kube Forensics Controller

CLUSTER_NAME="forensics-e2e"
IMAGE_NAME="controller:e2e"
NAMESPACE="debug-forensics"

echo "🚀 Starting E2E Test..."

# 1. Create Kind Cluster
if ! kind get clusters | grep -q "$CLUSTER_NAME"; then
    echo "Creating Kind cluster..."
    kind create cluster --name "$CLUSTER_NAME"
else
    echo "Cluster $CLUSTER_NAME already exists."
fi

# 2. Build and Load Image
echo "📦 Building Docker image..."
docker build -t "$IMAGE_NAME" .
echo "🚚 Loading image into Kind..."
kind load docker-image "$IMAGE_NAME" --name "$CLUSTER_NAME"

# 3. Deploy Controller
echo "🛠️ Deploying Controller..."
# We use kustomize or direct apply. The deploy/manager.yaml uses 'controller:latest', we need 'controller:e2e'
# Simple hack: replace image in a temp file
sed "s|image: controller:.*|image: $IMAGE_NAME|g" deploy/manager.yaml > deploy/manager-e2e.yaml
sed -i.bak "s|imagePullPolicy: IfNotPresent|imagePullPolicy: Never|g" deploy/manager-e2e.yaml

kubectl --context "kind-$CLUSTER_NAME" apply -f config/rbac/rbac.yaml
kubectl --context "kind-$CLUSTER_NAME" apply -f deploy/manager-e2e.yaml

# Wait for controller to be ready
echo "⏳ Waiting for controller to be ready..."
kubectl --context "kind-$CLUSTER_NAME" wait --for=condition=available --timeout=60s deployment/kube-forensics-controller -n default

# 4. Deploy Crashing Pod
echo "💥 Deploying Crashing Pod..."
kubectl --context "kind-$CLUSTER_NAME" apply -f example/crashing-pod.yaml

# 5. Verification
echo "🕵️  Verifying Forensic Pod creation..."
# Wait for the crash to happen (pod fails immediately)
sleep 10 

# Check for forensic pod in target namespace
FORENSIC_POD=$(kubectl --context "kind-$CLUSTER_NAME" get pods -n "$NAMESPACE" -l forensic-source-pod=crash-app -o jsonpath="{.items[0].metadata.name}")

if [ -z "$FORENSIC_POD" ]; then
    echo "❌ Test Failed: No forensic pod found in $NAMESPACE"
    kubectl --context "kind-$CLUSTER_NAME" get pods -A
    kubectl --context "kind-$CLUSTER_NAME" logs -n default -l control-plane=controller-manager
    exit 1
fi

echo "✅ Forensic Pod Found: $FORENSIC_POD"

# Check annotations
echo "🔍 Checking Annotations..."
EXIT_CODE=$(kubectl --context "kind-$CLUSTER_NAME" get pod -n "$NAMESPACE" "$FORENSIC_POD" -o jsonpath="{.metadata.annotations.forensic\.io/exit-code}")

if [ "$EXIT_CODE" == "1" ]; then
    echo "✅ Exit Code Verified: $EXIT_CODE"
else
    echo "❌ Test Failed: Expected Exit Code 1, got $EXIT_CODE"
    exit 1
fi

echo "🎉 E2E Test Passed!"

# Cleanup (Optional)
# kind delete cluster --name "$CLUSTER_NAME"
