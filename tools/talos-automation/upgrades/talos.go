package upgrades

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type TalosUpgrader struct {
	TalosConfigPath    string
	LogPath            string
	GitHubClient       GitHubFetcher
	mockCurrentVersion string
}

type GitHubFetcher interface {
	FetchBareMetalConfig(owner, repo string) ([]byte, error)
}

type ImageFactoryClient struct {
	BaseURL string
}

type SchematicResponse struct {
	ID string `json:"id"`
}

func NewTalosUpgrader(talosConfigPath, logPath string, githubClient GitHubFetcher) *TalosUpgrader {
	return &TalosUpgrader{
		TalosConfigPath: talosConfigPath,
		LogPath:         logPath,
		GitHubClient:    githubClient,
	}
}

func NewImageFactoryClient() *ImageFactoryClient {
	return &ImageFactoryClient{
		BaseURL: "https://factory.talos.dev",
	}
}

func (t *TalosUpgrader) UpgradeToVersion(version string, nodes []string, githubOwner, githubRepo string, executeCommands bool) error {
	log.Printf("=== Talos Upgrade Process Started ===")
	log.Printf("Target version: %s, Node count: %d, Execute commands: %t", version, len(nodes), executeCommands)
	if !executeCommands && t.mockCurrentVersion != "" {
		log.Printf("Mock current version set: %s", t.mockCurrentVersion)
	}
	log.Printf("Target nodes: %v", nodes)

	// Validate version format
	if !t.isValidVersion(version) {
		log.Printf("ERROR: Invalid Talos version format: %s", version)
		return fmt.Errorf("invalid Talos version format: %s", version)
	}
	log.Printf("Version format validation passed")

	// Clean version (remove 'v' prefix if present)
	cleanVersion := strings.TrimPrefix(version, "v")
	log.Printf("Cleaned target version: %s", cleanVersion)

	// Check if upgrade is needed by getting current version
	log.Printf("Fetching current Talos version...")
	currentVersion, err := t.getCurrentVersion(executeCommands)
	if err != nil {
		if executeCommands {
			// CRITICAL: In production, we must know current version before upgrading
			log.Printf("FATAL ERROR: Cannot determine current Talos version in production mode: %v", err)
			return fmt.Errorf("cannot determine current Talos version: %w", err)
		} else {
			// In test mode, we can proceed with warnings
			log.Printf("WARNING: Could not determine current Talos version in test mode: %v", err)
		}
	} else {
		log.Printf("Version comparison: Current Talos=%s, Target=%s", currentVersion, cleanVersion)
		
		// Check if upgrade is needed
		if currentVersion == cleanVersion {
			log.Printf("DECISION: Talos is already at version %s, no upgrade needed", cleanVersion)
			return nil
		}
		
		log.Printf("DECISION: Talos upgrade needed from %s to %s", currentVersion, cleanVersion)
	}

	if !executeCommands {
		log.Printf("*** DRY RUN MODE: Would upgrade Talos from %s to %s on %d nodes ***", currentVersion, cleanVersion, len(nodes))
		log.Printf("*** DRY RUN MODE: Skipping Image Factory and upgrade execution ***")
		return nil
	}
	
	log.Printf("*** PRODUCTION MODE: Executing actual upgrade commands ***")

	// Only fetch bare-metal config and call Image Factory if we're actually upgrading
	log.Printf("Fetching bare-metal configuration for upgrade...")
	bareMetalConfig, err := t.GitHubClient.FetchBareMetalConfig(githubOwner, githubRepo)
	if err != nil {
		return fmt.Errorf("failed to fetch bare-metal config: %w", err)
	}

	log.Printf("Creating schematic via Image Factory...")
	factory := NewImageFactoryClient()
	schematicID, err := factory.CreateSchematic(bareMetalConfig)
	if err != nil {
		return fmt.Errorf("failed to create schematic: %w", err)
	}

	log.Printf("Generated schematic ID: %s", schematicID)

	// Build installer image URL
	installerImage := fmt.Sprintf("factory.talos.dev/metal-installer-secureboot/%s:v%s", schematicID, cleanVersion)
	log.Printf("Using installer image: %s", installerImage)

	// Upgrade each node sequentially
	for i, node := range nodes {
		log.Printf("Upgrading node %d/%d: %s", i+1, len(nodes), node)
		
		if err := t.upgradeNode(node, installerImage, cleanVersion); err != nil {
			return fmt.Errorf("failed to upgrade node %s: %w", node, err)
		}
		
		log.Printf("Successfully upgraded node: %s", node)
	}

	log.Printf("Talos upgrade to version %s completed successfully for all nodes", cleanVersion)
	return nil
}

func (t *TalosUpgrader) upgradeNode(node, installerImage, version string) error {
	args := []string{
		"--talosconfig", t.TalosConfigPath,
		"upgrade",
		"--nodes", node,
		"--image", installerImage,
		"--preserve",
	}

	cmd := exec.Command("talosctl", args...)

	// Create log file for this node upgrade
	timestamp := time.Now().Format("20060102-150405")
	logFileName := fmt.Sprintf("talos-upgrade-node-%s-%s-%s.log", strings.ReplaceAll(node, ".", "-"), version, timestamp)
	logFilePath := fmt.Sprintf("%s/%s", t.LogPath, logFileName)

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

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("talosctl upgrade command failed for node %s: %w", node, err)
	}

	return nil
}

func (f *ImageFactoryClient) CreateSchematic(bareMetalConfig []byte) (string, error) {
	url := fmt.Sprintf("%s/schematics", f.BaseURL)

	resp, err := http.Post(url, "application/yaml", bytes.NewReader(bareMetalConfig))
	if err != nil {
		return "", fmt.Errorf("failed to create schematic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Image Factory API error %d: %s", resp.StatusCode, string(body))
	}

	var response SchematicResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode schematic response: %w", err)
	}

	return response.ID, nil
}

func (t *TalosUpgrader) isValidVersion(version string) bool {
	// Match semantic versions with optional 'v' prefix
	versionRegex := regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
	return versionRegex.MatchString(version)
}

// Get current Talos version from cluster
func (t *TalosUpgrader) getCurrentVersion(executeCommands bool) (string, error) {
	return t.GetCurrentVersion(executeCommands)
}

// GetCurrentVersion - public method to get current Talos version
func (t *TalosUpgrader) GetCurrentVersion(executeCommands bool) (string, error) {
	log.Printf("getCurrentVersion called: executeCommands=%t, mockVersion=%s", executeCommands, t.mockCurrentVersion)
	
	if !executeCommands {
		// Return mock version for testing - can be overridden by SetMockCurrentVersion
		if t.mockCurrentVersion != "" {
			log.Printf("Returning mock current Talos version: %s", t.mockCurrentVersion)
			return t.mockCurrentVersion, nil
		}
		log.Printf("Returning default mock current Talos version: 1.10.6")
		return "1.10.6", nil
	}
	
	log.Printf("PRODUCTION MODE: Getting actual current Talos version from cluster")

	cmd := exec.Command("talosctl", "--talosconfig", t.TalosConfigPath, "version", "--short")

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current Talos version: %w", err)
	}

	// Parse version from talosctl output
	versionRegex := regexp.MustCompile(`Client:\s+Talos v?(\d+\.\d+\.\d+)`)
	matches := versionRegex.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("could not parse Talos version from output: %s", string(output))
	}

	return matches[1], nil
}

func (t *TalosUpgrader) SetMockCurrentVersion(version string) {
	log.Printf("SetMockCurrentVersion called: setting mock Talos version to %s", version)
	t.mockCurrentVersion = version
	log.Printf("Mock Talos version now set to: %s", t.mockCurrentVersion)
}

// Method to validate that schematic hasn't changed unexpectedly
func (t *TalosUpgrader) validateSchematic(bareMetalConfig []byte, expectedHash string) error {
	// Calculate hash of the bare-metal config
	hasher := sha256.New()
	hasher.Write(bareMetalConfig)
	actualHash := hex.EncodeToString(hasher.Sum(nil))

	if expectedHash != "" && actualHash != expectedHash {
		log.Printf("Warning: bare-metal config hash changed from %s to %s", expectedHash, actualHash)
		// Don't fail, just warn - configuration may have legitimately changed
	}

	return nil
}