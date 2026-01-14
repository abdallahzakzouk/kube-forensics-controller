package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
)

const (
	LabelSourcePod = "forensic-source-pod"

	LabelSourcePodUID = "forensic-source-pod-uid"

	LabelForensicTime = "forensic-time"

	LabelForensicTTL = "forensic.io/ttl"

	LabelCrashSignature = "forensic.io/crash-signature"

	AnnotationNoSecretClone = "forensic.io/no-secret-clone"

	AnnotationForensicHold = "forensic.io/hold"

	LabelLogS3URL = "forensic.io/log-s3-url"

	ForensicTimeFormat = "2006-01-02T15-04-05Z"

	NetworkPolicyName = "deny-all-egress"

	LogConfigMapKey = "crash.log"
)

type ForensicsConfig struct {
	TargetNamespace string

	ForensicTTL time.Duration

	MaxLogSizeBytes int64

	IgnoreNamespaces []string

	WatchNamespaces []string

	EnableSecretCloning bool

	EnableCheckpointing bool

	RateLimitWindow time.Duration
}

// PodReconciler reconciles a Pod object

type PodReconciler struct {
	client.Client

	Scheme *runtime.Scheme

	KubeClient kubernetes.Interface

	Config ForensicsConfig

	Recorder record.EventRecorder

	Exporter Exporter
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get;update;patch

//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get

//+kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create

//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch

//+kubebuilder:rbac:groups="",resources=nodes/proxy,verbs=get;create

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := log.FromContext(ctx)

	// 0. Check Watch Namespaces (Allow-list)

	if len(r.Config.WatchNamespaces) > 0 {

		allowed := false

		for _, ns := range r.Config.WatchNamespaces {

			if req.Namespace == ns {

				allowed = true

				break

			}

		}

		if !allowed {

			return ctrl.Result{}, nil

		}

	}

	// 0.1 Check Ignore List

	for _, ns := range r.Config.IgnoreNamespaces {

		if req.Namespace == ns {

			return ctrl.Result{}, nil

		}

	}

	if req.Namespace == r.Config.TargetNamespace {

		return ctrl.Result{}, nil

	}

	// 1. Fetch the Pod

	var pod corev1.Pod

	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {

		return ctrl.Result{}, client.IgnoreNotFound(err)

	}

	// 2. Ignore if deleted

	if !pod.DeletionTimestamp.IsZero() {

		return ctrl.Result{}, nil

	}

	// 3. Check Crash Criteria and identify crashed container

	isCrash := false

	crashedContainerName := ""

	var exitCode int32 = 0

	// Helper to check a single status

	checkStatus := func(name string, state corev1.ContainerState, lastState corev1.ContainerState) bool {

		// Check current state

		if state.Terminated != nil {

			reason := state.Terminated.Reason

			if reason == "Error" || reason == "OOMKilled" || state.Terminated.ExitCode != 0 {

				crashedContainerName = name

				exitCode = state.Terminated.ExitCode

				return true

			}

		}

		// Check last state (for CrashLoopBackOff)

		if lastState.Terminated != nil {

			reason := lastState.Terminated.Reason

			if reason == "Error" || reason == "OOMKilled" || lastState.Terminated.ExitCode != 0 {

				crashedContainerName = name

				exitCode = lastState.Terminated.ExitCode

				return true

			}

		}

		return false

	}

	allStatuses := append(pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses...)

	for _, status := range allStatuses {

		if checkStatus(status.Name, status.State, status.LastTerminationState) {

			isCrash = true

			break

		}

	}

	// Fallback for PodFailed without specific container info

	if !isCrash && pod.Status.Phase == corev1.PodFailed {

		isCrash = true

		if len(pod.Spec.Containers) > 0 {

			crashedContainerName = pod.Spec.Containers[0].Name

			// We don't have an exit code easily here, default to 1

			exitCode = 1

		}

	}

	if !isCrash {

		return ctrl.Result{}, nil

	}

	logger.Info("Detected crashed pod", "pod", req.NamespacedName, "phase", pod.Status.Phase)

	// Metric: Crash Detected

	ForensicCrashesTotal.WithLabelValues(pod.Namespace, "CrashDetected").Inc()

	// 4. Deduplication & Rate Limiting

	// Calculate Crash Signature

	signature := r.getCrashSignature(&pod, crashedContainerName, exitCode)

	var forensicPods corev1.PodList

	if err := r.List(ctx, &forensicPods, client.InNamespace(r.Config.TargetNamespace), client.MatchingLabels{LabelCrashSignature: signature}); err != nil {

		logger.Error(err, "Failed to list forensic pods for deduplication")

		ForensicPodCreationErrorsTotal.WithLabelValues(pod.Namespace, "Deduplication").Inc()

		return ctrl.Result{}, err

	}

	// Check if any existing forensic pod is within the rate limit window

	now := time.Now()

	for _, fp := range forensicPods.Items {

		// Use CreationTimestamp as reference

		age := now.Sub(fp.CreationTimestamp.Time)

		if age < r.Config.RateLimitWindow {

			logger.Info("Skipping forensic creation (rate limited)", "original_pod", req.NamespacedName, "signature", signature, "age", age)

			return ctrl.Result{}, nil

		}

	}

	// Emit Event: Crash Detected

	r.Recorder.Eventf(&pod, corev1.EventTypeWarning, "ForensicAnalysisStarted", "Crash detected in container %s (ExitCode: %d). Creating forensic pod.", crashedContainerName, exitCode)

	// 5. Ensure Namespace Exists

	if err := r.ensureNamespace(ctx); err != nil {

		logger.Error(err, "Failed to ensure target namespace")

		ForensicPodCreationErrorsTotal.WithLabelValues(pod.Namespace, "EnsureNamespace").Inc()

		return ctrl.Result{}, err

	}

	// 6. Ensure Network Isolation

	if err := r.ensureNetworkPolicy(ctx); err != nil {

		logger.Error(err, "Failed to ensure network policy")

		ForensicPodCreationErrorsTotal.WithLabelValues(pod.Namespace, "EnsureNetworkPolicy").Inc()

		return ctrl.Result{}, err

	}

	// 7. Fetch Logs (Feature 1)

	logs, err := r.getPodLogs(ctx, &pod, crashedContainerName)

	if err != nil {

		logger.Error(err, "Failed to fetch logs (continuing without logs)")

		// Soft error, not incrementing creation failure metric

		logs = fmt.Sprintf("Error fetching logs: %v", err)

	}

	// 8. Upload Logs to S3 (Feature: Ops / Chain of Custody)

	var s3URL string

	if logs != "" {

		timestamp := time.Now().UTC().Format("2006/01/02/150405")

		key := fmt.Sprintf("%s/%s/%s/crash.log", pod.Namespace, pod.Name, timestamp)

		url, err := r.Exporter.Upload(ctx, key, []byte(logs))

		if err != nil {

			logger.Error(err, "Failed to upload logs to S3")

			r.Recorder.Eventf(&pod, corev1.EventTypeWarning, "ForensicExportFailed", "Failed to upload logs to S3: %v", err)

		} else if url != "" {

			s3URL = url

			r.Recorder.Eventf(&pod, corev1.EventTypeNormal, "ForensicExportSuccess", "Uploaded logs to %s", url)

		}

	}

	// 9. Clone ConfigMaps and Secrets

	resourceMap, err := r.cloneDependencies(ctx, &pod)

	if err != nil {

		logger.Error(err, "Failed to clone dependencies")

		ForensicPodCreationErrorsTotal.WithLabelValues(pod.Namespace, "CloneDependencies").Inc()

		return ctrl.Result{}, err

	}

	// 10. Create Log ConfigMap

	logCMName, err := r.createLogConfigMap(ctx, &pod, logs)

	if err != nil {

		logger.Error(err, "Failed to create log configmap")

		ForensicPodCreationErrorsTotal.WithLabelValues(pod.Namespace, "CreateLogCM").Inc()

		return ctrl.Result{}, err

	}

	// Calculate Log Hash (Chain of Custody)

	logHash := sha256.Sum256([]byte(logs))

	logHashStr := hex.EncodeToString(logHash[:])

	// 11. Snapshot PVCs (Feature: Robustness)

	snapshotMap, err := r.snapshotPVCs(ctx, &pod)

	if err != nil {

		// Log error but continue (soft failure for snapshots)

		logger.Error(err, "Failed to snapshot PVCs")

		r.Recorder.Eventf(&pod, corev1.EventTypeWarning, "ForensicSnapshotFailed", "Failed to snapshot PVCs: %v", err)

	} else if len(snapshotMap) > 0 {

		r.Recorder.Eventf(&pod, corev1.EventTypeNormal, "ForensicSnapshotsCreated", "Created volume snapshots for %d PVCs", len(snapshotMap))

	}

	// 12. Trigger Container Checkpoint (Feature: Excellence)

	var checkpointLocation string

	if r.Config.EnableCheckpointing && crashedContainerName != "" {

		loc, err := r.triggerCheckpoint(ctx, &pod, crashedContainerName)

		if err != nil {

			logger.Error(err, "Failed to trigger checkpoint")

			r.Recorder.Eventf(&pod, corev1.EventTypeWarning, "ForensicCheckpointFailed", "Failed to trigger checkpoint: %v", err)

		} else {

			checkpointLocation = loc

			r.Recorder.Eventf(&pod, corev1.EventTypeNormal, "ForensicCheckpointCreated", "Container checkpoint created at %s", loc)

		}

	}

	// 13. Create Forensic Pod

	if err := r.createForensicPod(ctx, &pod, resourceMap, logCMName, signature, crashedContainerName, exitCode, logHashStr, snapshotMap, checkpointLocation, s3URL); err != nil {

		logger.Error(err, "Failed to create forensic pod")

		ForensicPodCreationErrorsTotal.WithLabelValues(pod.Namespace, "CreateForensicPod").Inc()

		return ctrl.Result{}, err

	}

	logger.Info("Successfully created forensic pod", "original_pod", req.NamespacedName, "log_hash", logHashStr)

	r.Recorder.Eventf(&pod, corev1.EventTypeNormal, "ForensicPodCreated", "Created forensic pod %s (LogHash: %s)", r.Config.TargetNamespace, logHashStr)

	ForensicPodsCreatedTotal.WithLabelValues(pod.Namespace).Inc()

	return ctrl.Result{}, nil

}

func (r *PodReconciler) triggerCheckpoint(ctx context.Context, pod *corev1.Pod, containerName string) (string, error) {
	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		return "", fmt.Errorf("pod is not assigned to a node")
	}

	// Construct Checkpoint API Path: /checkpoint/namespace/pod/container
	path := fmt.Sprintf("checkpoint/%s/%s/%s", pod.Namespace, pod.Name, containerName)

	// Call Kubelet API via Proxy
	result := r.KubeClient.CoreV1().RESTClient().Post().
		Resource("nodes").
		Name(nodeName).
		SubResource("proxy").
		Suffix(path).
		Do(ctx)

	if result.Error() != nil {
		return "", result.Error()
	}

	// Read Response (should contain the path on the node)
	// Example: {"items":["/var/lib/kubelet/checkpoints/checkpoint-<pod>-<container>-<timestamp>.tar"]}
	// For now, we return a success message pointing to the node
	// In a real implementation, we would parse the JSON response.
	// But let's assume success means it's on the node.

	return fmt.Sprintf("node://%s/var/lib/kubelet/checkpoints/", nodeName), nil
}

func (r *PodReconciler) snapshotPVCs(ctx context.Context, pod *corev1.Pod) (map[string]string, error) {
	snapshotMap := make(map[string]string)

	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			pvcName := vol.PersistentVolumeClaim.ClaimName

			// Create Snapshot
			snap := &snapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("forensic-%s-%s-", pod.Name, vol.Name),
					Namespace:    pod.Namespace, // Snapshots must be in PVC namespace
					Labels: map[string]string{
						LabelSourcePodUID: string(pod.UID),
						LabelForensicTTL:  r.Config.ForensicTTL.String(),
					},
				},
				Spec: snapshotv1.VolumeSnapshotSpec{
					Source: snapshotv1.VolumeSnapshotSource{
						PersistentVolumeClaimName: &pvcName,
					},
				},
			}

			// Try to find if one already exists for this crash?
			// Deduplication logic handles the pod creation, but we might want to check here too.
			// Relying on GenerateName implies unique snapshots per attempt.

			if err := r.Create(ctx, snap); err != nil {
				// If CRD not found (cluster doesn't support snapshots), return error
				return nil, err
			}
			snapshotMap[pvcName] = snap.Name
		}
	}
	return snapshotMap, nil
}

func (r *PodReconciler) getCrashSignature(pod *corev1.Pod, containerName string, exitCode int32) string {
	// Try to identify the "Workload" name
	workloadName := pod.GenerateName
	if len(pod.OwnerReferences) > 0 {
		workloadName = pod.OwnerReferences[0].Name
	}
	if workloadName == "" {
		workloadName = pod.Name // Fallback for standalone pods
	}

	// Input: Namespace + WorkloadName + ContainerName + ExitCode
	input := fmt.Sprintf("%s-%s-%s-%d", pod.Namespace, workloadName, containerName, exitCode)
	hash := sha256.Sum256([]byte(input))
	// Truncate to 63 chars to satisfy K8s label limit
	return hex.EncodeToString(hash[:])[:63]
}

func (r *PodReconciler) ensureNamespace(ctx context.Context) error {

	ns := &corev1.Namespace{

		ObjectMeta: metav1.ObjectMeta{

			Name: r.Config.TargetNamespace,
		},
	}

	err := r.Create(ctx, ns)

	if err != nil && !errors.IsAlreadyExists(err) {

		return err

	}

	return nil

}

func (r *PodReconciler) ensureNetworkPolicy(ctx context.Context) error {

	policy := &networkingv1.NetworkPolicy{

		ObjectMeta: metav1.ObjectMeta{

			Name: NetworkPolicyName,

			Namespace: r.Config.TargetNamespace,
		},

		Spec: networkingv1.NetworkPolicySpec{

			PodSelector: metav1.LabelSelector{}, // Select all pods in the namespace

			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},

			Egress: []networkingv1.NetworkPolicyEgressRule{}, // Deny all egress

		},
	}

	err := r.Create(ctx, policy)

	if err != nil && !errors.IsAlreadyExists(err) {

		return err

	}

	return nil

}

// getPodLogs fetches logs from the crashed container
func (r *PodReconciler) getPodLogs(ctx context.Context, pod *corev1.Pod, containerName string) (string, error) {

	if containerName == "" {

		return "", fmt.Errorf("no container name specified")

	}

	logOpts := &corev1.PodLogOptions{

		Container: containerName,
	}

	req := r.KubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)

	stream, err := req.Stream(ctx)

	if err != nil {

		return "", err

	}

	defer stream.Close()

	// Limit reader to configured size
	buf := make([]byte, r.Config.MaxLogSizeBytes)

	n, err := io.ReadFull(stream, buf)

	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {

		return "", err

	}

	logs := string(buf[:n])

	if int64(n) == r.Config.MaxLogSizeBytes {

		logs += fmt.Sprintf("\n... [TRUNCATED %d KB] ...", r.Config.MaxLogSizeBytes/1024)

	}

	return logs, nil

}

func (r *PodReconciler) createLogConfigMap(ctx context.Context, pod *corev1.Pod, logs string) (string, error) {

	name := fmt.Sprintf("%s-logs", pod.Name)

	cm := &corev1.ConfigMap{

		ObjectMeta: metav1.ObjectMeta{

			Name: name,

			Namespace: r.Config.TargetNamespace,

			Labels: map[string]string{

				LabelSourcePodUID: string(pod.UID),
			},
		},

		Data: map[string]string{

			LogConfigMapKey: logs,
		},
	}

	cm.GenerateName = fmt.Sprintf("%s-logs-", pod.Name)

	cm.Name = ""

	if err := r.Create(ctx, cm); err != nil {

		return "", err

	}

	return cm.Name, nil

}

func (r *PodReconciler) cloneDependencies(ctx context.Context, pod *corev1.Pod) (map[string]string, error) {

	logger := log.FromContext(ctx)

	resourceMap := make(map[string]string)

	// Check Secret Cloning Opt-Out

	secretsDisabled := !r.Config.EnableSecretCloning

	if pod.Annotations[AnnotationNoSecretClone] == "true" {

		secretsDisabled = true

	}

	handleConfigMap := func(name string) error {

		key := fmt.Sprintf("cm/%s", name)

		if _, exists := resourceMap[key]; exists {

			return nil

		}

		var src corev1.ConfigMap

		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: pod.Namespace}, &src); err != nil {

			logger.Info("Warning: Referenced ConfigMap not found", "name", name, "namespace", pod.Namespace)

			return nil

		}

		newName := fmt.Sprintf("%s-%s", pod.Namespace, name)

		dst := &corev1.ConfigMap{

			ObjectMeta: metav1.ObjectMeta{

				Name: newName,

				Namespace: r.Config.TargetNamespace,

				Labels: src.Labels,
			},

			Data: src.Data,

			BinaryData: src.BinaryData,
		}

		if dst.Labels == nil {

			dst.Labels = make(map[string]string)

		}

		dst.Labels[LabelSourcePodUID] = string(pod.UID)

		if err := r.Create(ctx, dst); err != nil && !errors.IsAlreadyExists(err) {

			return err

		}

		resourceMap[key] = newName

		return nil

	}

	handleSecret := func(name string) error {

		key := fmt.Sprintf("secret/%s", name)

		if _, exists := resourceMap[key]; exists {

			return nil

		}

		var src corev1.Secret

		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: pod.Namespace}, &src); err != nil {

			logger.Info("Warning: Referenced Secret not found", "name", name, "namespace", pod.Namespace)

			return nil

		}

		newName := fmt.Sprintf("%s-%s", pod.Namespace, name)

		dst := &corev1.Secret{

			ObjectMeta: metav1.ObjectMeta{

				Name: newName,

				Namespace: r.Config.TargetNamespace,

				Labels: src.Labels,
			},

			Type: src.Type,

			Data: map[string][]byte{}, // Start empty

			StringData: map[string]string{},
		}

		if secretsDisabled {

			// Redaction Mode

			dst.StringData["WARNING"] = "Secret cloning is disabled for this pod. Values have been redacted."

			for k := range src.Data {

				dst.Data[k] = []byte("REDACTED")

			}

			for k := range src.StringData {

				dst.StringData[k] = "REDACTED"

			}

		} else {

			// Normal Cloning
			dst.Data = src.Data

			dst.StringData = src.StringData

		}

		if dst.Labels == nil {

			dst.Labels = make(map[string]string)

		}

		dst.Labels[LabelSourcePodUID] = string(pod.UID)

		if err := r.Create(ctx, dst); err != nil && !errors.IsAlreadyExists(err) {

			return err

		}

		resourceMap[key] = newName

		return nil

	}

	// 1. Scan Volumes
	for _, vol := range pod.Spec.Volumes {

		if vol.ConfigMap != nil {

			if err := handleConfigMap(vol.ConfigMap.Name); err != nil {

				return nil, err

			}

		}

		if vol.Secret != nil {

			if err := handleSecret(vol.Secret.SecretName); err != nil {

				return nil, err

			}

		}

		if vol.Projected != nil {

			for _, source := range vol.Projected.Sources {

				if source.ConfigMap != nil {

					if err := handleConfigMap(source.ConfigMap.Name); err != nil {

						return nil, err

					}

				}

				if source.Secret != nil {

					if err := handleSecret(source.Secret.Name); err != nil {

						return nil, err

					}

				}

			}

		}

	}

	// 2. Scan Containers for EnvFrom and Env

	scanContainers := func(containers []corev1.Container) error {

		for _, c := range containers {

			for _, envFrom := range c.EnvFrom {

				if envFrom.ConfigMapRef != nil {

					if err := handleConfigMap(envFrom.ConfigMapRef.Name); err != nil {

						return err

					}

				}

				if envFrom.SecretRef != nil {

					if err := handleSecret(envFrom.SecretRef.Name); err != nil {

						return err

					}

				}

			}

			for _, env := range c.Env {

				if env.ValueFrom != nil {

					if env.ValueFrom.ConfigMapKeyRef != nil {

						if err := handleConfigMap(env.ValueFrom.ConfigMapKeyRef.Name); err != nil {

							return err

						}

					}

					if env.ValueFrom.SecretKeyRef != nil {

						if err := handleSecret(env.ValueFrom.SecretKeyRef.Name); err != nil {

							return err

						}

					}

				}

			}

		}

		return nil

	}

	if err := scanContainers(pod.Spec.Containers); err != nil {

		return nil, err

	}

	if err := scanContainers(pod.Spec.InitContainers); err != nil {

		return nil, err

	}

	return resourceMap, nil

}

func (r *PodReconciler) createForensicPod(ctx context.Context, originalPod *corev1.Pod, resourceMap map[string]string, logCMName string, signature string, crashedContainerName string, exitCode int32, logHash string, snapshotMap map[string]string, checkpointLocation string, s3URL string) error {

	// Truncate original pod name for label

	sourcePodName := originalPod.Name

	if len(sourcePodName) > 63 {

		sourcePodName = sourcePodName[:63]

	}

	annotations := map[string]string{

		"forensic.io/exit-code": fmt.Sprintf("%d", exitCode),

		"forensic.io/log-sha256": logHash,
	}

	// Add Snapshot Info

	if len(snapshotMap) > 0 {

		var parts []string

		for pvc, snap := range snapshotMap {

			parts = append(parts, fmt.Sprintf("%s:%s", pvc, snap))

		}

		annotations["forensic.io/snapshots"] = strings.Join(parts, ",")

	}

	// Add Checkpoint Info

	if checkpointLocation != "" {

		annotations["forensic.io/checkpoint"] = checkpointLocation

	}

	// Add S3 Info

	if s3URL != "" {

		annotations[LabelLogS3URL] = s3URL

	}

	// Find original container to capture command/args

	for _, c := range originalPod.Spec.Containers {

		if c.Name == crashedContainerName {

			if len(c.Command) > 0 {

				annotations["forensic.io/original-command"] = strings.Join(c.Command, " ")

			}

			if len(c.Args) > 0 {

				annotations["forensic.io/original-args"] = strings.Join(c.Args, " ")

			}

			break

		}

	}

	// Also check init containers if not found

	if _, ok := annotations["forensic.io/original-command"]; !ok {

		for _, c := range originalPod.Spec.InitContainers {

			if c.Name == crashedContainerName {

				if len(c.Command) > 0 {

					annotations["forensic.io/original-command"] = strings.Join(c.Command, " ")

				}

				if len(c.Args) > 0 {

					annotations["forensic.io/original-args"] = strings.Join(c.Args, " ")

				}

				break

			}

		}

	}

	newPod := &corev1.Pod{

		ObjectMeta: metav1.ObjectMeta{

			GenerateName: fmt.Sprintf("%s-forensic-", originalPod.Name),

			Namespace: r.Config.TargetNamespace,

			Labels: map[string]string{

				LabelSourcePod: sourcePodName,

				LabelSourcePodUID: string(originalPod.UID),

				LabelCrashSignature: signature,

				LabelForensicTime: time.Now().UTC().Format(ForensicTimeFormat),

				LabelForensicTTL: r.Config.ForensicTTL.String(),
			},

			Annotations: annotations,
		},

		Spec: *originalPod.Spec.DeepCopy(),
	}

	// Clean up spec for new pod
	newPod.Spec.NodeName = "" // Let scheduler handle it
	newPod.Spec.RestartPolicy = corev1.RestartPolicyNever

	// Security Hardening: Disable ServiceAccount Token Mount
	// This prevents the forensic pod from using the source SA to attack the API
	falseVal := false
	newPod.Spec.AutomountServiceAccountToken = &falseVal
	newPod.Spec.ServiceAccountName = "" // Or keep empty to use default (which won't have token mounted)

	// Feature 1: Mount Log ConfigMap
	logVolName := "forensic-logs"
	newPod.Spec.Volumes = append(newPod.Spec.Volumes, corev1.Volume{
		Name: logVolName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: logCMName},
			},
		},
	})

	// Toolkit Volume
	toolsVolName := "toolbox"
	newPod.Spec.Volumes = append(newPod.Spec.Volumes, corev1.Volume{
		Name: toolsVolName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	// Init Container
	initContainer := corev1.Container{
		Name:    "install-toolkit",
		Image:   "busybox:1.36",
		Command: []string{"/bin/sh", "-c", "cp /bin/sh /bin/ls /bin/cat /tools/"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      toolsVolName,
				MountPath: "/tools",
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	}
	// Prepend to InitContainers
	newPod.Spec.InitContainers = append([]corev1.Container{initContainer}, newPod.Spec.InitContainers...)

	// Update References
	// 1. Volumes
	for i, vol := range newPod.Spec.Volumes {
		if vol.ConfigMap != nil {
			if newName, ok := resourceMap[fmt.Sprintf("cm/%s", vol.ConfigMap.Name)]; ok {
				newPod.Spec.Volumes[i].ConfigMap.Name = newName
			}
		}
		if vol.Secret != nil {
			if newName, ok := resourceMap[fmt.Sprintf("secret/%s", vol.Secret.SecretName)]; ok {
				newPod.Spec.Volumes[i].Secret.SecretName = newName
			}
		}
		if vol.Projected != nil {
			for j, source := range vol.Projected.Sources {
				if source.ConfigMap != nil {
					if newName, ok := resourceMap[fmt.Sprintf("cm/%s", source.ConfigMap.Name)]; ok {
						newPod.Spec.Volumes[i].Projected.Sources[j].ConfigMap.Name = newName
					}
				}
				if source.Secret != nil {
					if newName, ok := resourceMap[fmt.Sprintf("secret/%s", source.Secret.Name)]; ok {
						newPod.Spec.Volumes[i].Projected.Sources[j].Secret.Name = newName
					}
				}
			}
		}
	}

	// 2. Containers (Command Override, Probes Removal, Env Refs, Mounts)
	updateContainer := func(c *corev1.Container) {
		// Override Command
		// Update PATH in the command itself
		c.Command = []string{"/usr/local/bin/toolkit/sh", "-c", "export PATH=$PATH:/usr/local/bin/toolkit; echo 'Forensic Mode Active. Run your app manually.'; sleep infinity"}
		c.Args = nil

		// Remove Probes
		c.LivenessProbe = nil
		c.ReadinessProbe = nil
		c.StartupProbe = nil

		// Feature 1: Mount Logs
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
			Name:      logVolName,
			MountPath: "/forensics/original-logs",
			ReadOnly:  true,
		})

		// Feature 2: Mount Toolkit
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
			Name:      toolsVolName,
			MountPath: "/usr/local/bin/toolkit",
		})

		// Security Hardening: Drop Dangerous Capabilities
		if c.SecurityContext == nil {
			c.SecurityContext = &corev1.SecurityContext{}
		}
		if c.SecurityContext.Capabilities == nil {
			c.SecurityContext.Capabilities = &corev1.Capabilities{}
		}
		c.SecurityContext.Capabilities.Drop = append(c.SecurityContext.Capabilities.Drop,
			corev1.Capability("NET_ADMIN"),
			corev1.Capability("SYS_ADMIN"),
			corev1.Capability("SYS_PTRACE"),
		)

		// Update EnvFrom
		for i, envFrom := range c.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				if newName, ok := resourceMap[fmt.Sprintf("cm/%s", envFrom.ConfigMapRef.Name)]; ok {
					c.EnvFrom[i].ConfigMapRef.Name = newName
				}
			}
			if envFrom.SecretRef != nil {
				if newName, ok := resourceMap[fmt.Sprintf("secret/%s", envFrom.SecretRef.Name)]; ok {
					c.EnvFrom[i].SecretRef.Name = newName
				}
			}
		}

		// Update Env
		for i, env := range c.Env {
			if env.ValueFrom != nil {
				if env.ValueFrom.ConfigMapKeyRef != nil {
					if newName, ok := resourceMap[fmt.Sprintf("cm/%s", env.ValueFrom.ConfigMapKeyRef.Name)]; ok {
						c.Env[i].ValueFrom.ConfigMapKeyRef.Name = newName
					}
				}
				if env.ValueFrom.SecretKeyRef != nil {
					if newName, ok := resourceMap[fmt.Sprintf("secret/%s", env.ValueFrom.SecretKeyRef.Name)]; ok {
						c.Env[i].ValueFrom.SecretKeyRef.Name = newName
					}
				}
			}
		}
	}

	for i := range newPod.Spec.Containers {
		updateContainer(&newPod.Spec.Containers[i])
	}
	// We do not modify init containers (except the one we added) to have logs/toolkit,
	// unless necessary. But original init containers might need dependency fix.
	// We skip the first one which is ours (index 0).
	for i := 1; i < len(newPod.Spec.InitContainers); i++ {

		c := &newPod.Spec.InitContainers[i]
		for k, envFrom := range c.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				if newName, ok := resourceMap[fmt.Sprintf("cm/%s", envFrom.ConfigMapRef.Name)]; ok {
					c.EnvFrom[k].ConfigMapRef.Name = newName
				}
			}
			if envFrom.SecretRef != nil {
				if newName, ok := resourceMap[fmt.Sprintf("secret/%s", envFrom.SecretRef.Name)]; ok {
					c.EnvFrom[k].SecretRef.Name = newName
				}
			}
		}
		for k, env := range c.Env {
			if env.ValueFrom != nil {
				if env.ValueFrom.ConfigMapKeyRef != nil {
					if newName, ok := resourceMap[fmt.Sprintf("cm/%s", env.ValueFrom.ConfigMapKeyRef.Name)]; ok {
						c.Env[k].ValueFrom.ConfigMapKeyRef.Name = newName
					}
				}
				if env.ValueFrom.SecretKeyRef != nil {
					if newName, ok := resourceMap[fmt.Sprintf("secret/%s", env.ValueFrom.SecretKeyRef.Name)]; ok {
						c.Env[k].ValueFrom.SecretKeyRef.Name = newName
					}
				}
			}
		}
	}

	return r.Create(ctx, newPod)
}

// startTTLLoop runs a background loop to clean up old forensic pods
func (r *PodReconciler) startTTLLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	logger := log.FromContext(ctx).WithName("ttl-cleaner")
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.cleanupExpiredPods(ctx, logger)
		}
	}
}

func (r *PodReconciler) cleanupExpiredPods(ctx context.Context, logger logr.Logger) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(r.Config.TargetNamespace)); err != nil {
		logger.Error(err, "Failed to list pods for TTL cleanup")
		return
	}

	now := time.Now()
	for _, pod := range pods.Items {
		// Check Hold Annotation
		if pod.Annotations[AnnotationForensicHold] == "true" {
			continue
		}

		// Check label
		val, ok := pod.Labels[LabelForensicTTL]
		if !ok {
			continue
		}
		duration, err := time.ParseDuration(val)
		if err != nil {
			logger.Error(err, "Invalid TTL label", "pod", pod.Name, "value", val)
			continue
		}

		if pod.CreationTimestamp.Add(duration).Before(now) {
			logger.Info("Cleaning up expired forensic pod", "pod", pod.Name)

			// Capture UID to delete dependencies
			uid := string(pod.Labels[LabelSourcePodUID]) // Or pod.UID, but we want the source UID used for grouping

			// 1. Delete Pod
			if err := r.Delete(ctx, &pod); err != nil {
				logger.Error(err, "Failed to delete expired pod", "pod", pod.Name)
				continue
			}

			// 2. Delete Dependencies (ConfigMaps, Secrets) with same source UID
			// Note: This relies on LabelSourcePodUID being accurate on dependencies.
			if uid != "" {
				r.deleteDependencies(ctx, uid, logger)
			}
		}
	}
}

func (r *PodReconciler) deleteDependencies(ctx context.Context, sourceUID string, logger logr.Logger) {
	opts := []client.ListOption{
		client.InNamespace(r.Config.TargetNamespace),
		client.MatchingLabels{LabelSourcePodUID: sourceUID},
	}

	// ConfigMaps
	var cms corev1.ConfigMapList
	if err := r.List(ctx, &cms, opts...); err == nil {
		for _, cm := range cms.Items {
			r.Delete(ctx, &cm)
		}
	}

	// Secrets
	var secrets corev1.SecretList
	if err := r.List(ctx, &secrets, opts...); err == nil {
		for _, s := range secrets.Items {
			r.Delete(ctx, &s)
		}
	}

	// VolumeSnapshots (These are in the Source Namespace, so we search globally by UID)
	// We need to be careful not to list ALL snapshots if we can avoid it, but with LabelSelector it is fine.
	var snapshots snapshotv1.VolumeSnapshotList
	if err := r.List(ctx, &snapshots, client.MatchingLabels{LabelSourcePodUID: sourceUID}); err == nil {
		for _, snap := range snapshots.Items {
			logger.Info("Deleting forensic snapshot", "snapshot", snap.Name, "namespace", snap.Namespace)
			r.Delete(ctx, &snap)
		}
	} else {
		// Log but don't fail, CRD might not exist
		logger.V(1).Info("Could not list VolumeSnapshots for cleanup (CRD missing?)", "error", err)
	}
}

// ... (existing helper methods) ...

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize KubeClient
	config := mgr.GetConfig()
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	r.KubeClient = kubeClient

	// Start TTL Cleaner
	// We use the manager's context (which is cancelled on stop)
	err = mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		r.startTTLLoop(ctx)
		return nil
	}))
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
