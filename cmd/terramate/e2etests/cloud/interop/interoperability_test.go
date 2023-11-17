// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

//go:build interop

package interop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madlambda/spells/assert"
	. "github.com/terramate-io/terramate/cmd/terramate/e2etests/internal/runner"
)

func TestInteropSyncDeployment(t *testing.T) {
	tmcli := NewInteropCLI(t, datapath(t, "testdata/interop-stacks/empty"))
	AssertRunResult(t, tmcli.ListStacks(), RunExpected{
		Stdout: nljoin("."),
	})
	AssertRunResult(t,
		tmcli.Run("run", "--cloud-sync-deployment", "--", HelperPath, "false"),
		RunExpected{
			IgnoreStderr: true,
			Status:       1,
		},
	)
	AssertRunResult(t,
		tmcli.ListStacks("--experimental-status=unhealthy"), RunExpected{
			Stdout: nljoin("."),
		},
	)
	AssertRun(t, tmcli.Run("run", "--cloud-sync-deployment", "--", HelperPath, "true"))
	AssertRun(t, tmcli.ListStacks("--experimental-status=unhealthy"))
}

func TestInteropDrift(t *testing.T) {
	tmcli := NewInteropCLI(t, datapath(t, "testdata/interop-stacks/basic-drift"))
	AssertRunResult(t, tmcli.ListStacks(), RunExpected{
		Stdout: nljoin("."),
	})
	// inititialize the providers
	AssertRunResult(t,
		tmcli.Run("run", "--", TerraformTestPath, "init"),
		RunExpected{
			Status:       0,
			IgnoreStdout: true,
			IgnoreStderr: true,
		},
	)

	// basic drift, without details
	AssertRunResult(t,
		tmcli.Run("run", "--cloud-sync-drift-status", "--", TerraformTestPath, "plan", "-detailed-exitcode"),
		RunExpected{
			Status:       0,
			IgnoreStdout: true,
			IgnoreStderr: true,
		},
	)
	AssertRunResult(t,
		tmcli.ListStacks("--experimental-status=unhealthy"), RunExpected{
			Stdout: nljoin("."),
		},
	)
	// Check if there are no drift details
	AssertRunResult(t,
		tmcli.Run("experimental", "cloud", "drift", "show"), RunExpected{
			StderrRegex: "Stack .*? is drifted, but no details are available",
			Status:      1,
		},
	)

	// complete drift
	AssertRunResult(t,
		tmcli.Run(
			"run", "--cloud-sync-drift-status", "--cloud-sync-terraform-plan-file=out.plan", "--",
			TerraformTestPath, "plan", "-out=out.plan", "-detailed-exitcode",
		),
		RunExpected{
			Status:       0,
			IgnoreStdout: true,
			IgnoreStderr: true,
		},
	)
	AssertRunResult(t,
		tmcli.ListStacks("--experimental-status=unhealthy"), RunExpected{
			Stdout: nljoin("."),
		},
	)
	// Check the drift details
	AssertRunResult(t,
		tmcli.Run("experimental", "cloud", "drift", "show"), RunExpected{
			StdoutRegexes: []string{
				"hello world", // content of the file
				"local_file",  // name of the resource
			},
			Status: 0,
		},
	)

	// check reseting the drift status to OK
	AssertRun(t, tmcli.Run("run", "--cloud-sync-drift-status", "--", HelperPath, "exit", "0"))
	AssertRun(t, tmcli.ListStacks("--experimental-status=unhealthy"))
	AssertRunResult(t,
		tmcli.Run("experimental", "cloud", "drift", "show"), RunExpected{
			StdoutRegex: "Stack .*? is not drifted",
			Status:      0,
		},
	)
}

func datapath(t *testing.T, path string) string {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	return filepath.Join(wd, filepath.FromSlash(path))
}

func nljoin(stacks ...string) string {
	return strings.Join(stacks, "\n") + "\n"
}
