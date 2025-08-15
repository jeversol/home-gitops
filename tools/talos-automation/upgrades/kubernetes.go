package upgrades

import (
    "context"
    "crypto/tls"
    "crypto/x509"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/exec"
    "regexp"
    "strings"
    "time"

    "gopkg.in/yaml.v3"
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

    log.Printf("Getting Kubernetes version via kube-apiserver /version")

    // ALWAYS fetch real version from cluster APIs regardless of executeCommands
    // This ensures diagnostics and production workflows are identical until command execution
    
    // Fetch kubeconfig from the control plane node
    kubeconfig, err := k.fetchKubeconfig(node)
    if err != nil {
        return "", fmt.Errorf("failed to fetch kubeconfig: %w", err)
    }

    // Extract server + credentials
    server, ca, cert, key, token, err := parseKubeconfigForCreds(kubeconfig)
    if err != nil {
        return "", fmt.Errorf("failed to parse kubeconfig: %w", err)
    }

    // Build HTTP client with TLS using provided creds
    realVersion, err := getAPIServerVersion(server, ca, cert, key, token)
    if err != nil {
        return "", err
    }

    cleanRealVersion := strings.TrimPrefix(realVersion, "v")
    log.Printf("Real K8s version detected: %s", cleanRealVersion)
    
    // Apply test override if provided (for diagnostics testing)
    if !executeCommands && k.mockCurrentVersion != "" {
        log.Printf("DIAGNOSTICS: Overriding real version %s with test version %s", cleanRealVersion, k.mockCurrentVersion)
        return k.mockCurrentVersion, nil
    }
    
    return cleanRealVersion, nil
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

// fetchKubeconfig obtains the admin kubeconfig from a node
func (k *KubernetesUpgrader) fetchKubeconfig(node string) ([]byte, error) {
    log.Printf("Fetching kubeconfig from control-plane node: %s", node)
    // Write kubeconfig to a temporary file, then read it back.
    tmpFile, err := os.CreateTemp("", "talos-kubeconfig-*.yaml")
    if err != nil {
        return nil, fmt.Errorf("failed to create temp kubeconfig file: %w", err)
    }
    tmpPath := tmpFile.Name()
    tmpFile.Close()
    defer os.Remove(tmpPath)

    // Use positional local-path, and disable merge so we get a standalone file
    args := []string{"--talosconfig", k.TalosConfigPath, "kubeconfig", "--merge=false", "-n", node, "--force", tmpPath}
    log.Printf("Running: talosctl %s", strings.Join(args, " "))
    cmd := exec.Command("talosctl", args...)
    if out, err := cmd.CombinedOutput(); err != nil {
        return nil, fmt.Errorf("talosctl kubeconfig failed: %v: %s", err, string(out))
    }

    data, err := os.ReadFile(tmpPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read generated kubeconfig: %w", err)
    }
    return data, nil
}

// (removed K8S_API_ENDPOINT override; kubeconfig already contains the cluster endpoint)

// Kubeconfig structures (minimal)
type kubeConfig struct {
    CurrentContext string         `yaml:"current-context"`
    Clusters       []namedCluster `yaml:"clusters"`
    Contexts       []namedContext `yaml:"contexts"`
    Users          []namedUser    `yaml:"users"`
}

type namedCluster struct {
    Name    string       `yaml:"name"`
    Cluster clusterEntry `yaml:"cluster"`
}

type clusterEntry struct {
    Server                   string `yaml:"server"`
    CertificateAuthorityData string `yaml:"certificate-authority-data"`
}

type namedContext struct {
    Name    string       `yaml:"name"`
    Context contextEntry `yaml:"context"`
}

type contextEntry struct {
    Cluster string `yaml:"cluster"`
    User    string `yaml:"user"`
}

type namedUser struct {
    Name string    `yaml:"name"`
    User userEntry `yaml:"user"`
}

type userEntry struct {
    ClientCertificateData string `yaml:"client-certificate-data"`
    ClientKeyData         string `yaml:"client-key-data"`
    Token                 string `yaml:"token"`
}

// parseKubeconfigForCreds extracts server URL and credentials
func parseKubeconfigForCreds(data []byte) (server string, ca, cert, key []byte, token string, err error) {
    var kc kubeConfig
    if err = yaml.Unmarshal(data, &kc); err != nil {
        return
    }
    var ctx contextEntry
    for _, c := range kc.Contexts {
        if c.Name == kc.CurrentContext {
            ctx = c.Context
            break
        }
    }
    if ctx.Cluster == "" || ctx.User == "" {
        err = fmt.Errorf("current context not found or incomplete")
        return
    }
    var cl clusterEntry
    for _, c := range kc.Clusters {
        if c.Name == ctx.Cluster {
            cl = c.Cluster
            break
        }
    }
    if cl.Server == "" || cl.CertificateAuthorityData == "" {
        err = fmt.Errorf("cluster entry missing server/CA")
        return
    }
    var usr userEntry
    for _, u := range kc.Users {
        if u.Name == ctx.User {
            usr = u.User
            break
        }
    }
    // Decode CA and optional client cert/key
    ca, err = base64.StdEncoding.DecodeString(cl.CertificateAuthorityData)
    if err != nil {
        err = fmt.Errorf("failed to decode CA: %w", err)
        return
    }
    if usr.ClientCertificateData != "" && usr.ClientKeyData != "" {
        cert, err = base64.StdEncoding.DecodeString(usr.ClientCertificateData)
        if err != nil {
            err = fmt.Errorf("failed to decode client cert: %w", err)
            return
        }
        key, err = base64.StdEncoding.DecodeString(usr.ClientKeyData)
        if err != nil {
            err = fmt.Errorf("failed to decode client key: %w", err)
            return
        }
    } else if usr.Token != "" {
        token = usr.Token
    } else {
        err = fmt.Errorf("no usable credentials in kubeconfig user entry")
        return
    }
    server = strings.TrimRight(cl.Server, "/")
    return
}

// getAPIServerVersion hits the /version endpoint and returns gitVersion
func getAPIServerVersion(server string, ca, cert, key []byte, token string) (string, error) {
    // Root CA pool
    roots := x509.NewCertPool()
    if ok := roots.AppendCertsFromPEM(ca); !ok {
        return "", fmt.Errorf("failed to parse CA cert")
    }
    tlsCfg := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
    if len(cert) > 0 && len(key) > 0 {
        pair, err := tls.X509KeyPair(cert, key)
        if err != nil {
            return "", fmt.Errorf("invalid client cert/key: %w", err)
        }
        tlsCfg.Certificates = []tls.Certificate{pair}
    }
    tr := &http.Transport{TLSClientConfig: tlsCfg}
    client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

    const maxRetries = 3
    const retryDelay = 5 * time.Second

    var lastErr error
    for attempt := 1; attempt <= maxRetries; attempt++ {
        req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server+"/version", nil)
        if err != nil {
            return "", fmt.Errorf("failed to create request: %w", err)
        }
        req.Header.Set("Accept", "application/json")
        if token != "" && len(cert) == 0 {
            req.Header.Set("Authorization", "Bearer "+token)
        }

        resp, err := client.Do(req)
        if err != nil {
            lastErr = fmt.Errorf("attempt %d/%d: failed to call /version: %w", attempt, maxRetries, err)
        } else {
            defer resp.Body.Close()
            if resp.StatusCode != http.StatusOK {
                lastErr = fmt.Errorf("attempt %d/%d: /version returned %d", attempt, maxRetries, resp.StatusCode)
            } else {
                var ver struct{ GitVersion string `json:"gitVersion"` }
                if err := json.NewDecoder(resp.Body).Decode(&ver); err != nil {
                    lastErr = fmt.Errorf("attempt %d/%d: failed to decode /version: %w", attempt, maxRetries, err)
                } else if ver.GitVersion == "" {
                    lastErr = fmt.Errorf("attempt %d/%d: gitVersion missing in /version response", attempt, maxRetries)
                } else {
                    return ver.GitVersion, nil
                }
            }
        }

        log.Printf("K8s /version query failed: %v", lastErr)
        if attempt < maxRetries {
            log.Printf("Retrying in %s...", retryDelay)
            time.Sleep(retryDelay)
        }
    }
    if lastErr == nil {
        lastErr = fmt.Errorf("unknown error while querying /version")
    }
    return "", lastErr
}
