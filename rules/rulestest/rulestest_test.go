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

package rulestest_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/rules/rulestest"
)

func ExampleRun() {
	suite, errs := rulestest.Run("rules_test.yml", rulestest.Options{
		Parser: parser.NewParser(parser.Options{}),
		QueryOpts: promqltest.LazyLoaderOpts{
			EnableAtModifier:     true,
			EnableNegativeOffset: true,
		},
	})
	fmt.Printf("Ran %d tests\n", suite.TestCount)
	for _, err := range errs {
		fmt.Println(err)
	}
}

func TestRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules.yml"), []byte(`groups:
  - name: alerts
    rules:
      - alert: High
        expr: doubled > 5
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: 'value={{ $value }}'
  - name: records
    rules:
      - record: doubled
        expr: source * 2
`), 0o600))
	filename := filepath.Join(dir, "tests.yml")
	require.NoError(t, os.WriteFile(filename, []byte(`rule_files: ['rules*.yml', 'missing*.yml']
group_eval_order: [records, alerts]
tests:
  - name: passing
    start_timestamp: 1000.25
    input_series:
      - series: 'source{instance="a"}'
        values: '3x2'
    alert_rule_test:
      - eval_time: 1m
        alertname: High
        exp_alerts:
          - exp_labels: {instance: a, severity: warning}
            exp_annotations: {summary: 'value=6'}
    promql_expr_test:
      - expr: doubled
        eval_time: 1m
        exp_samples:
          - labels: 'doubled{instance="a"}'
            value: 6
      - expr: timestamp(source)
        eval_time: 1m
        exp_samples:
          - labels: '{instance="a"}'
            value: 1060.25
  - name: wrong-alert
    input_series:
      - series: 'source{instance="a"}'
        values: '3x2'
    alert_rule_test:
      - eval_time: 1m
        alertname: High
        exp_alerts: []
  - name: wrong-query
    promql_expr_test:
      - expr: vector(7)
        eval_time: 0m
        exp_samples:
          - value: 8
`), 0o600))

	for _, tc := range []struct {
		name     string
		run      *regexp.Regexp
		cases    int
		failures int
	}{
		{name: "all", cases: 3, failures: 2},
		{name: "passing", run: regexp.MustCompile("^passing$"), cases: 1},
		{name: "failures", run: regexp.MustCompile("^wrong-"), cases: 2, failures: 2},
		{name: "no matches", run: regexp.MustCompile("^absent$"), cases: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var diagnostics bytes.Buffer
			suite, errs := rulestest.Run(filename, rulestest.Options{
				Parser: parser.NewParser(parser.Options{}),
				QueryOpts: promqltest.LazyLoaderOpts{
					EnableAtModifier:     true,
					EnableNegativeOffset: true,
				},
				Run:         tc.run,
				Debug:       true,
				ErrorWriter: &diagnostics,
			})
			require.Equal(t, filename, suite.Name)
			require.Equal(t, tc.cases, suite.TestCount)
			require.Len(t, suite.Cases, tc.cases)
			require.Zero(t, suite.ErrorCount)
			require.Equal(t, tc.failures, suite.FailureCount)
			if tc.failures == 0 {
				require.Nil(t, errs)
			} else {
				require.Len(t, errs, tc.failures)
				require.Contains(t, errs[0].Error(), "alertname: High")
				require.Contains(t, errs[1].Error(), `expr: "vector(7)"`)
			}
			require.Contains(t, diagnostics.String(), "WARNING: no file match pattern "+filepath.Join(dir, "missing*.yml"))
			if tc.cases > 0 {
				require.Contains(t, diagnostics.String(), "DEBUG: Starting test")
				require.Contains(t, diagnostics.String(), "DEBUG: Dump of all data")
			}
		})
	}
}

func TestRunLoadingErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		yaml   string
		errMsg string
	}{
		{name: "missing file", errMsg: "tests.yml"},
		{name: "unknown field", yaml: "unknown: true", errMsg: "field unknown"},
		{name: "invalid glob", yaml: "rule_files: ['[']", errMsg: "syntax error in pattern"},
		{name: "duplicate order", yaml: "group_eval_order: [a, a]", errMsg: "group name repeated"},
		{name: "invalid timestamp", yaml: "tests: [{start_timestamp: yesterday}]", errMsg: `cannot parse "yesterday" to a valid timestamp`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			filename := filepath.Join(t.TempDir(), "tests.yml")
			if tc.yaml != "" {
				require.NoError(t, os.WriteFile(filename, []byte(tc.yaml), 0o600))
			}
			suite, errs := rulestest.Run(filename, rulestest.Options{Parser: parser.NewParser(parser.Options{})})
			require.Len(t, errs, 1)
			require.Contains(t, errs[0].Error(), tc.errMsg)
			require.Equal(t, 1, suite.ErrorCount)
			require.Zero(t, suite.FailureCount)
			require.Len(t, suite.Cases, 1)
			require.Equal(t, errs[0].Error(), suite.Cases[0].Error)
		})
	}
}
