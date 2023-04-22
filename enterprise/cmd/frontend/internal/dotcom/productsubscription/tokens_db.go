package productsubscription

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/keegancsmith/sqlf"

	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/hashutil"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type dbTokens struct {
	db database.DB
}

// SetAccessTokenSHA256 activates the value as a valid token for the license.
// The value should not contain any token prefixes.
func (t dbTokens) SetAccessTokenSHA256(ctx context.Context, licenseID string, value []byte) error {
	query := sqlf.Sprintf("UPDATE product_licenses SET access_token_sha256=%s WHERE id=%s",
		hashutil.ToSHA256Bytes(value), licenseID)
	res, err := t.db.ExecContext(ctx, query.Query(sqlf.PostgresBindVar), query.Args()...)
	if err != nil {
		return err
	}
	nrows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if nrows == 0 {
		return errLicenseNotFound
	}
	return nil
}

// LookupAccessToken returns the subscription ID if the token is valid and active,
// trimming token prefixes if there are any.
func (t dbTokens) LookupAccessToken(ctx context.Context, token string) (string, error) {
	decoded, err := hex.DecodeString(strings.TrimPrefix(token, productSubscriptionAccessTokenPrefix))
	if err != nil {
		return "", errors.New("invalid token")
	}
	query := sqlf.Sprintf("SELECT product_subscription_id FROM product_licenses WHERE access_token_sha256=%s",
		hashutil.ToSHA256Bytes(decoded))
	var subscriptionID string
	if err := t.db.QueryRowContext(ctx, query.Query(sqlf.PostgresBindVar), query.Args()...).
		Scan(&subscriptionID); err != nil {
		return "", errors.New("token not found")
	}
	return subscriptionID, nil
}
