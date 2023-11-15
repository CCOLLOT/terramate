// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/terramate-io/terramate/cloud"
	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/git"
	"github.com/terramate-io/terramate/hcl"
	"github.com/terramate-io/terramate/stack"
)

type project struct {
	rootdir        string
	wd             string
	isRepo         bool
	root           config.Root
	baseRef        string
	normalizedRepo string

	git struct {
		wrapper                   *git.Git
		headCommit                string
		localDefaultBranchCommit  string
		remoteDefaultBranchCommit string

		repoChecks stack.RepoChecks
	}
}

func (p project) gitcfg() *hcl.GitConfig {
	return p.root.Tree().Node.Terramate.Config.Git
}

func (p *project) prettyRepo() string {
	if p.normalizedRepo != "" {
		return p.normalizedRepo
	}
	if p.isRepo {
		repoURL, err := p.git.wrapper.URL(p.gitcfg().DefaultRemote)
		if err == nil {
			p.normalizedRepo = cloud.NormalizeGitURI(repoURL)
		} else {
			logger := log.With().
				Str("action", "project.prettyRepo").
				Logger()

			logger.
				Warn().
				Err(err).
				Msg("failed to retrieve repository URL")
		}
	}
	return p.normalizedRepo
}

func (p *project) headCommit() string {
	if p.git.headCommit != "" {
		return p.git.headCommit
	}

	logger := log.With().
		Str("action", "headCommit()").
		Logger()

	val, err := p.git.wrapper.RevParse("HEAD")
	if err != nil {
		logger.Fatal().Err(err).Send()
	}

	p.git.headCommit = val
	return val
}

func (p *project) remoteDefaultCommit() string {
	if p.git.remoteDefaultBranchCommit != "" {
		return p.git.remoteDefaultBranchCommit
	}

	logger := log.With().
		Str("action", "remoteDefaultCommit()").
		Logger()

	gitcfg := p.gitcfg()
	remoteRef, err := p.git.wrapper.FetchRemoteRev(gitcfg.DefaultRemote, gitcfg.DefaultBranch)
	if err != nil {
		logger.Fatal().Err(
			fmt.Errorf("fetching remote commit of %s/%s: %v",
				gitcfg.DefaultRemote, gitcfg.DefaultBranch,
				err,
			)).Send()
	}

	p.git.remoteDefaultBranchCommit = remoteRef.CommitID
	return p.git.remoteDefaultBranchCommit
}

// defaultBaseRev returns the revision used for change comparison based on the current Git state.
func (p *project) defaultBaseRev() string {
	// Details:
	// Given origin/main is the default remote/branch, at commit C.
	// We assume C is the state that ran the last deployment. HEAD is at commit H.
	//
	// There's three scenarios, selected if one of the respective cases match, evaluated in order of definition.
	//
	//   - Pending changes should be compared to origin/main to find out what has changed since the last deployment.
	//
	//     Case 1: H != C and H is not an ancestor of C -- an undeployed, unmerged commit
	//     Case 2: H == C and symbolic-ref(HEAD) != main -- a new, yet empty branch (=> no changes yet)
	//
	//   - Deployed changes should be compared to the previous deployment to find out what changed.
	//     If we assume that every commit on the main branch is a deployment, that means compare to HEAD^.
	//
	//     Case 3: H == C -- latest main commit
	//     Case 4: H is a first-parent ancestor of main -- previous main commit
	//
	//   - Historic changes are all other non-deployed and non-pending, i.e. commits from an already merged and deployed branch.
	//     They should be compared to the fork point with origin/main.
	//
	//     Case 5: H has a fork point with origin/main -- a merged branch commit
	gitcfg := p.gitcfg()
	gw := p.git.wrapper

	remoteDefaultBranchRef := p.remoteDefaultBranchRef()
	headRev, _ := gw.RevParse("HEAD")
	remoteDefaultRev, _ := gw.RevParse(remoteDefaultBranchRef)

	isRemoteDefaultRev := headRev != "" && headRev == remoteDefaultRev

	isRemoteDefaultRevAncestor, _ := gw.IsAncestor("HEAD", remoteDefaultBranchRef)
	if !isRemoteDefaultRev && !isRemoteDefaultRevAncestor {
		// Case 1 (pending)
		return remoteDefaultBranchRef
	}

	branch, _ := gw.CurrentBranch()
	isDefaultBranch := branch != "" && branch == gitcfg.DefaultBranch
	isEmptyPendingBranch := isRemoteDefaultRev && !isDefaultBranch

	if isEmptyPendingBranch {
		// Case 2 (pending)
		return remoteDefaultBranchRef
	}

	if isRemoteDefaultRev {
		// Case 3 (deployed)
		return gitcfg.DefaultBranchBaseRef
	}

	isRemoteDefaultBranchAncestor, _ := gw.IsFirstParentAncestor(remoteDefaultBranchRef, "HEAD")
	if isRemoteDefaultBranchAncestor {
		// Case 4 (deployed)
		return gitcfg.DefaultBranchBaseRef
	}

	forkPoint, _ := gw.FindForkPoint(remoteDefaultBranchRef, "HEAD")
	if forkPoint != "" {
		// Case 5 (historic)
		return forkPoint
	}

	// Fallback to deployed strategy
	return gitcfg.DefaultBranchBaseRef
}

func (p project) remoteDefaultBranchRef() string {
	git := p.gitcfg()
	return git.DefaultRemote + "/" + git.DefaultBranch
}

func (p *project) setDefaults() error {
	logger := log.With().
		Str("action", "setDefaults()").
		Str("workingDir", p.wd).
		Logger()

	logger.Debug().Msg("Set defaults.")

	cfg := &p.root.Tree().Node
	if cfg.Terramate == nil {
		// if config has no terramate block we create one with default
		// configurations.
		cfg.Terramate = &hcl.Terramate{}
	}

	if cfg.Terramate.Config == nil {
		cfg.Terramate.Config = &hcl.RootConfig{}
	}
	// Now some defaults are defined on the NewGitConfig but others here.
	// To define all defaults here we would need boolean pointers to
	// check if the config is defined or not, the zero value for booleans
	// is valid (simpler with strings). Maybe we could move all defaults
	// to NewGitConfig.
	if cfg.Terramate.Config.Git == nil {
		cfg.Terramate.Config.Git = hcl.NewGitConfig()
	}

	gitOpt := cfg.Terramate.Config.Git

	if gitOpt.DefaultBranchBaseRef == "" {
		gitOpt.DefaultBranchBaseRef = defaultBranchBaseRef
	}

	if gitOpt.DefaultBranch == "" {
		gitOpt.DefaultBranch = defaultBranch
	}

	if gitOpt.DefaultRemote == "" {
		gitOpt.DefaultRemote = defaultRemote
	}

	return nil
}

func (p project) checkDefaultRemote() error {
	logger := log.With().
		Str("action", "checkDefaultRemote()").
		Logger()

	logger.Trace().Msg("Get list of configured git remotes.")

	remotes, err := p.git.wrapper.Remotes()
	if err != nil {
		return fmt.Errorf("checking if remote %q exists: %v", defaultRemote, err)
	}

	var defRemote *git.Remote

	gitcfg := p.gitcfg()

	logger.Trace().
		Msg("Find default git remote.")
	for _, remote := range remotes {
		if remote.Name == gitcfg.DefaultRemote {
			defRemote = &remote
			break
		}
	}

	if defRemote == nil {
		return fmt.Errorf("repository must have a configured %q remote",
			gitcfg.DefaultRemote,
		)
	}

	logger.Trace().Msg("Find default git branch.")
	for _, branch := range defRemote.Branches {
		if branch == gitcfg.DefaultBranch {
			return nil
		}
	}

	return fmt.Errorf("remote %q has no default branch %q,branches:%v",
		gitcfg.DefaultRemote,
		gitcfg.DefaultBranch,
		defRemote.Branches,
	)
}

func (p *project) checkRemoteDefaultBranchIsReachable() error {
	gitcfg := p.gitcfg()

	remoteDesc := fmt.Sprintf("remote(%s/%s)", gitcfg.DefaultRemote, gitcfg.DefaultBranch)

	logger := log.With().
		Str("head_hash", p.headCommit()).
		Str("default_branch", remoteDesc).
		Str("default_hash", p.remoteDefaultCommit()).
		Logger()

	outOfDateErr := errors.E(
		ErrCurrentHeadIsOutOfDate,
		"Please update the current branch with the latest changes from the default branch.",
	)

	mergeBaseCommitID, err := p.git.wrapper.MergeBase(p.headCommit(), p.remoteDefaultCommit())
	if err != nil {
		logger.Debug().
			Msg("A common merge-base can not be determined between HEAD and default branch")
		return outOfDateErr
	}

	if mergeBaseCommitID != p.remoteDefaultCommit() {
		logger.Debug().
			Str("merge_base_hash", mergeBaseCommitID).
			Msg("The default branch is not equal to the common merge-base of HEAD")
		return outOfDateErr
	}

	return nil
}
