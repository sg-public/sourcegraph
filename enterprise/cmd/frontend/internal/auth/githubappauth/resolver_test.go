package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/sourcegraph/log/logtest"
	"github.com/stretchr/testify/require"

	gqlerrors "github.com/graph-gophers/graphql-go/errors"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	edb "github.com/sourcegraph/sourcegraph/enterprise/internal/database"
	"github.com/sourcegraph/sourcegraph/enterprise/internal/github_apps/store"
	ghtypes "github.com/sourcegraph/sourcegraph/enterprise/internal/github_apps/types"
	"github.com/sourcegraph/sourcegraph/internal/actor"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/extsvc"
	"github.com/sourcegraph/sourcegraph/internal/gitserver"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/schema"
)

// userCtx returns a context where give user ID identifies logged in user.
func userCtx(userID int32) context.Context {
	ctx := context.Background()
	a := actor.FromUser(userID)
	return actor.WithActor(ctx, a)
}

func TestResolver_DeleteGitHubApp(t *testing.T) {
	logger := logtest.Scoped(t)
	id := 1
	graphqlID := MarshalGitHubAppID(int64(id))

	userStore := database.NewMockUserStore()
	userStore.GetByCurrentAuthUserFunc.SetDefaultHook(func(ctx context.Context) (*types.User, error) {
		a := actor.FromContext(ctx)
		if a.UID == 1 {
			return &types.User{ID: 1, SiteAdmin: true}, nil
		}
		if a.UID == 2 {
			return &types.User{ID: 2, SiteAdmin: false}, nil
		}
		return nil, errors.New("not found")
	})

	gitHubAppsStore := store.NewStrictMockGitHubAppsStore()
	gitHubAppsStore.DeleteFunc.SetDefaultHook(func(ctx context.Context, app int) error {
		if app != id {
			return errors.New(fmt.Sprintf("app with id %d does not exist", id))
		}
		return nil
	})

	db := edb.NewStrictMockEnterpriseDB()

	db.GitHubAppsFunc.SetDefaultReturn(gitHubAppsStore)
	db.UsersFunc.SetDefaultReturn(userStore)

	adminCtx := userCtx(1)
	userCtx := userCtx(2)

	schema, err := graphqlbackend.NewSchema(db, gitserver.NewClient(), nil, graphqlbackend.OptionalResolver{GitHubAppsResolver: NewResolver(logger, db)})
	require.NoError(t, err)

	graphqlbackend.RunTests(t, []*graphqlbackend.Test{{
		Schema:  schema,
		Context: adminCtx,
		Query: `
			mutation DeleteGitHubApp($gitHubApp: ID!) {
				deleteGitHubApp(gitHubApp: $gitHubApp) {
					alwaysNil
				}
			}`,
		ExpectedResult: `{
			"deleteGitHubApp": {
				"alwaysNil": null
			}
		}`,
		Variables: map[string]any{
			"gitHubApp": string(graphqlID),
		},
	}, {
		Schema:  schema,
		Context: userCtx,
		Query: `
			mutation DeleteGitHubApp($gitHubApp: ID!) {
				deleteGitHubApp(gitHubApp: $gitHubApp) {
					alwaysNil
				}
			}`,
		ExpectedResult: `{"deleteGitHubApp": null}`,
		ExpectedErrors: []*gqlerrors.QueryError{{
			Message: "must be site admin",
			Path:    []any{string("deleteGitHubApp")},
		}},
		Variables: map[string]any{
			"gitHubApp": string(graphqlID),
		},
	}})
}

func TestResolver_GitHubApps(t *testing.T) {
	logger := logtest.Scoped(t)
	id := 1
	graphqlID := MarshalGitHubAppID(int64(id))

	userStore := database.NewMockUserStore()
	userStore.GetByCurrentAuthUserFunc.SetDefaultHook(func(ctx context.Context) (*types.User, error) {
		a := actor.FromContext(ctx)
		if a.UID == 1 {
			return &types.User{ID: 1, SiteAdmin: true}, nil
		}
		if a.UID == 2 {
			return &types.User{ID: 2, SiteAdmin: false}, nil
		}
		return nil, errors.New("not found")
	})

	gitHubAppsStore := store.NewStrictMockGitHubAppsStore()
	gitHubAppsStore.ListFunc.SetDefaultReturn([]*ghtypes.GitHubApp{
		{
			ID: 1,
		},
	}, nil)

	db := edb.NewStrictMockEnterpriseDB()

	db.GitHubAppsFunc.SetDefaultReturn(gitHubAppsStore)
	db.UsersFunc.SetDefaultReturn(userStore)

	adminCtx := userCtx(1)
	userCtx := userCtx(2)

	schema, err := graphqlbackend.NewSchema(db, gitserver.NewClient(), nil, graphqlbackend.OptionalResolver{GitHubAppsResolver: NewResolver(logger, db)})
	require.NoError(t, err)

	graphqlbackend.RunTests(t, []*graphqlbackend.Test{{
		Schema:  schema,
		Context: adminCtx,
		Query: `
			query GitHubApps() {
				gitHubApps {
					nodes {
						id
					}
				}
			}`,
		ExpectedResult: fmt.Sprintf(`{
			"gitHubApps": {
				"nodes": [
					{"id":"%s"}
				]
			}
		}`, graphqlID),
	}, {
		Schema:  schema,
		Context: userCtx,
		Query: `
			query GitHubApps() {
				gitHubApps {
					nodes {
						id
					}
				}
			}`,
		ExpectedResult: `null`,
		ExpectedErrors: []*gqlerrors.QueryError{{
			Message: "must be site admin",
			Path:    []any{string("gitHubApps")},
		}},
	}})
}

func TestResolver_GitHubApp(t *testing.T) {
	logger := logtest.Scoped(t)
	id := 1
	graphqlID := MarshalGitHubAppID(int64(id))

	userStore := database.NewMockUserStore()
	userStore.GetByCurrentAuthUserFunc.SetDefaultHook(func(ctx context.Context) (*types.User, error) {
		a := actor.FromContext(ctx)
		if a.UID == 1 {
			return &types.User{ID: 1, SiteAdmin: true}, nil
		}
		if a.UID == 2 {
			return &types.User{ID: 2, SiteAdmin: false}, nil
		}
		return nil, errors.New("not found")
	})

	gitHubAppsStore := store.NewStrictMockGitHubAppsStore()
	gitHubAppsStore.GetByIDFunc.SetDefaultReturn(&ghtypes.GitHubApp{
		ID:      1,
		AppID:   1,
		BaseURL: "https://github.com",
	}, nil)

	extsvcStore := database.NewMockExternalServiceStore()
	conn := &schema.GitHubConnection{
		Url: "https://github.com",
		GitHubAppDetails: &schema.GitHubAppDetails{
			AppID:          1,
			InstallationID: 123,
		},
	}
	extsvcConn, err := json.Marshal(conn)
	require.NoError(t, err)
	extsvcStore.ListFunc.SetDefaultReturn([]*types.ExternalService{{
		DisplayName: "MockExtsvc",
		Kind:        extsvc.KindGitHub,
		Config:      extsvc.NewUnencryptedConfig(string(extsvcConn)),
	}}, nil)

	db := edb.NewStrictMockEnterpriseDB()

	db.GitHubAppsFunc.SetDefaultReturn(gitHubAppsStore)
	db.UsersFunc.SetDefaultReturn(userStore)
	db.ExternalServicesFunc.SetDefaultReturn(extsvcStore)

	adminCtx := userCtx(1)
	userCtx := userCtx(2)

	schema, err := graphqlbackend.NewSchema(db, gitserver.NewClient(), nil, graphqlbackend.OptionalResolver{GitHubAppsResolver: NewResolver(logger, db)})
	require.NoError(t, err)

	graphqlbackend.RunTests(t, []*graphqlbackend.Test{{
		Schema:  schema,
		Context: adminCtx,
		Query: fmt.Sprintf(`
			query {
				gitHubApp(id: "%s") {
					id
					externalServices(first: 1) {
						nodes {
							displayName
						}
					}
				}
			}`, graphqlID),
		ExpectedResult: fmt.Sprintf(`{
			"gitHubApp": {
				"id": "%s",
				"externalServices": {
						"nodes": [{
							"displayName": "MockExtsvc"
					}]
				}
			}
		}`, graphqlID),
	}, {
		Schema:  schema,
		Context: userCtx,
		Query: fmt.Sprintf(`
			query {
				gitHubApp(id: "%s") {
					id
				}
			}`, graphqlID),
		ExpectedResult: `{"gitHubApp": null}`,
		ExpectedErrors: []*gqlerrors.QueryError{{
			Message: "must be site admin",
			Path:    []any{string("gitHubApp")},
		}},
	}})
}

func TestResolver_GitHubAppByAppID(t *testing.T) {
	logger := logtest.Scoped(t)
	id := 1
	appID := 1234
	baseURL := "https://github.com"
	name := "Horsegraph App"

	userStore := database.NewMockUserStore()
	userStore.GetByCurrentAuthUserFunc.SetDefaultHook(func(ctx context.Context) (*types.User, error) {
		a := actor.FromContext(ctx)
		if a.UID == 1 {
			return &types.User{ID: 1, SiteAdmin: true}, nil
		}
		if a.UID == 2 {
			return &types.User{ID: 2, SiteAdmin: false}, nil
		}
		return nil, errors.New("not found")
	})

	gitHubAppsStore := store.NewStrictMockGitHubAppsStore()
	gitHubAppsStore.GetByAppIDFunc.SetDefaultReturn(&ghtypes.GitHubApp{
		ID:      id,
		AppID:   appID,
		BaseURL: baseURL,
		Name:    name,
	}, nil)

	db := edb.NewStrictMockEnterpriseDB()

	db.GitHubAppsFunc.SetDefaultReturn(gitHubAppsStore)
	db.UsersFunc.SetDefaultReturn(userStore)

	adminCtx := userCtx(1)
	userCtx := userCtx(2)

	schema, err := graphqlbackend.NewSchema(db, gitserver.NewClient(), nil, graphqlbackend.OptionalResolver{GitHubAppsResolver: NewResolver(logger, db)})
	require.NoError(t, err)

	graphqlbackend.RunTests(t, []*graphqlbackend.Test{{
		Schema:  schema,
		Context: adminCtx,
		Query: fmt.Sprintf(`
			query {
				gitHubAppByAppID(appID: %d, baseURL: "%s") {
					id
					name
				}
			}`, appID, baseURL),
		ExpectedResult: fmt.Sprintf(`{
			"gitHubAppByAppID": {
				"id": "%s",
				"name": "%s"
			}
		}`, MarshalGitHubAppID(int64(id)), name),
	}, {
		Schema:  schema,
		Context: userCtx,
		Query: fmt.Sprintf(`
			query {
				gitHubAppByAppID(appID: %d, baseURL: "%s") {
					id
				}
			}`, appID, baseURL),
		ExpectedResult: `{"gitHubAppByAppID": null}`,
		ExpectedErrors: []*gqlerrors.QueryError{{
			Message: "must be site admin",
			Path:    []any{string("gitHubAppByAppID")},
		}},
	}})
}
