package collector

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobConfig holds configuration for the Collector Job
type JobConfig struct {
	Namespace      string
	NodeName       string
	CheckpointPath string
	S3Bucket       string
	S3Region       string
	S3Key          string
	Image          string
	OwnerReference metav1.OwnerReference
}

// BuildJob constructs the Collector Job
func BuildJob(cfg JobConfig) *batchv1.Job {
	hostPathType := corev1.HostPathFile

	// We run as root to read the checkpoint file owned by root on the node
	// This requires privileged PSP/PSA or explicit security context
	rootUser := int64(0)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "forensic-collector-",
			Namespace:    cfg.Namespace,
			Labels: map[string]string{
				"forensic-job": "collector",
			},
			OwnerReferences: []metav1.OwnerReference{cfg.OwnerReference},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: func(i int32) *int32 { return &i }(300), // Cleanup after 5 mins
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeName:           cfg.NodeName, // Pin to the node where the file is
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "kube-forensics-controller", // Use same SA to simplify auth if needed, or default
					Containers: []corev1.Container{
						{
							Name:  "collector",
							Image: cfg.Image, // Use the controller image which has the 'collector' subcommand
							Command: []string{
								"/manager",
								"collector",
								"--file=" + cfg.CheckpointPath,
								"--s3-bucket=" + cfg.S3Bucket,
								"--s3-region=" + cfg.S3Region,
								"--s3-key=" + cfg.S3Key,
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  &rootUser,                              // Checkpoints are usually root:root
								Privileged: func(b bool) *bool { return &b }(true), // Likely needed for hostPath read
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "checkpoint-file",
									MountPath: cfg.CheckpointPath, // Mount exact file path
									ReadOnly:  true,
								},
							},
							// Inject AWS Creds if present in Env (handled by SA/IRSA usually)
							// If we rely on Env vars in the controller, we should propagate them here.
							// For MVP, we assume IRSA or Node Role.
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "checkpoint-file",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: cfg.CheckpointPath,
									Type: &hostPathType,
								},
							},
						},
					},
				},
			},
		},
	}
	return job
}
