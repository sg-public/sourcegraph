// Package svcmain runs one or more services.
package svcmain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/getsentry/sentry-go"
	"github.com/sourcegraph/log"
	"github.com/sourcegraph/log/output"
	"github.com/urfave/cli/v2"

	"github.com/sourcegraph/sourcegraph/internal/conf"
	"github.com/sourcegraph/sourcegraph/internal/conf/deploy"
	"github.com/sourcegraph/sourcegraph/internal/debugserver"
	"github.com/sourcegraph/sourcegraph/internal/env"
	"github.com/sourcegraph/sourcegraph/internal/hostname"
	"github.com/sourcegraph/sourcegraph/internal/logging"
	"github.com/sourcegraph/sourcegraph/internal/observation"
	"github.com/sourcegraph/sourcegraph/internal/profiler"
	sgservice "github.com/sourcegraph/sourcegraph/internal/service"
	"github.com/sourcegraph/sourcegraph/internal/singleprogram"
	"github.com/sourcegraph/sourcegraph/internal/syncx"
	"github.com/sourcegraph/sourcegraph/internal/tracer"
	"github.com/sourcegraph/sourcegraph/internal/version"
)

type Config struct {
	AfterConfigure func() // run after all services' Configure hooks are called
}

// Main is called from the `main` function of the `sourcegraph-oss` and
// `sourcegraph` commands.
//
// args is the commandline arguments (usually os.Args).
func Main(services []sgservice.Service, config Config, args []string) {
	// Unlike other sourcegraph binaries we expect Sourcegraph App to be run
	// by a user instead of deployed to a cloud. So adjust the default output
	// format before initializing log.
	if _, ok := os.LookupEnv(log.EnvLogFormat); !ok && deploy.IsApp() {
		os.Setenv(log.EnvLogFormat, string(output.FormatConsole))
	}

	liblog := log.Init(log.Resource{
		Name:       env.MyName,
		Version:    version.Version(),
		InstanceID: hostname.Get(),
	},
		// Experimental: DevX is observing how sampling affects the errors signal.
		log.NewSentrySinkWith(
			log.SentrySink{
				ClientOptions: sentry.ClientOptions{SampleRate: 0.2},
			},
		),
	)

	app := cli.NewApp()
	app.Name = filepath.Base(args[0])
	app.Usage = "The Sourcegraph App"
	app.Version = version.Version()
	app.Flags = []cli.Flag{
		&cli.PathFlag{
			Name:        "cacheDir",
			DefaultText: "OS default cache",
			Usage:       "Which directory should be used to cache data",
			EnvVars:     []string{"SRC_APP_CACHE"},
			TakesFile:   false,
			Action: func(ctx *cli.Context, p cli.Path) error {
				return os.Setenv("SRC_APP_CACHE", p)
			},
		},
		&cli.PathFlag{
			Name:        "configDir",
			DefaultText: "OS default config",
			Usage:       "Directory where the configuration should be saved",
			EnvVars:     []string{"SRC_APP_CONFIG"},
			TakesFile:   false,
			Action: func(ctx *cli.Context, p cli.Path) error {
				return os.Setenv("SRC_APP_CONFIG", p)
			},
		},
	}
	app.Action = func(_ *cli.Context) error {
		logger := log.Scoped("sourcegraph", "Sourcegraph")
		singleprogram.Init(logger)
		run(liblog, logger, services, config, true, true)
		return nil
	}

	if err := app.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// DeprecatedSingleServiceMain is called from the `main` function of a command to start a single
// service (such as frontend or gitserver).
//
// DEPRECATED: Building per-service commands (i.e., a separate binary for frontend, gitserver, etc.)
// is deprecated.
func DeprecatedSingleServiceMain(svc sgservice.Service, config Config, validateConfig, useConfPackage bool) {
	liblog := log.Init(log.Resource{
		Name:       env.MyName,
		Version:    version.Version(),
		InstanceID: hostname.Get(),
	},
		// Experimental: DevX is observing how sampling affects the errors signal.
		log.NewSentrySinkWith(
			log.SentrySink{
				ClientOptions: sentry.ClientOptions{SampleRate: 0.2},
			},
		),
	)
	logger := log.Scoped("sourcegraph", "Sourcegraph")
	run(liblog, logger, []sgservice.Service{svc}, config, validateConfig, useConfPackage)
}

func run(
	liblog *log.PostInitCallbacks,
	logger log.Logger,
	services []sgservice.Service,
	config Config,
	validateConfig bool,
	useConfPackage bool,
) {
	defer liblog.Sync()

	// Initialize log15. Even though it's deprecated, it's still fairly widely used.
	logging.Init() //nolint:staticcheck // Deprecated, but logs unmigrated to sourcegraph/log look really bad without this.

	if useConfPackage {
		conf.Init()
		go conf.Watch(liblog.Update(conf.GetLogSinks))
		tracer.Init(log.Scoped("tracer", "internal tracer package"), conf.DefaultClient())
	}
	profiler.Init()

	obctx := observation.NewContext(logger)
	ctx := context.Background()

	allReady := make(chan struct{})

	// Run the services' Configure funcs before env vars are locked.
	var (
		serviceConfigs          = make([]env.Config, len(services))
		allDebugserverEndpoints []debugserver.Endpoint
	)
	for i, s := range services {
		var debugserverEndpoints []debugserver.Endpoint
		serviceConfigs[i], debugserverEndpoints = s.Configure()
		allDebugserverEndpoints = append(allDebugserverEndpoints, debugserverEndpoints...)
	}

	// Validate each service's configuration.
	//
	// This cannot be done for executor, see the executorcmd package for details.
	if validateConfig {
		for i, c := range serviceConfigs {
			if c == nil {
				continue
			}
			if err := c.Validate(); err != nil {
				logger.Fatal("invalid configuration", log.String("service", services[i].Name()), log.Error(err))
			}
		}
	}

	env.Lock()
	env.HandleHelpFlag()

	if config.AfterConfigure != nil {
		config.AfterConfigure()
	}

	// Start the debug server. The ready boolean state it publishes will become true when *all*
	// services report ready.
	var allReadyWG sync.WaitGroup
	go debugserver.NewServerRoutine(allReady, allDebugserverEndpoints...).Start()

	// Start the services.
	for i := range services {
		service := services[i]
		serviceConfig := serviceConfigs[i]
		allReadyWG.Add(1)
		go func() {
			// TODO(sqs): TODO(single-binary): Consider using the goroutine package and/or the errgroup package to report
			// errors and listen to signals to initiate cleanup in a consistent way across all
			// services.
			obctx := observation.ContextWithLogger(log.Scoped(service.Name(), service.Name()), obctx)

			// ensure ready is only called once and always call it.
			ready := syncx.OnceFunc(allReadyWG.Done)
			defer ready()

			err := service.Start(ctx, obctx, ready, serviceConfig)
			if err != nil {
				logger.Fatal("failed to start service", log.String("service", service.Name()), log.Error(err))
			}
		}()
	}

	// Pass along the signal to the debugserver that all started services are ready.
	go func() {
		allReadyWG.Wait()
		close(allReady)
	}()

	select {}
}
