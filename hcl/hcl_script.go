// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package hcl

import (
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/hcl/ast"
)

// Errors returned during the HCL parsing of script block
const (
	ErrScriptNoAttrs      errors.Kind = "terramate schema error: (script): no attributes defined"
	ErrScriptNoBlocks     errors.Kind = "terramate schema error: (script): no blocks defined"
	ErrScriptEmptyLabel   errors.Kind = "terramate schema error: (script): must provide labels"
	ErrScriptTwoLabels    errors.Kind = "terramate schema error: (script): must provide exactly two labels"
	ErrScriptInvalidDesc  errors.Kind = "terramate schema error: (script): invalid description"
	ErrScriptUnknownAttr  errors.Kind = "terramate schema error: (script): unknown attribute"
	ErrScriptUnknownBlock errors.Kind = "terramate schema error: (script): unknown block"
	ErrScriptInvalidJob   errors.Kind = "terramate schema error: (script): invalid job"
	ErrScriptInvalidCmd   errors.Kind = "terramate schema error: (script): invalid command"
	ErrScriptInvalidCmds  errors.Kind = "terramate schema error: (script): invalid commands"
	ErrScriptCmdConflict  errors.Kind = "terramate schema error: (script): command and commands both set"
)

// Command represents an executable command
type Command ast.Attribute

// Commands represents a list of executable commands
type Commands [][]string

// ScriptJob represent a Job within a Script
type ScriptJob struct {
	Command  *Command // Command is a single executable command
	Commands Commands // Commands is a list of executable commands
}

// Script represents a parsed script block
type Script struct {
	Labels      []string      // Labels of the script block used for grouping scripts
	Description ast.Attribute // Description is a human readable description of a script
	Jobs        []*ScriptJob  // Job represents the command(s) part of this script
}

func (p *TerramateParser) parseScriptBlock(block *ast.Block) (*Script, error) {
	errs := errors.L()

	if len(block.Labels) != 2 {
		errs.Append(errors.E(ErrScriptTwoLabels, block.OpenBraceRange))
	} else if block.Labels[0] == "" {
		errs.Append(errors.E(ErrScriptEmptyLabel, block.OpenBraceRange))
	}

	parsedScript := &Script{
		Labels: block.Labels,
	}

	if len(block.Attributes) == 0 {
		errs.Append(errors.E(ErrScriptNoAttrs, block.OpenBraceRange))
	}

	for _, attr := range block.Attributes {
		switch attr.Name {
		case "description":
			desc, err := p.validateDescription(attr)
			if err != nil {
				errs.Append(errors.E(ErrScriptInvalidDesc, attr.NameRange))
				continue
			}
			parsedScript.Description = desc
		default:
			errs.Append(errors.E(ErrScriptUnknownAttr, attr.NameRange))
		}
	}

	if len(block.Blocks) < 1 {
		errs.Append(errors.E(ErrScriptNoBlocks, block.TypeRange))
	}

	for _, nestedBlock := range block.Blocks {
		switch nestedBlock.Type {
		case "job":
			parsedJobBlock, err := validateScriptJobBlock(nestedBlock)
			if err != nil {
				errs.Append(err)
			}
			parsedScript.Jobs = append(parsedScript.Jobs, parsedJobBlock)
		default:
			errs.Append(errors.E(ErrScriptUnknownBlock, nestedBlock.TypeRange, nestedBlock.Type))

		}
	}

	if err := errs.AsError(); err != nil {
		return nil, err
	}

	return parsedScript, nil

}

func (p *TerramateParser) validateDescription(attr ast.Attribute) (ast.Attribute, error) {
	errs := errors.L()
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		errs.Append(diags)
		return ast.Attribute{}, errs.AsError()
	}

	if val.AsString() == "" {
		errs.Append(errors.E("empty description"))
		return ast.Attribute{}, errs.AsError()
	}

	return attr, nil
}

func validateScriptJobBlock(block *ast.Block) (*ScriptJob, error) {
	errs := errors.L()

	var foundCmd, foundCmds bool
	parsedScriptJob := &ScriptJob{}
	for _, attr := range block.Attributes {
		switch attr.Name {
		case "command":
			cmdAttr, err := validateCommand(attr)
			if err != nil {
				errs.Append(errors.E(ErrScriptInvalidCmd, attr.NameRange, attr.Name))
				continue
			}
			parsedScriptJob.Command = cmdAttr
			foundCmd = true
		case "commands":
			parsedCmds, err := validateCommands(attr)
			if err != nil {
				errs.Append(errors.E(ErrScriptInvalidCmds, attr.NameRange, attr.Name))
				continue
			}
			parsedScriptJob.Commands = parsedCmds
			foundCmds = true
		default:
			errs.Append(errors.E(ErrScriptUnknownAttr, attr.NameRange, attr.Name))

		}
	}

	// job.command and job.commands are mutually exclusive
	if foundCmd && foundCmds {
		errs.Append(errors.E(ErrScriptCmdConflict, block.TypeRange))
	}

	if err := errs.AsError(); err != nil {
		return nil, err
	}

	return parsedScriptJob, nil
}

// validateCommand validates the provided script job block, parses the attribute
// into Command and returns an error if validation fails
func validateCommand(cmdAttr ast.Attribute) (*Command, error) {
	errs := errors.L()
	val, diags := cmdAttr.Attribute.Expr.Value(nil)
	if diags.HasErrors() {
		errs.Append(diags)
	}

	_, err := ValueAsStringList(val)
	if err != nil {
		errs.Append(err)
	}

	if err := errs.AsError(); err != nil {
		return nil, err
	}

	parsed := Command(cmdAttr)

	return &parsed, nil

}

// validateCommands validates the provided cmdsAttr, parses the attribute into
// Commands and returns an error if validation fails
func validateCommands(cmdsAttr ast.Attribute) (Commands, error) {
	errs := errors.L()
	val, diags := cmdsAttr.Attribute.Expr.Value(nil)
	if diags.HasErrors() {
		errs.Append(diags)
	}

	if !val.Type().IsTupleType() {
		errs.Append(errors.E("wrong type"))
	}

	valSlice := val.AsValueSlice()
	cmds := make([][]string, 0, len(valSlice))
	for _, val := range valSlice {
		parsedCmds, err := ValueAsStringList(val)
		if err != nil {
			errs.Append(err)
		}
		cmds = append(cmds, parsedCmds)
	}

	if err := errs.AsError(); err != nil {
		return nil, err
	}

	return Commands(cmds), nil

}
