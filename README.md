# Paketo Buildpack for Clojure Tools

The Paketo Clojure Tools Buildpack is a Cloud Native Buildpack that builds Clojure-based applications from source.

## Behavior
This buildpack will participate all the following conditions are met

* `<APPLICATION_ROOT>/build.clj` exists
* `<APPLICATION_ROOT>/deps.edn` exists

The buildpack will do the following:

* Requests that a JDK be installed
* Links the `~/.m2` to a layer for caching
* If `<APPLICATION_ROOT>/build.clj` does not exist
  * Contributes Clojure Tools to a layer with all commands on `$PATH`
  * Runs `<CLOJURE_TOOLS_ROOT>/clojure -T:build uber` to build the application
* If `<APPLICATION_ROOT>/deps.edn` does not exist
  * Contributes Clojure Tools to a layer with all commands on `$PATH`
  * Runs `<CLOJURE_TOOLS_ROOT>/clojure -X:uberjar` to build the application
* Removes the source code in `<APPLICATION_ROOT>`
* Expands `<APPLICATION_ROOT>/target/*.jar` to `<APPLICATION_ROOT>`

## Configuration
| Environment Variable | Description
| -------------------- | -----------
| `$BP_CLJ_TOOLS_BUILD_ARGUMENTS` | Configure the arguments to pass to build system.  Defaults to `-T:build uber`.
| `$BP_CLJ_DEPS_ARGUMENTS` | Configure the arguments to pass to build system.  Defaults to `-X:uberjar`.
| `$BP_CLJ_BUILT_MODULE` | Configure the module to find application artifact in.  Defaults to the root module (empty).
| `$BP_CLJ_BUILT_ARTIFACT` | Configure the built application artifact explicitly.  Supersedes `$BP_CLJ_BUILT_MODULE`  Defaults to `target/*.jar`.

