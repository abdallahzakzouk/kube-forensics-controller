package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var (
	targetNamespace string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kubectl_forensic",
		Short: "Kubectl plugin for interacting with Forensic Pods",
		Long: `A kubectl plugin to list, access, and manage forensic pods created by the kube-forensics-controller. 
		
Examples:
  kubectl forensic list
  kubectl forensic access <pod-name>
  kubectl forensic logs <pod-name>
  kubectl forensic export <pod-name> > crash.log`,
	}

	rootCmd.PersistentFlags().StringVarP(&targetNamespace, "namespace", "n", "debug-forensics", "Target namespace for forensic pods")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available forensic pods",
		Run: func(cmd *cobra.Command, args []string) {
			c := exec.Command("kubectl", "get", "pods", "-n", targetNamespace, "-l", "forensic-time")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Run()
		},
	}

	accessCmd := &cobra.Command{
		Use:   "access [POD_NAME]",
		Short: "Exec into a forensic pod using the injected toolkit",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			podName := args[0]
			fmt.Fprintf(os.Stderr, "Connecting to forensic pod %s in %s...\n", podName, targetNamespace)

			// Fetch Hints
			hintCmd := exec.Command("kubectl", "get", "pod", "-n", targetNamespace, podName, "-o", "jsonpath={.metadata.annotations.forensic\\.io/original-command} {.metadata.annotations.forensic\\.io/original-args}")
			hintBytes, _ := hintCmd.Output()
			hint := strings.TrimSpace(string(hintBytes))

			if hint != "" {
				fmt.Fprintf(os.Stderr, "\n💡 Suggested Start Command (from crash):\n   %s\n\n", hint)
			} else {
				fmt.Fprintf(os.Stderr, "\n(No original command captured in annotations)\n")
			}

			// Try to connect using the toolkit shell
			c := exec.Command("kubectl", "exec", "-it", "-n", targetNamespace, podName, "--", "/usr/local/bin/toolkit/sh")
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
			}
		},
	}

	logsCmd := &cobra.Command{
		Use:   "logs [POD_NAME]",
		Short: "View the original crash logs stored in the forensic pod",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			podName := args[0]

			c := exec.Command("kubectl", "exec", "-n", targetNamespace, podName, "--", "cat", "/forensics/original-logs/crash.log")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Run()
		},
	}

	exportCmd := &cobra.Command{
		Use:   "export [POD_NAME]",
		Short: "Export logs with integrity check (SHA256)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			podName := args[0]

			// 1. Get Hash from Annotation
			hashCmd := exec.Command("kubectl", "get", "pod", "-n", targetNamespace, podName, "-o", "jsonpath={.metadata.annotations.forensic\\.io/log-sha256}")
			expectedHashBytes, err := hashCmd.Output()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to get log hash: %v\n", err)
				os.Exit(1)
			}
			expectedHash := strings.TrimSpace(string(expectedHashBytes))

			// 2. Fetch Logs
			fetchCmd := exec.Command("kubectl", "exec", "-n", targetNamespace, podName, "--", "cat", "/forensics/original-logs/crash.log")
			logData, err := fetchCmd.Output()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to fetch logs: %v\n", err)
				os.Exit(1)
			}

			// 3. Verify Hash
			hasher := sha256.New()
			hasher.Write(logData)
			actualHash := hex.EncodeToString(hasher.Sum(nil))

			if expectedHash != "" && actualHash != expectedHash {
				fmt.Fprintf(os.Stderr, "WARNING: INTEGRITY CHECK FAILED!\nExpected: %s\nActual:   %s\n", expectedHash, actualHash)
			} else {
				fmt.Fprintf(os.Stderr, "Integrity Check Passed: %s\n", actualHash)
			}

			// 4. Output to Stdout
			fmt.Print(string(logData))
		},
	}

	cleanupCmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete all forensic pods in the namespace",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Deleting all forensic pods in %s...\n", targetNamespace)
			c := exec.Command("kubectl", "delete", "pods", "-n", targetNamespace, "-l", "forensic-time")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Run()
		},
	}

	rootCmd.AddCommand(listCmd, accessCmd, logsCmd, exportCmd, cleanupCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
