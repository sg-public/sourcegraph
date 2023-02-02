package executor_test

import (
	"context"
	"testing"

	"github.com/sourcegraph/log/logtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/enterprise/internal/executor"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/dbtest"
	"github.com/sourcegraph/sourcegraph/internal/observation"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func TestJobTokenStore_Create(t *testing.T) {
	tokenStore := newTokenStore(t)

	tests := []struct {
		name        string
		jobId       int
		queue       string
		expectedErr error
	}{
		{
			name:  "Token created",
			jobId: 10,
			queue: "test",
		},
		{
			name:        "No jobId",
			queue:       "test",
			expectedErr: errors.New("missing jobId"),
		},
		{
			name:        "No queue",
			jobId:       10,
			expectedErr: errors.New("missing queue"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := tokenStore.Create(context.Background(), test.jobId, test.queue)
			if test.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectedErr.Error(), err.Error())
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, token)
			}
		})
	}
}

func TestJobTokenStore_Create_Duplicate(t *testing.T) {
	logger := logtest.Scoped(t)
	db := database.NewDB(logger, dbtest.NewDB(logger, t))
	tokenStore := executor.NewJobTokenStore(&observation.TestContext, db)

	_, err := tokenStore.Create(context.Background(), 10, "test")
	require.NoError(t, err)
	_, err = tokenStore.Create(context.Background(), 10, "test")
	require.Error(t, err)
}

func TestJobTokenStore_Regenerate(t *testing.T) {
	tokenStore := newTokenStore(t)

	// Create an existing token to test against
	_, err := tokenStore.Create(context.Background(), 10, "test")
	require.NoError(t, err)

	tests := []struct {
		name        string
		jobId       int
		queue       string
		expectedErr error
	}{
		{
			name:  "Regenerate Token",
			jobId: 10,
			queue: "test",
		},
		{
			name:        "Token does not exist",
			jobId:       100,
			queue:       "test1",
			expectedErr: errors.New("job token does not exist"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := tokenStore.Regenerate(context.Background(), test.jobId, test.queue)
			if test.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectedErr.Error(), err.Error())
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, token)
			}
		})
	}
}

func TestJobTokenStore_Exists(t *testing.T) {
	tokenStore := newTokenStore(t)

	// Create an existing token to test against
	_, err := tokenStore.Create(context.Background(), 10, "test")
	require.NoError(t, err)

	tests := []struct {
		name           string
		jobId          int
		queue          string
		expectedExists bool
		expectedErr    error
	}{
		{
			name:           "Token exists",
			jobId:          10,
			queue:          "test",
			expectedExists: true,
		},
		{
			name:  "Token does not exist",
			jobId: 100,
			queue: "test1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exists, err := tokenStore.Exists(context.Background(), test.jobId, test.queue)
			if test.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectedErr.Error(), err.Error())
				assert.False(t, exists)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedExists, exists)
			}
		})
	}
}

func TestJobTokenStore_Get(t *testing.T) {
	tokenStore := newTokenStore(t)

	// Create an existing token to test against
	_, err := tokenStore.Create(context.Background(), 10, "test")
	require.NoError(t, err)

	tests := []struct {
		name             string
		jobId            int
		queue            string
		expectedJobToken executor.JobToken
		expectedErr      error
	}{
		{
			name:  "Retrieve token",
			jobId: 10,
			queue: "test",
			expectedJobToken: executor.JobToken{
				Id:    1,
				JobId: 10,
				Queue: "test",
			},
		},
		{
			name:        "Token does not exist",
			jobId:       100,
			queue:       "test1",
			expectedErr: errors.New("sql: no rows in result set"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jobToken, err := tokenStore.Get(context.Background(), test.jobId, test.queue)
			if test.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectedErr.Error(), err.Error())
				assert.Zero(t, jobToken.Id)
				assert.Empty(t, jobToken.Value)
				assert.Zero(t, jobToken.JobId)
				assert.Empty(t, jobToken.Queue)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedJobToken.Id, jobToken.Id)
				assert.Equal(t, test.expectedJobToken.JobId, jobToken.JobId)
				assert.Equal(t, test.expectedJobToken.Queue, jobToken.Queue)
				assert.NotEmpty(t, jobToken.Value)
			}
		})
	}
}

func TestJobTokenStore_GetByToken(t *testing.T) {
	tokenStore := newTokenStore(t)

	// Create an existing token to test against
	token, err := tokenStore.Create(context.Background(), 10, "test")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	tests := []struct {
		name             string
		token            string
		expectedJobToken executor.JobToken
		expectedErr      error
	}{
		{
			name:  "Retrieve token",
			token: token,
			expectedJobToken: executor.JobToken{
				Id:    1,
				JobId: 10,
				Queue: "test",
			},
		},
		{
			name:        "Token does not exist",
			token:       "666f6f626172", // foobar
			expectedErr: errors.New("sql: no rows in result set"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jobToken, err := tokenStore.GetByToken(context.Background(), test.token)
			if test.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectedErr.Error(), err.Error())
				assert.Zero(t, jobToken.Id)
				assert.Empty(t, jobToken.Value)
				assert.Zero(t, jobToken.JobId)
				assert.Empty(t, jobToken.Queue)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedJobToken.Id, jobToken.Id)
				assert.Equal(t, test.expectedJobToken.JobId, jobToken.JobId)
				assert.Equal(t, test.expectedJobToken.Queue, jobToken.Queue)
				assert.NotEmpty(t, jobToken.Value)
			}
		})
	}
}

func TestJobTokenStore_Delete(t *testing.T) {
	tokenStore := newTokenStore(t)

	// Create an existing token to test against
	_, err := tokenStore.Create(context.Background(), 10, "test")
	require.NoError(t, err)

	tests := []struct {
		name        string
		jobId       int
		queue       string
		expectedErr error
	}{
		{
			name:  "Token deleted",
			jobId: 10,
			queue: "test",
		},
		{
			name:  "Token does not exist",
			jobId: 100,
			queue: "test1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := tokenStore.Delete(context.Background(), test.jobId, test.queue)
			if test.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectedErr.Error(), err.Error())
			} else {
				require.NoError(t, err)
				// Double-check the token has been deleted
				exists, err := tokenStore.Exists(context.Background(), test.jobId, test.queue)
				require.NoError(t, err)
				assert.False(t, exists)
			}
		})
	}
}

func newTokenStore(t *testing.T) executor.JobTokenStore {
	logger := logtest.Scoped(t)
	db := database.NewDB(logger, dbtest.NewDB(logger, t))
	tokenStore := executor.NewJobTokenStore(&observation.TestContext, db)
	return tokenStore
}
