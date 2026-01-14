package controllers

import (
	"regexp"
	"testing"
	"time"
)

func TestForensicTimeFormat(t *testing.T) {
	// The regex from Kubernetes validation error message:
	// '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?'
	// We want to ensure our time format produces a string that matches this.

	now := time.Now().UTC()
	val := now.Format(ForensicTimeFormat)

	// 1. Check for colons (primary cause of the bug)
	match, _ := regexp.MatchString(":", val)
	if match {
		t.Errorf("Time format produced string with colons: %s", val)
	}

	// 2. Check against strict K8s label validation regex
	k8sLabelRegex := `^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$`
	matched, err := regexp.MatchString(k8sLabelRegex, val)
	if err != nil {
		t.Fatalf("Regex error: %v", err)
	}
	if !matched {
		t.Errorf("Time format produced invalid K8s label value: %s", val)
	}

	t.Logf("Generated label value: %s", val)
}
