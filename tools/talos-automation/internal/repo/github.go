package repo

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"gopkg.in/yaml.v3"
)

type Versions struct {
	TalosVersion      string `yaml:"talosVersion"`
	KubernetesVersion string `yaml:"kubernetesVersion"`
}

type GitHubContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type GitHubClient struct {
	Token string
}

func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{Token: token}
}

func (g *GitHubClient) FetchVersions(owner, repo string) (*Versions, error) {
	decoded, err := g.fetchFileContent(owner, repo, "infrastructure/cluster/track-versions.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch versions file: %w", err)
	}

	var versions Versions
	if err := yaml.Unmarshal(decoded, &versions); err != nil {
		return nil, fmt.Errorf("failed to parse versions YAML: %w", err)
	}

	return &versions, nil
}

func (g *GitHubClient) FetchBareMetalConfig(owner, repo string) ([]byte, error) {
	return g.fetchFileContent(owner, repo, "infrastructure/cluster/bare-metal.yaml")
}

func (g *GitHubClient) fetchFileContent(owner, repo, filePath string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, filePath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if g.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", g.Token))
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch file from GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	var content GitHubContent
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub API response: %w", err)
	}

	if content.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding: %s", content.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(content.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %w", err)
	}

	return decoded, nil
}