package productsubscription

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/sourcegraph/log/logtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/enterprise/internal/license"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/dbtest"
	"github.com/sourcegraph/sourcegraph/internal/timeutil"
)

func TestProductLicensesSetAccessTokenSHA256(t *testing.T) {
	logger := logtest.Scoped(t)
	db := database.NewDB(logger, dbtest.NewDB(logger, t))
	ctx := context.Background()

	u, err := db.Users().Create(ctx, database.NewUser{Username: "u"})
	require.NoError(t, err)

	ps, err := dbSubscriptions{db: db}.Create(ctx, u.ID, "")
	require.NoError(t, err)

	now := timeutil.Now()
	info := license.Info{
		Tags:      []string{"true-up"},
		UserCount: 10,
		ExpiresAt: now,
	}
	pl, err := dbLicenses{db: db}.Create(ctx, ps, "k", 1, info)
	require.NoError(t, err)

	token := []byte("foobar")

	t.Run("set token", func(t *testing.T) {
		err = dbTokens{db: db}.SetAccessTokenSHA256(ctx, pl, token)
		require.NoError(t, err)
	})

	t.Run("lookup token", func(t *testing.T) {
		prefixedToken := productSubscriptionAccessTokenPrefix + hex.EncodeToString(token)
		gotPS, err := dbTokens{db: db}.LookupAccessToken(ctx, prefixedToken)
		require.NoError(t, err)
		assert.Equal(t, gotPS, ps)
	})
}
