package completions

import (
	"context"
	"net/http"

	"github.com/sourcegraph/log"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/backend"
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

	enterpriseServices.NewCompletionsStreamHandler = func() http.Handler {
		completionsHandler := streaming.NewCompletionsStreamHandler(logger, db)
		return requireVerifiedEmailMiddleware(db, observationCtx.Logger, completionsHandler)
	}
	enterpriseServices.NewCodeCompletionsHandler = func() http.Handler {
		codeCompletionsHandler := streaming.NewCodeCompletionsHandler(logger, db)
		return requireVerifiedEmailMiddleware(db, observationCtx.Logger, codeCompletionsHandler)
	}
	enterpriseServices.CompletionsResolver = resolvers.NewCompletionsResolver(db, observationCtx.Logger)

	return nil
}

func requireVerifiedEmailMiddleware(db database.DB, logger log.Logger, next http.Handler) http.Handler {
	emails := backend.NewUserEmailsService(db, logger)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verified, err := emails.HasVerifiedEmail(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if !verified {
			// Report HTTP 403 Forbidden if user has no verified email address.
			code := http.StatusForbidden
			http.Error(w, "No verified email address found.", code)
			return
		}

		next.ServeHTTP(w, r)
	})
}
