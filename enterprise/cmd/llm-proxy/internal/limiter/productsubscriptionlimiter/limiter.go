package productsubscriptionlimiter

import (
	"context"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/sourcegraph/log"

	"github.com/sourcegraph/sourcegraph/enterprise/cmd/llm-proxy/internal/dotcom"
	"github.com/sourcegraph/sourcegraph/enterprise/cmd/llm-proxy/internal/limiter"
	"github.com/sourcegraph/sourcegraph/internal/redispool"
)

// Limiter implements limiter.Limiter backed by product subscriptions on Sourcegraph.com.
type Limiter struct {
	log    log.Logger
	redis  redispool.KeyValue
	dotcom graphql.Client
}

func NewLimiter(logger log.Logger, redis redispool.KeyValue, dotcomClient graphql.Client) limiter.Limiter {
	return &Limiter{
		log:    logger.Scoped("productsubscriptionlimiter", "Limiter backed by product subscriptions on Sourcegraph.com"),
		redis:  redis,
		dotcom: dotcomClient,
	}
}

type cachedProductSubscription struct {
	dotcom.ProductSubscription

	StaleAt time.Time `json:"staleAt"`
}

func (l *Limiter) TryAcquire(ctx context.Context, token string) error {
	var subscription cachedProductSubscription
	if v := l.redis.Get(token); !v.IsNil() {
		if err := v.UnmarshalBytes(subscription); err != nil {
			return err
		}
		if subscription.StaleAt.After(time.Now()) {
			// TODO
		}
	} else {
		resp, err := dotcom.CheckAccessToken(ctx, l.dotcom, token)
		if err != nil {
			return err
		}
		subscription = cachedProductSubscription{
			ProductSubscription: resp.Dotcom.ProductSubscriptionByAccessToken.ProductSubscription,
			StaleAt:             time.Now().Add(24 * time.Hour),
		}

		if err := l.redis.Set(token, subscription); err != nil {
			// TODO
		}
	}

	if subscription.IsArchived || subscription.LlmProxyAccess.RateLimit == nil {
		return limiter.NoAccessError{}
	}

	// TODO: Check if a rate limit is hit (limiter.RateLimitExceededError) and
	// queue the subscription for an update.
	return limiter.StaticLimiter{
		Redis:    l.redis,
		Limit:    subscription.LlmProxyAccess.RateLimit.Limit,
		Interval: time.Duration(subscription.LlmProxyAccess.RateLimit.IntervalSeconds) * time.Second,
	}.TryAcquire(ctx,
		// aggregate rate limit by product subscription ID
		subscription.Uuid)
}

func (l *Limiter) getSubscription(ctx context.Context, token string) (string, error) {

}
