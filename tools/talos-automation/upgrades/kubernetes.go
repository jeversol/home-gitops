package upgrades

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type KubernetesUpgrader struct {
	TalosConfigPath    string
	LogPath            string
	mockCurrentVersion string
}

func NewKubernetesUpgrader(talosConfigPath, logPath string) *KubernetesUpgrader {
	return &KubernetesUpgrader{
		TalosConfigPath: talosConfigPath,
		LogPath:         logPath,
	}
}

func (k *KubernetesUpgrader) UpgradeToVersion(version, controlPlaneNode string, executeCommands bool) error {
	log.Printf("=== Kubernetes Upgrade Process Started ===")
	log.Printf("Target version: %s, Control plane node: %s, Execute commands: %t", version, controlPlaneNode, executeCommands)
	if !executeCommands && k.mockCurrentVersion != "" {
		log.Printf("Mock current version set: %s", k.mockCurrentVersion)
	}

	// Validate version format (e.g., "1.33.4" or "v1.33.4")
	if !k.isValidVersion(version) {
		log.Printf("ERROR: Invalid version format detected: %s", version)
		return fmt.Errorf("invalid Kubernetes version format: %s", version)
	}
	log.Printf("Version format validation passed")

	// Remove 'v' prefix if present
	cleanVersion := strings.TrimPrefix(version, "v")
	log.Printf("Cleaned target version: %s", cleanVersion)

	// Get current version for comparison
	log.Printf("Fetching current Kubernetes version...")
	currentVersion, err := k.GetCurrentVersion(controlPlaneNode, executeCommands)
	if err != nil {
		if executeCommands {
			// CRITICAL: In production, we must know current version before upgrading
			log.Printf("FATAL ERROR: Cannot determine current Kubernetes version in production mode: %v", err)
			return fmt.Errorf("cannot determine current Kubernetes version: %w", err)
		} else {
			// In test mode, we can proceed with warnings
			log.Printf("WARNING: Could not determine current Kubernetes version in test mode: %v", err)
		}
	} else {
		log.Printf("Version comparison: Current K8s=%s, Target=%s", currentVersion, cleanVersion)
		
		// Check if upgrade is needed
		if currentVersion == cleanVersion {
			log.Printf("DECISION: Kubernetes is already at version %s, no upgrade needed", cleanVersion)
			return nil
		}
		
		// Check for downgrades
		if k.isDowngrade(currentVersion, cleanVersion) {
			log.Printf("ERROR: Downgrade detected - refusing to downgrade from %s to %s", currentVersion, cleanVersion)
			return fmt.Errorf("refusing to downgrade Kubernetes from %s to %s", currentVersion, cleanVersion)
		}
		
		log.Printf("DECISION: Upgrade needed from %s to %s", currentVersion, cleanVersion)
	}

	if !executeCommands {
		log.Printf("*** DRY RUN MODE: Would upgrade Kubernetes from %s to %s ***", currentVersion, cleanVersion)
		log.Printf("*** DRY RUN MODE: Skipping actual upgrade execution ***")
		return nil
	}
	
	log.Printf("*** PRODUCTION MODE: Executing actual upgrade commands ***")

	// Run dry-run first
	log.Printf("Running dry-run for Kubernetes upgrade...")
	if err := k.runUpgradeCommand(cleanVersion, controlPlaneNode, true, executeCommands); err != nil {
		return fmt.Errorf("dry-run failed: %w", err)
	}

	log.Printf("Dry-run successful, proceeding with actual upgrade...")
	
	// Run actual upgrade
	if err := k.runUpgradeCommand(cleanVersion, controlPlaneNode, false, executeCommands); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}

	log.Printf("Kubernetes upgrade to version %s completed successfully", cleanVersion)
	return nil
}

func (k *KubernetesUpgrader) runUpgradeCommand(version, node string, dryRun, executeCommands bool) error {
	args := []string{
		"--talosconfig", k.TalosConfigPath,
		"upgrade-k8s",
		"--to", version,
		"-n", node,
	}

	if dryRun {
		args = append(args, "--dry-run")
	}

	cmd := exec.Command("talosctl", args...)

	// Create log file for this upgrade
	timestamp := time.Now().Format("20060102-150405")
	logFileName := fmt.Sprintf("k8s-upgrade-%s-%s.log", version, timestamp)
	logFilePath := fmt.Sprintf("%s/%s", k.LogPath, logFileName)

	logFile, err := os.Create(logFilePath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	// Redirect command output to both log file and stdout
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	log.Printf("Running: talosctl %s", strings.Join(args, " "))
	log.Printf("Logging to: %s", logFilePath)

	if !executeCommands {
		log.Printf("DRY RUN: Would execute command but executeCommands=false")
		return nil
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("talosctl command failed: %w", err)
	}

	return nil
}

func (k *KubernetesUpgrader) GetCurrentVersion(node string, executeCommands bool) (string, error) {
	log.Printf("GetCurrentVersion called: node=%s, executeCommands=%t, mockVersion=%s", node, executeCommands, k.mockCurrentVersion)
	
	if !executeCommands {
		// In diagnostics mode, check for mock version override first
		if k.mockCurrentVersion != "" {
			log.Printf("DIAGNOSTICS MODE: Using test override K8s version: %s", k.mockCurrentVersion)
			return k.mockCurrentVersion, nil
		}
		log.Printf("DIAGNOSTICS MODE: Using default mock K8s version: 1.33.3")
		return "1.33.3", nil
	}
	
	log.Printf("PRODUCTION MODE: Getting actual current version from cluster")

	cmd := exec.Command("talosctl", "--talosconfig", k.TalosConfigPath, "get", "members", "-o", "json", "-n", node)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current version: %w", err)
	}

	// Parse Kubernetes version from members JSON output
	var members []map[string]interface{}
	if err := json.Unmarshal(output, &members); err != nil {
		return "", fmt.Errorf("failed to parse members JSON: %w", err)
	}

	if len(members) == 0 {
		return "", fmt.Errorf("no cluster members found")
	}

	// Look for kubernetes version in the first member
	member := members[0]
	if spec, ok := member["spec"].(map[string]interface{}); ok {
		if k8sVersion, ok := spec["kubernetesVersion"].(string); ok {
			// Clean the version (remove 'v' prefix if present)
			cleanVersion := strings.TrimPrefix(k8sVersion, "v")
			return cleanVersion, nil
		}
	}

	return "", fmt.Errorf("could not find Kubernetes version in members output")
}

func (k *KubernetesUpgrader) isValidVersion(version string) bool {
	// Match semantic versions with optional 'v' prefix
	versionRegex := regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
	return versionRegex.MatchString(version)
}

func (k *KubernetesUpgrader) isDowngrade(current, target string) bool {
	// Simple string comparison for now - could be enhanced with proper semver parsing
	return strings.Compare(current, target) > 0
}

func (k *KubernetesUpgrader) SetMockCurrentVersion(version string) {
	log.Printf("SetMockCurrentVersion called: setting mock version to %s", version)
	k.mockCurrentVersion = version
	log.Printf("Mock version now set to: %s", k.mockCurrentVersion)
}