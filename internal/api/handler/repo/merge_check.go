// Copyright 2022 Harness Inc. All rights reserved.
// Use of this source code is governed by the Polyform Free Trial License
// that can be found in the LICENSE.md file for this repository.

package repo

import (
	"net/http"

	"github.com/harness/gitness/internal/api/controller/repo"
	"github.com/harness/gitness/internal/api/render"
	"github.com/harness/gitness/internal/api/request"
)

// HandleMergeCheck checks if two branches are mergeable.
func HandleMergeCheck(repoCtrl *repo.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session, _ := request.AuthSessionFrom(ctx)
		repoRef, err := request.GetRepoRefFromPath(r)
		if err != nil {
			render.TranslatedUserError(w, err)
			return
		}

		path := request.GetOptionalRemainderFromPath(r)

		err = repoCtrl.MergeCheck(ctx, session, repoRef, path)
		if err != nil {
			render.TranslatedUserError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
