#!/usr/bin/env python3
"""
Update Kubernetes version constraints in renovate.json5 based on Talos support matrix.

This script reads the current Talos version from base-controlplane.yaml (or accepts it as an argument),
fetches the Talos support matrix to determine which Kubernetes versions are supported,
and updates the allowedVersions constraint in .github/renovate.json5 accordingly.

Exit codes:
  0 - Success, constraint was updated
  1 - Error occurred
  2 - No changes needed (constraint already up to date)
"""

import yaml
import re
import urllib.request
import json
import sys
from html.parser import HTMLParser


class TalosMatrixParser(HTMLParser):
    """
    HTML parser to extract Kubernetes versions from Talos support matrix.

    The support matrix is structured as an HTML table with:
    - Header row: Talos versions (e.g., "1.12", "1.11")
    - Data rows: Features/Components with version support in each column
    - Kubernetes row: Contains supported K8s versions for each Talos version
    """

    def __init__(self, talos_minor):
        super().__init__()
        self.talos_minor = talos_minor
        self.k8s_versions = set()

        # Parser state
        self.in_table = False
        self.in_row = False
        self.in_cell = False
        self.current_row_cells = []
        self.current_cell_text = []

        # Table structure tracking
        self.talos_col_idx = None
        self.header_processed = False

    def handle_starttag(self, tag, attrs):
        if tag == 'table':
            self.in_table = True
        elif tag == 'tr' and self.in_table:
            self.in_row = True
            self.current_row_cells = []
        elif tag in ('td', 'th') and self.in_row:
            self.in_cell = True
            self.current_cell_text = []

    def handle_data(self, data):
        if self.in_cell:
            text = data.strip()
            if text:
                self.current_cell_text.append(text)

    def handle_endtag(self, tag):
        if tag == 'table':
            self.in_table = False
            self.header_processed = False
            self.talos_col_idx = None
        elif tag == 'tr' and self.in_row:
            self.process_row(self.current_row_cells)
            self.in_row = False
        elif tag in ('td', 'th') and self.in_cell:
            cell_content = ' '.join(self.current_cell_text)
            self.current_row_cells.append(cell_content)
            self.in_cell = False

    def process_row(self, cells):
        if not cells:
            return

        # First row in table is the header - find our Talos version column
        if not self.header_processed:
            for i, cell in enumerate(cells):
                if cell == self.talos_minor or cell == f"v{self.talos_minor}" or cell == f"{self.talos_minor}":
                    self.talos_col_idx = i
                    print(f"  Found Talos {self.talos_minor} column at index {i}")
                    break
            self.header_processed = True
            return

        # Skip if we haven't found the Talos column yet
        if self.talos_col_idx is None:
            return

        # Check if this is the Kubernetes row
        first_cell_lower = cells[0].lower()
        if 'kubernetes' not in first_cell_lower and 'k8s' not in first_cell_lower:
            return

        # Extract K8s versions from the Talos column
        if self.talos_col_idx < len(cells):
            cell_text = cells[self.talos_col_idx]
            print(f"  Kubernetes row found, Talos {self.talos_minor} cell: {cell_text}")

            # Extract all version numbers like "1.30", "1.31", etc.
            matches = re.findall(r'\b1\.(\d+)', cell_text)
            # Filter to only keep versions >= 1.30
            versions = [v for v in matches if int(v) >= 30]

            if versions:
                self.k8s_versions.update(versions)
                print(f"    Found K8s versions: {sorted(versions)}")
            else:
                print(f"    No valid K8s versions (>= 1.30) found")


def fetch_support_matrix_html(talos_minor):
    """
    Fetch and parse the Talos support matrix from the official documentation.

    Returns:
        set: Set of supported Kubernetes minor versions (as strings, e.g., {"30", "31", "32"})
    """
    matrix_url = f"https://www.talos.dev/v{talos_minor}/introduction/support-matrix/"
    print(f"Fetching support matrix from: {matrix_url}")

    try:
        print("Fetching support matrix HTML...")
        req = urllib.request.Request(
            matrix_url,
            headers={'User-Agent': 'renovate-bot/update-k8s-constraints'}
        )

        with urllib.request.urlopen(req, timeout=10) as response:
            html_content = response.read().decode('utf-8')
            print(f"HTTP {response.status} - Got {len(html_content)} bytes")

        # Parse HTML to extract K8s versions
        parser = TalosMatrixParser(talos_minor)
        parser.feed(html_content)

        if parser.k8s_versions:
            print("Parsed support matrix successfully")
            return parser.k8s_versions
        else:
            print("WARNING: Could not find specific versions in support matrix")
            return set()

    except urllib.error.HTTPError as e:
        print(f"WARNING: HTTP error fetching support matrix: {e.code} {e.reason}")
        return set()
    except urllib.error.URLError as e:
        print(f"WARNING: Failed to fetch support matrix: {e.reason}")
        return set()
    except Exception as e:
        print(f"WARNING: Unexpected error parsing support matrix: {e}")
        return set()


def fetch_support_from_github_api(talos_minor):
    """
    Fallback: Fetch K8s version support from GitHub release notes.

    Returns:
        set: Set of supported Kubernetes minor versions
    """
    gh_url = "https://api.github.com/repos/siderolabs/talos/releases"
    print(f"Trying fallback: GitHub API...")
    print(f"GitHub API: {gh_url}")

    try:
        req = urllib.request.Request(
            gh_url,
            headers={'User-Agent': 'renovate-bot/update-k8s-constraints'}
        )

        with urllib.request.urlopen(req, timeout=10) as response:
            releases = json.loads(response.read().decode('utf-8'))

        # Find the latest release for this minor version
        target_release = None
        for release in releases:
            if release['tag_name'].startswith(f"v{talos_minor}."):
                target_release = release
                break

        if not target_release:
            print(f"ERROR: Could not find release for v{talos_minor}")
            raise Exception("No matching release found")

        print(f"Found release: {target_release['tag_name']}")

        # Parse release notes for Kubernetes version
        body = target_release.get('body', '')
        print("Searching release notes for Kubernetes versions...")

        # Look for patterns like "Kubernetes 1.35" or "k8s v1.35"
        k8s_matches = re.findall(r'[Kk]ubernetes.*?v?1\.(\d+)', body)
        # Filter to versions >= 1.30
        k8s_versions = set(v for v in k8s_matches if int(v) >= 30)

        if k8s_versions:
            print(f"Found in release notes: {sorted(k8s_versions)}")
            return k8s_versions
        else:
            print("ERROR: Could not find Kubernetes versions in release notes")
            raise Exception("No K8s versions found in release notes")

    except Exception as e:
        print(f"ERROR: GitHub API also failed: {e}")
        return set()


def get_supported_k8s_versions(talos_minor):
    """
    Get supported Kubernetes versions for a given Talos version.
    Tries support matrix first, falls back to GitHub API.

    Returns:
        list: Sorted list of supported K8s minor versions
    """
    # Try primary method: HTML support matrix
    supported_k8s_versions = fetch_support_matrix_html(talos_minor)

    # Fallback: GitHub API
    if not supported_k8s_versions:
        print("Trying fallback method...")
        supported_k8s_versions = fetch_support_from_github_api(talos_minor)

    if not supported_k8s_versions:
        print("ERROR: Cannot determine supported Kubernetes versions")
        print("Please check https://www.talos.dev/latest/introduction/support-matrix/ manually")
        sys.exit(1)

    return sorted(list(supported_k8s_versions))


def extract_talos_version():
    """
    Extract Talos version from base-controlplane.yaml.

    Returns:
        str: Talos minor version (e.g., "1.12")
    """
    try:
        with open('tools/cluster/base-controlplane.yaml', 'r') as f:
            config = yaml.safe_load(f)

        factory_image = config['machine']['install']['image']
        talos_version_match = re.search(r':v(\d+\.\d+)\.\d+$', factory_image)

        if not talos_version_match:
            print("ERROR: Could not extract Talos version from factory image")
            sys.exit(1)

        return talos_version_match.group(1)
    except FileNotFoundError:
        print("ERROR: Could not find tools/cluster/base-controlplane.yaml")
        print("Make sure you're running this script from the repository root")
        sys.exit(1)
    except Exception as e:
        print(f"ERROR: Failed to read Talos version: {e}")
        sys.exit(1)


def update_renovate_config(talos_minor, supported_k8s_versions):
    """
    Update the renovate.json5 file with new Kubernetes version constraints.

    Returns:
        bool: True if changes were made, False if already up to date
    """
    # Generate the allowedVersions regex
    if len(supported_k8s_versions) == 1:
        allowed_versions = f"/^v1\\.{supported_k8s_versions[0]}\\./"
    else:
        versions_str = '|'.join(supported_k8s_versions)
        allowed_versions = f"/^v1\\.({versions_str})\\./"

    print(f"Generated allowedVersions constraint: {allowed_versions}")

    # Read current renovate.json5
    try:
        with open('.github/renovate.json5', 'r') as f:
            renovate_content = f.read()
    except FileNotFoundError:
        print("ERROR: Could not find .github/renovate.json5")
        sys.exit(1)

    # Find the kubernetes-components group's allowedVersions
    pattern = r"(groupName:\s*['\"]kubernetes-components['\"],[\s\S]*?allowedVersions:\s*)['\"]([^'\"]+)['\"]"
    current_match = re.search(pattern, renovate_content)

    if not current_match:
        print("ERROR: Could not find kubernetes-components allowedVersions in renovate.json5")
        sys.exit(1)

    current_constraint = current_match.group(2)
    print(f"Current constraint: {current_constraint}")

    # Check if already up to date
    if current_constraint == allowed_versions.strip("'\""):
        print("✓ Constraint is already up to date!")
        return False

    # Update the constraint
    new_content = re.sub(
        pattern,
        rf"\1'{allowed_versions}'",
        renovate_content
    )

    # Also update the comment that documents current support
    comment_pattern = r"(//\s*3\.\s*Current:.*?)Talos \d+\.\d+\.x supports Kubernetes.*"
    new_comment = f"Talos {talos_minor}.x supports Kubernetes {', '.join([f'1.{v}.x' for v in supported_k8s_versions])}"
    new_content = re.sub(
        comment_pattern,
        rf"\1{new_comment}",
        new_content
    )

    # Write updated renovate.json5
    with open('.github/renovate.json5', 'w') as f:
        f.write(new_content)

    print("✓ Updated renovate.json5")
    return True


def main():
    """Main entry point for the script."""

    # Get Talos version from CLI argument or extract from file
    if len(sys.argv) > 1:
        talos_minor = sys.argv[1]
        print(f"Using Talos version from argument: v{talos_minor}.x")

        # Validate format
        if not re.match(r'^\d+\.\d+$', talos_minor):
            print(f"ERROR: Invalid Talos version format: {talos_minor}")
            print("Expected format: X.Y (e.g., 1.12)")
            sys.exit(1)
    else:
        talos_minor = extract_talos_version()
        print(f"Current Talos version: v{talos_minor}.x")

    # Get supported K8s versions
    supported_k8s_versions = get_supported_k8s_versions(talos_minor)
    print(f"Supported Kubernetes versions: {supported_k8s_versions}")

    # Update renovate config
    changed = update_renovate_config(talos_minor, supported_k8s_versions)

    if changed:
        print(f"\nSuccess! Updated constraint to allow K8s versions: {', '.join([f'1.{v}.x' for v in supported_k8s_versions])}")
        sys.exit(0)
    else:
        print("\nNo changes needed - constraint already matches current Talos support matrix")
        sys.exit(2)


if __name__ == '__main__':
    main()
