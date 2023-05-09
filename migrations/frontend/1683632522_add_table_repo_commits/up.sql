CREATE TABLE IF NOT EXISTS repo_commits (
    id SERIAL PRIMARY KEY,
    repo_id integer NOT NULL REFERENCES repo(id) ON DELETE CASCADE DEFERRABLE,
    commit_sha text NOT NULL,
    perforce_changelist_id text
);

CREATE UNIQUE INDEX IF NOT EXISTS repo_commit_sha_unique ON repo_commits USING btree (repo_id, commit_sha);
