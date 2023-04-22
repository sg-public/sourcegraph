package productsubscription

import (
	"context"
	"encoding/hex"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/internal/auth"
	"github.com/sourcegraph/sourcegraph/internal/hashutil"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// productSubscriptionAccessTokenPrefix is the prefix used for identifying tokens
// generated for product subscriptions.
const productSubscriptionAccessTokenPrefix = "sgs_"

type productSubscriptionAccessToken struct {
	accessToken string
}

func (t productSubscriptionAccessToken) AccessToken() string { return t.accessToken }

func (r ProductSubscriptionLicensingResolver) GenerateAccessTokenForSubscription(ctx context.Context, args *graphqlbackend.GenerateAccessTokenForSubscriptionArgs) (graphqlbackend.ProductSubscriptionAccessToken, error) {
	// 🚨 SECURITY: Only site admins may generate product access tokens.
	if err := auth.CheckCurrentUserIsSiteAdmin(ctx, r.DB); err != nil {
		return nil, err
	}

	sub, err := productSubscriptionByID(ctx, r.DB, args.ProductSubscriptionID)
	if err != nil {
		return nil, err
	}

	active, err := dbLicenses{db: r.DB}.Active(ctx, sub.v.ID)
	if err != nil {
		return nil, err
	} else if active == nil {
		return nil, errors.New("an active license is required")
	}

	// Access token comprises of subscription ID and the license key.
	accessTokenRaw := hashutil.ToSHA256Bytes([]byte(sub.v.ID + " " + active.LicenseKey))

	// The token comprises of a prefix and the above token.
	accessToken := productSubscriptionAccessToken{
		accessToken: productSubscriptionAccessTokenPrefix + hex.EncodeToString(accessTokenRaw),
	}

	// Token already enabled, just return the token
	if len(active.AccessTokenSHA256) > 0 {
		return accessToken, nil
	}

	// Otherwise, enable before returning
	if err := (dbTokens{db: r.DB}).SetAccessTokenSHA256(ctx, active.ID, accessTokenRaw); err != nil {
		return nil, err
	}
	return accessToken, nil
}

func (r ProductSubscriptionLicensingResolver) ProductSubscriptionByAccessToken(ctx context.Context, args *graphqlbackend.ProductSubscriptionByAccessTokenArgs) (graphqlbackend.ProductSubscription, error) {
	// 🚨 SECURITY: Only site admins may generate product access tokens.
	if err := auth.CheckCurrentUserIsSiteAdmin(ctx, r.DB); err != nil {
		return nil, err
	}

	subID, err := dbTokens{db: r.DB}.LookupAccessToken(ctx, args.AccessToken)
	if err != nil {
		return nil, err
	}
	v, err := dbSubscriptions{db: r.DB}.GetByID(ctx, subID)
	if err != nil {
		return nil, err
	}
	sub := &productSubscription{v: v, db: r.DB}
	if sub.IsArchived() {
		return nil, errors.New("subscription archived")
	}
	return sub, nil
}
