package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// Client handles interaction with the Kubelet Checkpoint API
type Client struct {
	KubeClient kubernetes.Interface
	Timeout    time.Duration
}

// NewClient creates a new Checkpoint Client
func NewClient(kubeClient kubernetes.Interface) *Client {
	return &Client{
		KubeClient: kubeClient,
		Timeout:    60 * time.Second, // Default timeout for large memory dumps
	}
}

// TriggerCheckpoint triggers a checkpoint on the node via the API Server Proxy
// POST /api/v1/nodes/{node}/proxy/checkpoint/{namespace}/{pod}/{container}
func (c *Client) TriggerCheckpoint(ctx context.Context, pod *corev1.Pod, containerName string) (string, error) {
	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		return "", fmt.Errorf("pod is not assigned to a node")
	}

	// Path: checkpoint/namespace/pod/container
	path := fmt.Sprintf("checkpoint/%s/%s/%s", pod.Namespace, pod.Name, containerName)

	// We use the RESTClient directly to hit the Node Proxy
	// This authenticates as the Controller ServiceAccount
	result := c.KubeClient.CoreV1().RESTClient().Post().
		Resource("nodes").
		Name(nodeName).
		SubResource("proxy").
		Suffix(path).
		Timeout(c.Timeout).
		Do(ctx)

	if result.Error() != nil {
		// Handle specific errors
		if result.Error().Error() == "404 Not Found" {
			return "", fmt.Errorf("checkpoint feature not enabled on kubelet or container runtime")
		}
		return "", result.Error()
	}

	// Parse Response
	// Expected JSON: {"items":["/var/lib/kubelet/checkpoints/checkpoint-...tar"]}
	raw, err := result.Raw()
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	var resp struct {
		Items []string `json:"items"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("failed to parse checkpoint response: %v", err)
	}

	if len(resp.Items) == 0 {
		return "", fmt.Errorf("checkpoint created but no file path returned")
	}

	return resp.Items[0], nil
}
