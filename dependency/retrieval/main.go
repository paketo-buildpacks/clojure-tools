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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/paketo-buildpacks/libdependency/retrieve"
	"github.com/paketo-buildpacks/libdependency/upstream"
	"github.com/paketo-buildpacks/libdependency/versionology"
	"github.com/paketo-buildpacks/packit/v2/cargo"
)

const (
	id   = "clojure"
	name = "Clojure"

	org  = "clojure"
	repo = "brew-install"
)

var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

type clojureVersion struct {
	version *semver.Version
	branch  string
}

func (v clojureVersion) Version() *semver.Version {
	return v.version
}

func main() {
	retrieve.NewMetadata(id, getAllVersions, generateMetadata)
}

func getAllVersions() (versionology.VersionFetcherArray, error) {
	branches, err := fetchBranches(org, repo)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch branches\n%w", err)
	}

	var versions versionology.VersionFetcherArray
	for _, branch := range branches {
		if !versionPattern.MatchString(branch) {
			continue
		}

		v, err := semver.NewVersion(branch)
		if err != nil {
			fmt.Printf("Skipping %s: unable to parse version\n", branch)
			continue
		}

		versions = append(versions, clojureVersion{version: v, branch: branch})
	}

	return versions, nil
}

func generateMetadata(versionFetcher versionology.VersionFetcher) ([]versionology.Dependency, error) {
	version, ok := versionFetcher.(clojureVersion)
	if !ok {
		return nil, fmt.Errorf("unexpected version type %T", versionFetcher)
	}

	versionString := version.version.String()

	installerVersion, err := fetchStableVersion(version.branch)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch stable version for %s\n%w", versionString, err)
	}

	if !strings.HasPrefix(installerVersion, versionString+".") {
		fmt.Printf("Skipping %s: stable installer version %s does not match\n", versionString, installerVersion)
		return nil, nil
	}

	uri := fmt.Sprintf("https://download.clojure.org/install/linux-install-%s.sh", installerVersion)
	checksum, err := upstream.GetSHA256OfRemoteFile(uri)
	if err != nil {
		return nil, fmt.Errorf("unable to checksum %s\n%w", uri, err)
	}

	source := fmt.Sprintf("https://github.com/clojure/clojure/archive/refs/tags/clojure-%s.tar.gz", version.branch)
	sourceChecksum, err := upstream.GetSHA256OfRemoteFile(source)
	if err != nil {
		return nil, fmt.Errorf("unable to checksum %s\n%w", source, err)
	}

	dependency := cargo.ConfigMetadataDependency{
		Checksum: fmt.Sprintf("sha256:%s", checksum),
		CPE:      fmt.Sprintf("cpe:2.3:a:cognitect:clojure:%s:*:*:*:*:*:*:*", versionString),
		ID:       id,
		Licenses: []interface{}{
			map[string]string{
				"type": "Eclipse Public License - v 1.0",
				"uri":  "https://github.com/clojure/clojure/blob/master/epl-v10.html",
			},
		},
		Name:           name,
		PURL:           retrieve.GeneratePURL(id, versionString, checksum, uri),
		Source:         source,
		SourceChecksum: fmt.Sprintf("sha256:%s", sourceChecksum),
		Stacks:         []string{"io.buildpacks.stacks.bionic", "io.paketo.stacks.tiny", "*"},
		URI:            uri,
		Version:        versionString,
	}

	return versionology.NewDependencyArray(dependency, "")
}

type githubBranch struct {
	Name string `json:"name"`
}

func fetchBranches(owner, repo string) ([]string, error) {
	var branches []string
	for page := 1; ; page++ {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/branches?page=%d&per_page=100", owner, repo, page)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		pageBranches, err := decodeBranches(resp)
		if err != nil {
			return nil, err
		}

		if len(pageBranches) == 0 {
			break
		}

		for _, b := range pageBranches {
			branches = append(branches, b.Name)
		}
	}

	return branches, nil
}

func decodeBranches(resp *http.Response) ([]githubBranch, error) {
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned status %d", resp.StatusCode)
	}

	var branches []githubBranch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, err
	}

	return branches, nil
}

func fetchStableVersion(branch string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/clojure/brew-install/%s/stable.properties", branch)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("unable to download %s: %w", url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

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
