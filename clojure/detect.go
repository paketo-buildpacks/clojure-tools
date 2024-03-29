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
	"path/filepath"
	
	"github.com/buildpacks/libcnb"
	"github.com/paketo-buildpacks/libpak/bard"
)

const (
	PlanEntryClj                   = "clojure"
	PlanEntryJVMApplicationPackage = "jvm-application-package"
	PlanEntryJDK                   = "jdk"
	PlanEntrySyft                  = "syft"
)

type Detect struct{
	Logger bard.Logger
}

func (d Detect) Detect(context libcnb.DetectContext) (libcnb.DetectResult, error) {
	file := filepath.Join(context.Application.Path, "deps.edn")

	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		d.Logger.Info("SKIPPED: no 'deps.edn' file found in the application path")
		return libcnb.DetectResult{Pass: false}, nil
	} else if err != nil {
		return libcnb.DetectResult{}, fmt.Errorf("unable to determine if %s exists\n%w", file, err)
	}

	return libcnb.DetectResult{
		Pass: true,
		Plans: []libcnb.BuildPlan{
			{
				Provides: []libcnb.BuildPlanProvide{
					{Name: PlanEntryClj},
					{Name: PlanEntryJVMApplicationPackage},
				},
				Requires: []libcnb.BuildPlanRequire{
					{Name: PlanEntrySyft},
					{Name: PlanEntryClj},
					{Name: PlanEntryJDK},
				},
			},
		},
	}, nil
}
