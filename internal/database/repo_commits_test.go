package database

import (
	"context"
	"testing"

	"github.com/sourcegraph/log/logtest"
	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/database/dbtest"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/stretchr/testify/require"
)

func TestRepoCommits(t *testing.T) {
	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))

	repos := db.Repos()
	err := repos.Create(ctx, &types.Repo{ID: 1, Name: "foo"})
	require.NoError(t, err, "failed to insert repo")

	repoID := int32(1)
	data := map[string]string{
		"commit1": "123",
		"commit2": "124",
		"commit3": "125",
	}

	s := RepoCommitsWith(logger, db)

	err = s.BatchInsertCommitSHAsWithPerforceChangelistID(ctx, api.RepoID(repoID), data)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("BatchInsertCommitSHAsWithPerforceChangelistID", func(t *testing.T) {
		rows, err := db.QueryContext(ctx, `SELECT repo_id, commit_sha, perforce_changelist_id FROM repo_commits ORDER by id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()

		wantedRows := map[string]string{
			"commit1": "123",
			"commit2": "124",
			"commit3": "125",
		}

		totalRows := 0
		for rows.Next() {
			var haveRepoID int32
			var haveCommitSHA, haveChangelist string
			if err := rows.Scan(&haveRepoID, &haveCommitSHA, &haveChangelist); err != nil {
				t.Fatal(err)
			}

			require.Equal(t, repoID, haveRepoID, "mismatched repoID")
			require.Equal(t, wantedRows[haveCommitSHA], haveChangelist, "mismatched commitSHA and changelist ID")

			totalRows += 1
		}

		require.Equal(t, 3, totalRows, "mismatched number of rows")
	})

	t.Run("GetLatestForRepo", func(t *testing.T) {
		repoCommit, err := s.GetLatestForRepo(ctx, api.RepoID(repoID))
		require.NoError(t, err, "unexpected error in GetLatestForRepo")
		require.NotNil(t, repoCommit, "repoCommit was not expected to be nil")
		require.Equal(
			t,
			&types.RepoCommit{
				ID:                   3,
				RepoID:               api.RepoID(repoID),
				CommitSHA:            "commit3",
				PerforceChangelistID: "125",
			},
			repoCommit,
			"repoCommit row is not as expected",
		)
	})

}
