// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/grafana/regexp"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/rules/rulestest"
	"github.com/prometheus/prometheus/util/junitxml"
)

// RulesUnitTest does unit testing of rules based on the unit testing files provided.
// More info about the file format can be found in the docs.
func RulesUnitTest(queryOpts promqltest.LazyLoaderOpts, p parser.Parser, runStrings []string, diffFlag, debug, ignoreUnknownFields bool, files ...string) int {
	return RulesUnitTestResult(io.Discard, queryOpts, p, runStrings, diffFlag, debug, ignoreUnknownFields, files...)
}

func RulesUnitTestResult(results io.Writer, queryOpts promqltest.LazyLoaderOpts, p parser.Parser, runStrings []string, diffFlag, debug, ignoreUnknownFields bool, files ...string) int {
	failed := false
	junit := &junitxml.JUnitXML{}

	var run *regexp.Regexp
	if runStrings != nil {
		run = regexp.MustCompile(strings.Join(runStrings, "|"))
	}

	for _, f := range files {
		fmt.Printf("Processing test file: %s\n", f)
		suite, errs := rulestest.Run(f, rulestest.Options{
			Parser:              p,
			QueryOpts:           queryOpts,
			Run:                 run,
			Diff:                diffFlag,
			Debug:               debug,
			IgnoreUnknownFields: ignoreUnknownFields,
			ErrorWriter:         os.Stderr,
		})
		// Keep YAML diagnostics compatible with the former package main types.
		if suite.ErrorCount > 0 {
			if yamlErr, ok := errors.AsType[*yaml.TypeError](errs[0]); ok {
				for i, msg := range yamlErr.Errors {
					typeOffset := max(strings.LastIndex(msg, " in type "), strings.LastIndex(msg, " into "))
					if typeOffset >= 0 {
						yamlErr.Errors[i] = msg[:typeOffset] + strings.ReplaceAll(msg[typeOffset:], "rulestest.", "main.")
					}
				}
				suite.Cases[0].Error = yamlErr.Error()
			}
		}
		junit.Suites = append(junit.Suites, suite)
		if errs != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
				fmt.Println()
			}
			failed = true
		} else {
			fmt.Println("  SUCCESS")
		}
		fmt.Println()
	}
	err := junit.WriteXML(results)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write JUnit XML: %s\n", err)
	}
	if failed {
		return failureExitCode
	}
	return successExitCode
}
