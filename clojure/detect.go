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
)

const (
	PlanEntryClj                   = "clojure"
	PlanEntryJVMApplicationPackage = "jvm-application-package"
	PlanEntryJDK                   = "jdk"
)

type Detect struct{}

func (Detect) Detect(context libcnb.DetectContext) (libcnb.DetectResult, error) {
	files := []string{
		filepath.Join(context.Application.Path, "deps.edn"),
		filepath.Join(context.Application.Path, "build.clj"),
	}

	for _, file := range files {
		_, err := os.Stat(file)
		if os.IsNotExist(err) {
			continue
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
						{Name: PlanEntryClj},
						{Name: PlanEntryJDK},
					},
				},
			},
		}, nil
	}

	return libcnb.DetectResult{Pass: false}, nil
}
