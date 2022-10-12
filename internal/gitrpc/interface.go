// Copyright 2022 Harness Inc. All rights reserved.
// Use of this source code is governed by the Polyform Free Trial License
// that can be found in the LICENSE.md file for this repository.

package gitrpc

import "context"

// gitAdapter for accessing git commands from gitea.
type gitAdapter interface {
	InitRepository(ctx context.Context, path string, bare bool) error
	Clone(ctx context.Context, from, to string, opts cloneRepoOption) error
	AddFiles(repoPath string, all bool, files ...string) error
	Commit(repoPath string, opts commitChangesOptions) error
	Push(ctx context.Context, repoPath string, opts pushOptions) error
}

type Interface interface {
	CreateRepository(ctx context.Context, params *CreateRepositoryParams) error
}
