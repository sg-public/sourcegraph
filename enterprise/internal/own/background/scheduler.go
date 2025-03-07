package background

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/derision-test/glock"
	"github.com/keegancsmith/sqlf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	logger "github.com/sourcegraph/log"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/basestore"
	"github.com/sourcegraph/sourcegraph/internal/goroutine"
	"github.com/sourcegraph/sourcegraph/internal/metrics"
	"github.com/sourcegraph/sourcegraph/internal/observation"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type IndexJobType struct {
	Name            string
	Id              JobTypeID
	IndexInterval   time.Duration
	RefreshInterval time.Duration
}

// QueuePerRepoIndexJobs is a slice of jobs that will automatically initialize and will queue up one index job per repo every IndexInterval.
var QueuePerRepoIndexJobs = []IndexJobType{{
	Name:            "recent-contributors",
	Id:              RecentContributors,
	IndexInterval:   time.Hour * 24,
	RefreshInterval: time.Minute * 5,
}}

var repoCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "src",
	Name:      "own_background_index_scheduler_repos_queued_total",
	Help:      "Number of repositories queued for indexing in Sourcegraph Own",
}, []string{"op"})

func GetOwnIndexSchedulerRoutines(db database.DB, observationCtx *observation.Context) (routines []goroutine.BackgroundRoutine) {
	redMetrics := metrics.NewREDMetrics(
		observationCtx.Registerer,
		"own_background_index_scheduler",
		metrics.WithLabels("op"),
		metrics.WithCountHelp("Total number of method invocations."),
	)

	op := func(jobType IndexJobType) *observation.Operation {
		return observationCtx.Operation(observation.Op{
			Name:              fmt.Sprintf("own.background.index.scheduler.%s", jobType.Name),
			MetricLabelValues: []string{jobType.Name},
			Metrics:           redMetrics,
		})
	}

	makeRoutine := func(jobType IndexJobType, op *observation.Operation, handler goroutine.Handler) goroutine.BackgroundRoutine {
		ffw := newFeatureFlagWrapper(db, jobType, op, handler)
		return goroutine.NewPeriodicGoroutineWithMetrics(context.Background(), jobType.Name, "", jobType.RefreshInterval, ffw, op)
	}

	for _, jobType := range QueuePerRepoIndexJobs {
		operation := op(jobType)
		routines = append(routines, makeRoutine(jobType, operation, newOwnRepoIndexSchedulerJob(db, jobType, operation.Logger)))
	}

	recent := IndexJobType{
		Name:            "recent-views",
		Id:              RecentViews,
		RefreshInterval: time.Minute * 5,
	}
	routines = append(routines, makeRoutine(recent, op(recent), newRecentViewsIndexer(db, observationCtx.Logger)))

	return routines
}

type featureFlagWrapper struct {
	jobType IndexJobType
	logger  logger.Logger
	db      database.DB
	handler goroutine.Handler
}

func newFeatureFlagWrapper(db database.DB, jobType IndexJobType, op *observation.Operation, handler goroutine.Handler) *featureFlagWrapper {
	return &featureFlagWrapper{
		jobType: jobType,
		logger:  op.Logger,
		db:      db,
		handler: handler,
	}
}

func (f *featureFlagWrapper) Handle(ctx context.Context) error {
	logJobDisabled := func() {
		f.logger.Info("skipping own indexing job, job disabled", logger.String("job-name", f.jobType.Name))
	}

	flag, err := database.FeatureFlagsWith(f.db).GetFeatureFlag(ctx, featureFlagName(f.jobType))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logJobDisabled()
			return nil
		} else {
			return errors.Wrap(err, "database.FeatureFlagsWith")
		}
	}
	res, ok := flag.EvaluateGlobal()
	if !ok || !res {
		logJobDisabled()
		return nil
	}
	// okay, so the job is enabled - proceed!
	f.logger.Info("Scheduling repo indexes for own job", logger.String("job-name", f.jobType.Name))
	return f.handler.Handle(ctx)
}

type ownRepoIndexSchedulerJob struct {
	store   *basestore.Store
	jobType IndexJobType
	logger  logger.Logger
	clock   glock.Clock
}

func newOwnRepoIndexSchedulerJob(db database.DB, jobType IndexJobType, logger logger.Logger) *ownRepoIndexSchedulerJob {
	store := basestore.NewWithHandle(db.Handle())
	return &ownRepoIndexSchedulerJob{jobType: jobType, store: store, logger: logger, clock: glock.NewRealClock()}
}

func (o *ownRepoIndexSchedulerJob) Handle(ctx context.Context) error {

	// convert duration to hours to match the query
	after := o.clock.Now().Add(-1 * o.jobType.IndexInterval)

	query := sqlf.Sprintf(ownIndexRepoQuery, o.jobType.Id, after, o.jobType.Id)
	val, err := o.store.ExecResult(ctx, query)
	if err != nil {
		return errors.Wrapf(err, "ownRepoIndexSchedulerJob.Handle %s", o.jobType.Name)
	}

	rows, _ := val.RowsAffected()
	o.logger.Info("Own index job scheduled", logger.String("job-name", o.jobType.Name), logger.Int64("row-count", rows))
	repoCounter.WithLabelValues(o.jobType.Name).Add(float64(rows))
	return nil
}

// Every X duration the scheduler will run and try to index repos for each job type. It will obey the following rules:
// 1. ignore jobs in progress, queued, or still in retry-backoff
// 2. ignore repos that have indexed more recently than the configured index interval for the job, ex. 24 hours
// 3. add all remaining cloned repos to the queue
// This means each (job, repo) tuple will only be index maximum once in a single interval duration
var ownIndexRepoQuery = `
WITH ineligible_repos AS (SELECT repo_id
                          FROM own_background_jobs
                          WHERE job_type = %d
                              AND (state IN ('failed', 'completed') AND finished_at > %s)
                             OR (state IN ('processing', 'errored', 'queued')))
insert into own_background_jobs (repo_id, job_type) (SELECT gr.repo_id, %d
FROM gitserver_repos gr
WHERE gr.repo_id NOT IN (SELECT * FROM ineligible_repos) and gr.clone_status = 'cloned');
`
