// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package hcl_test

import (
	"testing"

	hhcl "github.com/hashicorp/hcl/v2"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/hcl"
	"github.com/terramate-io/terramate/hcl/ast"
	"github.com/terramate-io/terramate/test"
	. "github.com/terramate-io/terramate/test/hclutils"
)

func TestHCLScript(t *testing.T) {
	makeAttribute := func(t *testing.T, name string, expr string) ast.Attribute {
		t.Helper()
		return ast.Attribute{
			Attribute: &hhcl.Attribute{
				Name: name,
				Expr: test.NewExpr(t, expr),
			},
		}
	}

	makeCommand := func(t *testing.T, name, expr string) *hcl.Command {
		cmd := hcl.Command(makeAttribute(t, "command", expr))
		return &cmd
	}

	for _, tc := range []testcase{
		{
			name: "script with unrecognized blocks",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						    description = "some desc"
							block1 {}
							block2 {}
						}
					`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptUnknownBlock,
						Mkrange("script.tm", Start(5, 8, 95), End(5, 14, 101))),
					errors.E(hcl.ErrScriptUnknownBlock,
						Mkrange("script.tm", Start(5, 8, 95), End(5, 14, 101))),
				},
			},
		},
		{
			name: "script without a description attr",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						}
					`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptNoAttrs,
						Mkrange("script.tm", Start(2, 33, 33), End(2, 34, 34))),
					errors.E(hcl.ErrScriptNoBlocks,
						Mkrange("script.tm", Start(2, 7, 7), End(2, 13, 13))),
				},
			},
		},
		{
			name: "script with an empty description attr",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = ""
						}
					`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptInvalidDesc,
						Mkrange("script.tm", Start(3, 9, 43), End(3, 20, 54))),
					errors.E(hcl.ErrScriptNoBlocks,
						Mkrange("script.tm", Start(2, 7, 7), End(2, 13, 13))),
				},
			},
		},
		{
			name: "script with a description attr",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = "some description"
						}
					`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptNoBlocks,
						Mkrange("script.tm", Start(2, 7, 7), End(2, 13, 13))),
				},
			},
		},
		{
			name: "script with an unknown attr",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = "some description"
						  unknownattr = "abc"
						  job {
						    command = ["ls"]
						  }
						}
					`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptUnknownAttr,
						Mkrange("script.tm", Start(4, 9, 84), End(4, 20, 95))),
				},
			},
		},
		{
			name: "script with a description attr and job command",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = "some description"
						  job {
							command = ["echo", "hello"]
						  }
						}
					`,
				},
			},
			want: want{
				errs: nil,
				config: hcl.Config{
					Scripts: []*hcl.Script{
						{
							Labels:      []string{"group1", "script1"},
							Description: makeAttribute(t, "description", `"some description"`),
							Jobs: []*hcl.ScriptJob{
								{
									Command: makeCommand(t, "command", `["echo", "hello"]`),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "script with a description attr and job commands",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = "some description"
						  job {
							commands = [
							  ["echo", "hello"],
							  ["echo", "bye"],
							]
						  }
						}
					`,
				},
			},
			want: want{
				errs: nil,
				config: hcl.Config{
					Scripts: []*hcl.Script{
						{
							Labels:      []string{"group1", "script1"},
							Description: makeAttribute(t, "description", `"some description"`),
							Jobs: []*hcl.ScriptJob{
								{
									Commands: [][]string{
										{"echo", "hello"},
										{"echo", "bye"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "script with job command and commands",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
					  script "group1" "script1" {
						description = "some description"
						job {
						  command = ["ls", "-l"]
						  commands = [
							["echo", "hello"],
							["echo", "bye"],
						  ]
						}
					  }
		`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptCmdConflict,
						Mkrange("script.tm", Start(4, 7, 81), End(4, 10, 84))),
				},
			},
		},
		{
			name: "script with unknown job attrs",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
					  script "group1" "script1" {
						description = "some description"
						job {
						  command = ["ls", "-l"]
						  unknownattr = "abc"
						}
					  }
		`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptUnknownAttr,
						Mkrange("script.tm", Start(6, 9, 126), End(6, 20, 137))),
				},
			},
		},
		{
			name: "script with invalid command",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = "some description"
						  job {
							command = ["ls", 1, "-l"]
						  }
						}
					`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptInvalidCmd,
						Mkrange("script.tm", Start(5, 8, 97), End(5, 15, 104))),
				},
			},
		},
		{
			name: "script with invalid commands",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = "some description"
						  job {
							commands = [
							  ["ls"],
							  ["ls", 123],
							]
						  }
						}
					`,
				},
			},
			want: want{
				errs: []error{
					errors.E(hcl.ErrScriptInvalidCmds,
						Mkrange("script.tm", Start(5, 8, 97), End(5, 16, 105))),
				},
			},
		},
		{
			name: "script with multiple jobs",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = "some description"
						  job {
							commands = [
							  ["echo", "hello"],
							  ["echo", "bye"],
							]
						  }
						  job {
							commands = [
							  ["ls", "-l"],
							  ["date"],
							]
						  }
						  job {
							command = ["stat", "."]
						  }
						}
					`,
				},
			},
			want: want{
				errs: nil,
				config: hcl.Config{
					Scripts: []*hcl.Script{
						{
							Labels:      []string{"group1", "script1"},
							Description: makeAttribute(t, "description", `"some description"`),
							Jobs: []*hcl.ScriptJob{
								{
									Commands: [][]string{
										{"echo", "hello"},
										{"echo", "bye"},
									},
								},
								{
									Commands: [][]string{
										{"ls", "-l"},
										{"date"},
									},
								},
								{
									Command: makeCommand(t, "command", `["stat", "."]`),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple scripts",
			input: []cfgfile{
				{
					filename: "script.tm",
					body: `
						script "group1" "script1" {
						  description = "script1 desc"
						  job {
							commands = [
							  ["echo", "hello"],
							  ["echo", "bye"],
							]
						  }
						}

						script "group1" "script2" {
						  description = "script2 desc"
						  job {
							commands = [
							  ["cat", "main.tf"],
							]
						  }
						}
					`,
				},
			},
			want: want{
				errs: nil,
				config: hcl.Config{
					Scripts: []*hcl.Script{
						{
							Labels:      []string{"group1", "script1"},
							Description: makeAttribute(t, "description", `"script1 desc"`),
							Jobs: []*hcl.ScriptJob{
								{
									Commands: [][]string{
										{"echo", "hello"},
										{"echo", "bye"},
									},
								},
							},
						},
						{
							Labels:      []string{"group1", "script2"},
							Description: makeAttribute(t, "description", `"script2 desc"`),
							Jobs: []*hcl.ScriptJob{
								{
									Commands: [][]string{
										{"cat", "main.tf"},
									},
								},
							},
						},
					},
				},
			},
		},
	} {
		testParser(t, tc)
	}
}
