// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

// helper is a utility command that implements behaviors that are
// useful when testing terramate run features in a way that reduces
// dependencies on the environment to run the tests.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-json/sanitize"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("%s requires at least one subcommand argument", os.Args[0])
	}

	// note: unrecovered panic() aborts the program with exit code 2 and this
	// could be confused with a *detected drift* (see: run --cloud-sync-drift-status)
	// then avoid panics here and do proper os.Exit(1) in case of errors.

	switch os.Args[1] {
	case "echo":
		args := os.Args[2:]
		for i, arg := range args {
			fmt.Print(arg)
			if i+1 < len(args) {
				fmt.Print(" ")
			}
		}
		fmt.Print("\n")
	case "true":
		os.Exit(0)
	case "false":
		os.Exit(1)
	case "exit":
		exit(os.Args[2])
	case "hang":
		hang()
	case "sleep":
		sleep(os.Args[2])
	case "env":
		env()
	case "cat":
		cat(os.Args[2])
	case "rm":
		rm(os.Args[2])
	case "tempdir":
		tempDir()
	case "stack-abs-path":
		stackAbsPath(os.Args[2])
	case "tf-plan-sanitize":
		tfPlanSanitize(os.Args[2])
	default:
		log.Fatalf("unknown command %s", os.Args[1])
	}
}

// hang will hang the process forever, ignoring any signals.
// It is useful to validate forced kill behavior.
// It will print "ready" when it starts to receive the signals.
// It will print the name of the received signals, which may also be useful in testing.
func hang() {
	signals := make(chan os.Signal, 10)
	signal.Notify(signals)

	fmt.Println("ready")

	for s := range signals {
		fmt.Println(s)
	}
}

// sleep put the test process to sleep.
func sleep(durationStr string) {
	d, err := time.ParseDuration(durationStr)
	checkerr(err)
	fmt.Println("ready")
	time.Sleep(d)
}

// exit with the provided exitCode.
func exit(exitCodeStr string) {
	code, err := strconv.Atoi(exitCodeStr)
	checkerr(err)
	os.Exit(code)
}

// env sends os.Environ() on stdout and exits.
func env() {
	for _, env := range os.Environ() {
		fmt.Println(env)
	}
}

// cat the file contents to stdout.
func cat(fname string) {
	bytes, err := os.ReadFile(fname)
	checkerr(err)
	fmt.Printf("%s", string(bytes))
}

// rm remove the given path.
func rm(fname string) {
	err := os.RemoveAll(fname)
	checkerr(err)
}

// tempdir creates a temporary directory.
func tempDir() {
	tmpdir, err := os.MkdirTemp("", "tm-tmpdir")
	checkerr(err)
	fmt.Print(tmpdir)
}

func stackAbsPath(base string) {
	cwd, err := os.Getwd()
	checkerr(err)
	rel, err := filepath.Rel(base, cwd)
	checkerr(err)
	fmt.Println("/" + filepath.ToSlash(rel))
}

func tfPlanSanitize(fname string) {
	var oldPlan tfjson.Plan
	oldPlanData, err := os.ReadFile(fname)
	checkerr(err)
	err = json.Unmarshal(oldPlanData, &oldPlan)
	checkerr(err)
	newPlan, err := sanitize.SanitizePlan(&oldPlan)
	checkerr(err)
	newPlanData, err := json.Marshal(newPlan)
	checkerr(err)
	fmt.Print(string(newPlanData))
}

func checkerr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
