// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"testing"

	"github.com/terramate-io/terramate/test/sandbox"
)

func TestE2EListNonGit(t *testing.T) {
	t.Parallel()
	for _, tcase := range listTestcases() {
		tc := tcase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := sandbox.NoGit(t, true)
			s.BuildTree(tc.layout)

			cli := newCLI(t, s.RootDir())
			var args []string
			for _, filter := range tc.filterTags {
				args = append(args, "--tags", filter)
			}
			for _, filter := range tc.filterNoTags {
				args = append(args, "--no-tags", filter)
			}
			assertRunResult(t, cli.listStacks(args...), tc.want)
		})
	}
}
