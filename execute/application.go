package execute

import (
	"context"

	"github.com/celestiaorg/celestia-node/header"
	"github.com/celestiaorg/celestia-node/share"
	"github.com/celestiaorg/nmt/namespace"
	"github.com/tendermint/tendermint/metro"
	metroproto "github.com/tendermint/tendermint/proto/tendermint/metro"
	"github.com/tendermint/tendermint/types"
)

type Application struct {
	cfg Config

	blockExec BlockExecutor
	namespace namespace.ID

	hs header.Subscriber
	ss *share.Service
}

func NewApplication(cfg Config) *Application { return &Application{} }

func (app *Application) Start(ctx context.Context) error {

	// app.ss.GetSharesByNamespace(ctx, , app.namespace)
	return nil
}

func (app *Application) Stop() {}

func (app *Application) subscribeBlocks(ctx context.Context, blockPipe chan<- *metro.MultiBlock) error {
	defer close(blockPipe)
	// TODO: we need a process to sync or to make sure that we are at the tip of
	// the rollup (for celestia we don't need to do this)
	sub, err := app.hs.Subscribe()
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			head, err := sub.NextHeader(ctx)
			if err != nil {
				// TODO: how do we handle errors here? presumably we need to start
				// this in its own goroutine
				return err
			}
			rawShares, err := app.ss.GetSharesByNamespace(ctx, head.DAH, app.namespace)
			if err != nil {
				return err
			}
			// todo: update to use latest code from celestia-app
			msgs, err := types.ParseMsgs(rawShares)
			if err != nil {
				return err
			}

			for _, msg := range msgs.MessagesList {
				var protoMB metroproto.MultiBlock
				err = protoMB.Unmarshal(msg.Data)
				if err != nil {
					return err
				}

				mBlock, err := metro.MultiBlockFromProto(&protoMB)
				if err != nil {
					return err
				}

				blockPipe <- mBlock
			}
		}
	}
}

func (app *Application) processBlocks(mBlocks <-chan *metro.MultiBlock) {
	for mBlock := range mBlocks {
		// attempt to execute the block
	}
}
