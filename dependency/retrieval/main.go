// Copyright 2018-2026 the original author or authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/semver/v3"
	"github.com/paketo-buildpacks/packit/v2/cargo"
)

var httpClient = &http.Client{}

func main() {
	var buildpackTomlPath, outputPath string
	flag.StringVar(&buildpackTomlPath, "buildpack-toml-path", "", "Path to buildpack.toml")
	flag.StringVar(&outputPath, "output", "", "Path to output metadata.json")
	flag.Parse()

	if buildpackTomlPath == "" || outputPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s --buildpack-toml-path <path> --output <path>\n", os.Args[0])
		os.Exit(1)
	}

	// Load buildpack.toml
	file, err := os.Open(buildpackTomlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening buildpack.toml: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var config cargo.Config
	if _, err := toml.NewDecoder(file).Decode(&config); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing buildpack.toml: %v\n", err)
		os.Exit(1)
	}

	// Get constraints for clojure
	var constraints []cargo.ConfigMetadataDependencyConstraint
	for _, c := range config.Metadata.DependencyConstraints {
		if c.ID == "clojure" {
			constraints = append(constraints, c)
		}
	}

	// Get maximum existing version
	var maxExistingVersion *semver.Version
	for _, dep := range config.Metadata.Dependencies {
		if dep.ID == "clojure" {
			v, err := semver.NewVersion(dep.Version)
			if err == nil {
				if maxExistingVersion == nil || v.GreaterThan(maxExistingVersion) {
					maxExistingVersion = v
				}
			}
		}
	}

	// Fetch version branches from the brew tap repo. Each Clojure release has
	// a branch named after its version (e.g. 1.12.6) whose stable.properties
	// file pins the installer version (e.g. 1.12.6.1673).
	branches, err := fetchBranches("clojure", "brew-install")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching branches: %v\n", err)
		os.Exit(1)
	}

	type candidate struct {
		branch  string
		version *semver.Version
	}
	var candidates []candidate
	versionPattern := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	for _, branch := range branches {
		if !versionPattern.MatchString(branch) {
			continue
		}
		v, err := semver.NewVersion(branch)
		if err != nil {
			fmt.Printf("Skipping %s: unable to parse version\n", branch)
			continue
		}
		if maxExistingVersion != nil && !v.GreaterThan(maxExistingVersion) {
			fmt.Printf("Skipping %s: not newer than max existing version %s\n", v.String(), maxExistingVersion.String())
			continue
		}
		if !matchesConstraints(v, constraints) {
			continue
		}
		candidates = append(candidates, candidate{branch: branch, version: v})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].version.LessThan(candidates[j].version)
	})

	var output []OutputMetadata

	for _, c := range candidates {
		versionStr := c.version.String()

		// The stable.properties file on the version branch pins the installer
		// version (e.g. 1.12.6.1673) for a given Clojure version
		installerVersion, err := fetchStableVersion(c.branch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", versionStr, err)
			continue
		}

		if !strings.HasPrefix(installerVersion, versionStr+".") {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: stable installer version %s does not match\n", versionStr, installerVersion)
			continue
		}

		uri := fmt.Sprintf("https://download.clojure.org/install/linux-install-%s.sh", installerVersion)

		// Compute checksums
		fmt.Printf("Processing version %s...\n", versionStr)
		checksum, err := computeChecksum(uri)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to checksum uri for %s: %v\n", versionStr, err)
			continue
		}

		sourceURL := fmt.Sprintf("https://github.com/clojure/clojure/archive/refs/tags/clojure-%s.tar.gz", c.branch)
		sourceChecksum, err := computeChecksum(sourceURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to checksum source for %s: %v\n", versionStr, err)
			continue
		}

		cpe := fmt.Sprintf("cpe:2.3:a:cognitect:clojure:%s:*:*:*:*:*:*:*", versionStr)
		purl := fmt.Sprintf("pkg:generic/clojure@%s?arch=amd64", versionStr)

		licenses := []map[string]string{
			{
				"type": "Eclipse Public License - v 1.0",
				"uri":  "https://github.com/clojure/clojure/blob/master/epl-v10.html",
			},
		}

		output = append(output, OutputMetadata{
			ID:             "clojure",
			Name:           "Clojure",
			Version:        versionStr,
			URI:            uri,
			Checksum:       "sha256:" + checksum,
			Source:         sourceURL,
			SourceChecksum: "sha256:" + sourceChecksum,
			CPE:            cpe,
			PURL:           purl,
			Licenses:       licenses,
			Stacks:         []string{"io.buildpacks.stacks.bionic", "io.paketo.stacks.tiny", "*"},
		})
	}

	// Write output
	outFile, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	encoder := json.NewEncoder(outFile)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully wrote %d dependency entries to %s\n", len(output), outputPath)
}

type OutputMetadata struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Version        string              `json:"version"`
	URI            string              `json:"uri"`
	Checksum       string              `json:"checksum"`
	Source         string              `json:"source,omitempty"`
	SourceChecksum string              `json:"source-checksum,omitempty"`
	CPE            string              `json:"cpe,omitempty"`
	PURL           string              `json:"purl,omitempty"`
	Licenses       []map[string]string `json:"licenses,omitempty"`
	Stacks         []string            `json:"stacks,omitempty"`
}

type GitHubBranch struct {
	Name string `json:"name"`
}

func fetchBranches(owner, repo string) ([]string, error) {
	var branches []string
	page := 1
	for {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/branches?page=%d&per_page=100", owner, repo, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
		}

		var pageBranches []GitHubBranch
		if err := json.NewDecoder(resp.Body).Decode(&pageBranches); err != nil {
			return nil, err
		}

		if len(pageBranches) == 0 {
			break
		}

		for _, b := range pageBranches {
			branches = append(branches, b.Name)
		}

		page++
	}

	return branches, nil
}

func fetchStableVersion(branch string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/clojure/brew-install/%s/stable.properties", branch)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("unable to download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unable to download %s: status %d", url, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read %s: %w", url, err)
	}

	parts := strings.Fields(string(b))
	if len(parts) == 0 {
		return "", fmt.Errorf("unable to parse stable version from %s", url)
	}

	return parts[0], nil
}

func computeChecksum(uri string) (string, error) {
	resp, err := httpClient.Get(uri)
	if err != nil {
		return "", fmt.Errorf("unable to download %s: %w", uri, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unable to download %s: status %d", uri, resp.StatusCode)
	}

	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return "", fmt.Errorf("unable to read %s: %w", uri, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func matchesConstraints(v *semver.Version, constraints []cargo.ConfigMetadataDependencyConstraint) bool {
	if len(constraints) == 0 {
		return true
	}
	for _, c := range constraints {
		cstr, err := semver.NewConstraint(c.Constraint)
		if err != nil {
			continue
		}
		if cstr.Check(v) {
			return true
		}
	}
	return false
}
