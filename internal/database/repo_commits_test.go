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

func TestBatchInsertCommitSHAsWithPerforceChangelistID(t *testing.T) {
	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))

	repos := db.Repos()
	err := repos.Create(ctx, &types.Repo{ID: 1, Name: "foo"})
	require.NoError(t, err, "failed to insert repo")

	s := RepoCommitsWith(logger, db)

	repoID := int32(1)
	data := map[string]string{
		"commit1": "changelist1",
		"commit2": "changelist2",
		"commit3": "changelist3",
	}

	err = s.BatchInsertCommitSHAsWithPerforceChangelistID(ctx, api.RepoID(repoID), data)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.QueryContext(ctx, `SELECT repo_id, commit_sha, perforce_changelist_id FROM repo_commits ORDER by id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	wantedRows := map[string]string{
		"commit1": "changelist1",
		"commit2": "changelist2",
		"commit3": "changelist3",
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
}
