package application

import (
	"context"

	"go.uber.org/fx"

	"github.com/celestiaorg/celestia-node/execute"
	"github.com/celestiaorg/celestia-node/nodebuilder/node"
)

func ConstructModule(tp node.Type) fx.Option {
	return fx.Module(
		"application",
		fx.Provide(fx.Annotate(
			execute.NewApplication,
			fx.OnStart(func(ctx context.Context, app *execute.Application) error {
				return app.Start(ctx)
			}),
			fx.OnStop(func(app *execute.Application) {
				app.Stop()
			}),
		)),
	)
}
