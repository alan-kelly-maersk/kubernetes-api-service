// Command apiserver runs the widgets.example.com aggregated API server.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/healthz"
	genericoptions "k8s.io/apiserver/pkg/server/options"

	// pflag is a mandatory transitive dependency of k8s.io/apiserver's own
	// option types (they bind flags via *pflag.FlagSet); used directly here
	// instead of pulling in a CLI framework like cobra on top of it.
	"github.com/spf13/pflag"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alan-kelly-maersk/kubernetes-api-service/internal/adapters/postgres"
	restadapter "github.com/alan-kelly-maersk/kubernetes-api-service/internal/adapters/restapi"
	apiserverpkg "github.com/alan-kelly-maersk/kubernetes-api-service/internal/apiserver"
	"github.com/alan-kelly-maersk/kubernetes-api-service/internal/domain/widget"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	o := newOptions()

	fs := pflag.NewFlagSet("apiserver", pflag.ExitOnError)
	o.addFlags(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return o.run(ctx)
}

// options holds everything needed to start the server, kept as a plain
// struct (no CLI framework) so flag wiring stays a single, readable place.
type options struct {
	recommended *genericoptions.RecommendedOptions
	postgresDSN string
}

func newOptions() *options {
	o := &options{
		recommended: genericoptions.NewRecommendedOptions(
			"", // no default etcd prefix; we don't use the etcd-backed storage path
			apiserverpkg.Codecs.LegacyCodec(),
		),
	}
	// This service persists to Postgres, not etcd - drop the inherited
	// etcd options so users aren't misled into configuring them.
	o.recommended.Etcd = nil
	return o
}

func (o *options) addFlags(fs *pflag.FlagSet) {
	o.recommended.SecureServing.AddFlags(fs)
	o.recommended.Authentication.AddFlags(fs)
	o.recommended.Authorization.AddFlags(fs)
	o.recommended.Audit.AddFlags(fs)
	o.recommended.Features.AddFlags(fs)
	o.recommended.CoreAPI.AddFlags(fs)
	fs.StringVar(&o.postgresDSN, "postgres-dsn", os.Getenv("POSTGRES_DSN"),
		"PostgreSQL connection string (falls back to POSTGRES_DSN env var)")
}

func (o *options) run(ctx context.Context) error {
	if o.postgresDSN == "" {
		return fmt.Errorf("--postgres-dsn (or POSTGRES_DSN) is required")
	}

	if errs := o.validate(); len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	pool, err := pgxpool.New(ctx, o.postgresDSN)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	repo := postgres.New(pool)
	svc := widget.NewService(repo, widget.SystemClock)
	widgetStorage := restadapter.NewWidgetStorage(svc)

	serverConfig := genericapiserver.NewRecommendedConfig(apiserverpkg.Codecs)
	if err := o.recommended.ApplyTo(serverConfig); err != nil {
		return err
	}
	serverConfig.HealthzChecks = append(serverConfig.HealthzChecks, healthz.NamedCheck("postgres", func(_ *http.Request) error {
		return pool.Ping(ctx)
	}))

	cfg := &apiserverpkg.Config{GenericConfig: serverConfig}
	server, err := cfg.New(apiserverpkg.Storage{
		"widgets": widgetStorage,
	})
	if err != nil {
		return err
	}

	prepared := server.GenericAPIServer.PrepareRun()
	return prepared.Run(ctx.Done())
}

func (o *options) validate() []error {
	var errs []error
	errs = append(errs, o.recommended.SecureServing.Validate()...)
	errs = append(errs, o.recommended.Authentication.Validate()...)
	errs = append(errs, o.recommended.Authorization.Validate()...)
	errs = append(errs, o.recommended.Audit.Validate()...)
	errs = append(errs, o.recommended.Features.Validate()...)
	errs = append(errs, o.recommended.CoreAPI.Validate()...)
	return errs
}
