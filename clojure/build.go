/*
 * Copyright 2018-2021 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package clojure

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/paketo-buildpacks/libpak/effect"
	"github.com/paketo-buildpacks/libpak/sbom"

	"github.com/buildpacks/libcnb"
	"github.com/paketo-buildpacks/libbs"
	"github.com/paketo-buildpacks/libpak"
	"github.com/paketo-buildpacks/libpak/bard"
)

type Build struct {
	Logger             bard.Logger
	ApplicationFactory ApplicationFactory
}

type ApplicationFactory interface {
	NewApplication(additionalMetadata map[string]interface{}, arguments []string, artifactResolver libbs.ArtifactResolver,
		cache libbs.Cache, command string, bom *libcnb.BOM, applicationPath string, sBOMScanner sbom.SBOMScanner) (libbs.Application, error)
}

func (b Build) Build(context libcnb.BuildContext) (libcnb.BuildResult, error) {
	b.Logger.Title(context.Buildpack)
	result := libcnb.NewBuildResult()

	cr, err := libpak.NewConfigurationResolver(context.Buildpack, &b.Logger)
	if err != nil {
		return libcnb.BuildResult{}, fmt.Errorf("unable to create configuration resolver\n%w", err)
	}

	dr, err := libpak.NewDependencyResolver(context)
	if err != nil {
		return libcnb.BuildResult{}, fmt.Errorf("unable to create dependency resolver\n%w", err)
	}

	dc, err := libpak.NewDependencyCache(context)
	if err != nil {
		return libcnb.BuildResult{}, fmt.Errorf("unable to create dependency cache\n%w", err)
	}
	dc.Logger = b.Logger

	command := filepath.Join(context.Application.Path, "clojure")
	if _, err := os.Stat(command); os.IsNotExist(err) {
		dep, err := dr.Resolve("clojure", "")
		if err != nil {
			return libcnb.BuildResult{}, fmt.Errorf("unable to find dependency\n%w", err)
		}

		d, be := NewDistribution(dep, dc)
		d.Logger = b.Logger
		result.Layers = append(result.Layers, d)
		result.BOM.Entries = append(result.BOM.Entries, be)

		command = filepath.Join(context.Layers.Path, d.Name(), "bin", "clojure")
	} else if err != nil {
		return libcnb.BuildResult{}, fmt.Errorf("unable to stat %s\n%w", command, err)
	} else {
		if err := os.Chmod(command, 0755); err != nil {
			return libcnb.BuildResult{}, fmt.Errorf("unable to chmod %s\n%w", command, err)
		}
	}

	u, err := user.Current()
	if err != nil {
		return libcnb.BuildResult{}, fmt.Errorf("unable to determine user home directory\n%w", err)
	}

	c := libbs.Cache{Path: filepath.Join(u.HomeDir, ".m2")}
	c.Logger = b.Logger
	result.Layers = append(result.Layers, c)

	ednFileExists := fileExists(filepath.Join(context.Application.Path, "deps.edn"))
	toolsBuildEnabled, _ := libbs.ResolveArguments("BP_CLJ_TOOLS_BUILD_ENABLED", cr)
	var args []string
	if ednFileExists && strings.ToLower(toolsBuildEnabled[0]) == "true" {
		args, err = libbs.ResolveArguments("BP_CLJ_TOOLS_BUILD_ARGUMENTS", cr)
		if err != nil {
			return libcnb.BuildResult{}, fmt.Errorf("unable to resolve build arguments\n%w", err)
		}
	} else if ednFileExists {
		args, err = libbs.ResolveArguments("BP_CLJ_DEPS_ARGUMENTS", cr)
		if err != nil {
			return libcnb.BuildResult{}, fmt.Errorf("unable to resolve build arguments\n%w", err)
		}
	}

	art := libbs.ArtifactResolver{
		ArtifactConfigurationKey: "BP_CLJ_BUILT_ARTIFACT",
		ConfigurationResolver:    cr,
		ModuleConfigurationKey:   "BP_CLJ_BUILT_MODULE",
		InterestingFileDetector:  libbs.AlwaysInterestingFileDetector{},
	}

	sbomScanner := sbom.NewSyftCLISBOMScanner(context.Layers, effect.CommandExecutor{}, b.Logger)

	a, err := b.ApplicationFactory.NewApplication(
		map[string]interface{}{},
		args,
		art,
		c,
		command,
		result.BOM,
		context.Application.Path,
		sbomScanner,
	)
	if err != nil {
		return libcnb.BuildResult{}, fmt.Errorf("unable to create application layer\n%w", err)
	}
	a.Logger = b.Logger
	result.Layers = append(result.Layers, a)

	return result, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
