package database

import (
	"context"

	"github.com/sourcegraph/log"
	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/database/basestore"
	"github.com/sourcegraph/sourcegraph/internal/database/batch"
)

type RepoCommitsStore interface {
	Done(error) error
	Transact(context.Context) (RepoCommitsStore, error)
	With(basestore.ShareableStore) RepoCommitsStore

	BatchInsertCommitSHAsWithPerforceChangelistID(context.Context, api.RepoID, map[string]string) error
}

type repoCommitsStore struct {
	*basestore.Store
	logger log.Logger
}

var _ RepoCommitsStore = (*repoCommitsStore)(nil)

func RepoCommitsWith(logger log.Logger, other basestore.ShareableStore) RepoCommitsStore {
	return &repoCommitsStore{
		logger: logger,
		Store:  basestore.NewWithHandle(other.Handle()),
	}
}

func (s *repoCommitsStore) With(other basestore.ShareableStore) RepoCommitsStore {
	return &repoCommitsStore{logger: s.logger, Store: s.Store.With(other)}
}

func (s *repoCommitsStore) Transact(ctx context.Context) (RepoCommitsStore, error) {
	txBase, err := s.Store.Transact(ctx)
	if err != nil {
		return nil, err
	}

	return &repoCommitsStore{logger: s.logger, Store: txBase}, nil
}

func (s *repoCommitsStore) BatchInsertCommitSHAsWithPerforceChangelistID(ctx context.Context, repo_id api.RepoID, data map[string]string) error {
	tx, err := s.Store.Transact(ctx)
	if err != nil {
		return err
	}
	defer func() { err = tx.Done(err) }()

	inserter := batch.NewInserter(ctx, tx.Handle(), "repo_commits", batch.MaxNumPostgresParameters, "repo_id", "commit_sha", "perforce_changelist_id")
	for commitSHA, perforceChangelistID := range data {
		if err := inserter.Insert(
			ctx,
			int32(repo_id),
			commitSHA,
			perforceChangelistID,
		); err != nil {
			return err
		}
	}
	return inserter.Flush(ctx)
}
