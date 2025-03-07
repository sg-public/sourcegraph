package completions

import (
	"context"
	"net/http"

	"github.com/sourcegraph/log"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/enterprise"
	"github.com/sourcegraph/sourcegraph/enterprise/cmd/frontend/internal/completions/resolvers"
	"github.com/sourcegraph/sourcegraph/enterprise/internal/codeintel"
	"github.com/sourcegraph/sourcegraph/enterprise/internal/completions/streaming"
	"github.com/sourcegraph/sourcegraph/internal/conf/conftypes"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/observation"
)

func Init(
	ctx context.Context,
	observationCtx *observation.Context,
	db database.DB,
	_ codeintel.Services,
	_ conftypes.UnifiedWatchable,
	enterpriseServices *enterprise.Services,
) error {
	logger := log.Scoped("completions", "")
	enterpriseServices.NewCompletionsStreamHandler = func() http.Handler { return streaming.NewCompletionsStreamHandler(logger, db) }
	enterpriseServices.NewCodeCompletionsHandler = func() http.Handler { return streaming.NewCodeCompletionsHandler(logger, db) }
	enterpriseServices.CompletionsResolver = resolvers.NewCompletionsResolver(db)

	return nil
}
