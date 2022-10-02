package execute

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/crypto/tmhash"
	"github.com/tendermint/tendermint/libs/log"
	tmrand "github.com/tendermint/tendermint/libs/rand"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"github.com/tendermint/tendermint/state"
	"github.com/tendermint/tendermint/test/factory"
	"github.com/tendermint/tendermint/types"
	tmtime "github.com/tendermint/tendermint/types/time"
)

const validationTestsStopHeight int64 = 10

func TestValidateBlockHeader(t *testing.T) {
	proxyApp := newTestApp()
	require.NoError(t, proxyApp.Start())
	defer proxyApp.Stop() //nolint:errcheck // ignore for tests

	s, stateDB, privVals := makeState(3, 1)
	stateStore := state.NewStore(stateDB)
	blockExec := NewBlockExecutor(
		stateStore,
		log.TestingLogger(),
		proxyApp.Consensus(),
	)
	lastCommit := types.NewCommit(0, 0, types.BlockID{Hash: tmrand.Bytes(32), PartSetHeader: types.PartSetHeader{Hash: tmrand.Bytes(32)}}, nil)

	// some bad values
	wrongHash := tmhash.Sum([]byte("this hash is wrong"))
	wrongVersion1 := s.Version.Consensus
	wrongVersion1.Block += 2
	wrongVersion2 := s.Version.Consensus
	wrongVersion2.App += 2

	// Manipulation of any header field causes failure.
	testCases := []struct {
		name          string
		malleateBlock func(block *types.Block)
	}{
		{"Version wrong1", func(block *types.Block) { block.Version = wrongVersion1 }},
		{"Version wrong2", func(block *types.Block) { block.Version = wrongVersion2 }},
		{"ChainID wrong", func(block *types.Block) { block.ChainID = "not-the-real-one" }},
		{"Height wrong", func(block *types.Block) { block.Height += 10 }},
		{"Time wrong", func(block *types.Block) { block.Time = block.Time.Add(-time.Second * 1) }},

		{"LastBlockID wrong", func(block *types.Block) { block.LastBlockID.PartSetHeader.Total += 10 }},
		{"LastCommitHash wrong", func(block *types.Block) { block.LastCommitHash = wrongHash }},

		{"ValidatorsHash wrong", func(block *types.Block) { block.ValidatorsHash = wrongHash }},
		{"NextValidatorsHash wrong", func(block *types.Block) { block.NextValidatorsHash = wrongHash }},
		{"ConsensusHash wrong", func(block *types.Block) { block.ConsensusHash = wrongHash }},
		{"AppHash wrong", func(block *types.Block) { block.AppHash = wrongHash }},
		{"LastResultsHash wrong", func(block *types.Block) { block.LastResultsHash = wrongHash }},

		{"EvidenceHash wrong", func(block *types.Block) { block.EvidenceHash = wrongHash }},
		{"Proposer wrong", func(block *types.Block) { block.ProposerAddress = ed25519.GenPrivKey().PubKey().Address() }},
		{"Proposer invalid", func(block *types.Block) { block.ProposerAddress = []byte("wrong size") }},
	}

	// Build up state for multiple heights
	for height := int64(1); height < validationTestsStopHeight; height++ {
		proposerAddr := s.Validators.GetProposer().Address
		/*
			Invalid blocks don't pass
		*/
		for _, tc := range testCases {
			block, _ := s.MakeBlock(height, factory.MakeData(makeTxs(height), nil, nil), lastCommit, proposerAddr)
			tc.malleateBlock(block)
			err := blockExec.ValidateBlock(s, block)
			require.Error(t, err, tc.name)
		}

		/*
			A good block passes
		*/
		var err error
		s, _, lastCommit, err = makeAndCommitGoodBlock(s, height, lastCommit, proposerAddr, blockExec, privVals, nil)
		require.NoError(t, err, "height %d", height)
	}

	nextHeight := validationTestsStopHeight
	block, _ := s.MakeBlock(
		nextHeight,
		factory.MakeData(factory.MakeTenTxs(nextHeight), nil, nil),
		lastCommit,
		s.Validators.GetProposer().Address,
	)
	s.InitialHeight = nextHeight + 1
	err := blockExec.ValidateBlock(s, block)
	require.Error(t, err, "expected an error when state is ahead of block")
	assert.Contains(t, err.Error(), "lower than initial height")
}

func TestValidateBlockCommit(t *testing.T) {
	proxyApp := newTestApp()
	require.NoError(t, proxyApp.Start())
	defer proxyApp.Stop() //nolint:errcheck // ignore for tests

	s, stateDB, privVals := makeState(1, 1)
	stateStore := state.NewStore(stateDB)
	blockExec := NewBlockExecutor(
		stateStore,
		log.TestingLogger(),
		proxyApp.Consensus(),
	)
	lastCommit := types.NewCommit(0, 0, types.BlockID{}, nil)
	wrongSigsCommit := types.NewCommit(1, 0, types.BlockID{}, nil)
	badPrivVal := types.NewMockPV()

	for height := int64(1); height < validationTestsStopHeight; height++ {
		proposerAddr := s.Validators.GetProposer().Address
		if height > 1 {
			/*
				#2589: ensure state.LastValidators.VerifyCommit fails here
			*/
			// should be height-1 instead of height
			wrongHeightVote, err := types.MakeVote(
				height,
				s.LastBlockID,
				s.Validators,
				privVals[proposerAddr.String()],
				chainID,
				time.Now(),
			)
			require.NoError(t, err, "height %d", height)
			wrongHeightCommit := types.NewCommit(
				wrongHeightVote.Height,
				wrongHeightVote.Round,
				s.LastBlockID,
				[]types.CommitSig{wrongHeightVote.CommitSig()},
			)
			block, _ := s.MakeBlock(
				height,
				factory.MakeData(factory.MakeTenTxs(height), nil, nil),
				wrongHeightCommit,
				proposerAddr,
			)
			require.False(t, block.LastBlockID.IsZero())
			err = blockExec.ValidateBlock(s, block)
			_, isErrInvalidCommitHeight := err.(types.ErrInvalidCommitHeight)
			require.True(t, isErrInvalidCommitHeight, "expected ErrInvalidCommitHeight at height %d but got: %v", height, err)

			/*
				#2589: test len(block.LastCommit.Signatures) == state.LastValidators.Size()
			*/
			block, _ = s.MakeBlock(
				height,
				factory.MakeData(factory.MakeTenTxs(height), nil, nil),
				wrongSigsCommit,
				proposerAddr,
			)
			err = blockExec.ValidateBlock(s, block)
			_, isErrInvalidCommitSignatures := err.(types.ErrInvalidCommitSignatures)
			require.True(t, isErrInvalidCommitSignatures,
				"expected ErrInvalidCommitSignatures at height %d, but got: %v",
				height,
				err,
			)
		}

		/*
			A good block passes
		*/
		var err error
		var blockID types.BlockID
		s, blockID, lastCommit, err = makeAndCommitGoodBlock(
			s,
			height,
			lastCommit,
			proposerAddr,
			blockExec,
			privVals,
			nil,
		)
		require.NoError(t, err, "height %d", height)

		/*
			wrongSigsCommit is fine except for the extra bad precommit
		*/
		goodVote, err := types.MakeVote(height,
			blockID,
			s.Validators,
			privVals[proposerAddr.String()],
			chainID,
			time.Now(),
		)
		require.NoError(t, err, "height %d", height)

		bpvPubKey, err := badPrivVal.GetPubKey()
		require.NoError(t, err)

		badVote := &types.Vote{
			ValidatorAddress: bpvPubKey.Address(),
			ValidatorIndex:   0,
			Height:           height,
			Round:            0,
			Timestamp:        tmtime.Now(),
			Type:             tmproto.PrecommitType,
			BlockID:          blockID,
		}

		g := goodVote.ToProto()
		b := badVote.ToProto()

		err = badPrivVal.SignVote(chainID, g)
		require.NoError(t, err, "height %d", height)
		err = badPrivVal.SignVote(chainID, b)
		require.NoError(t, err, "height %d", height)

		goodVote.Signature, badVote.Signature = g.Signature, b.Signature

		wrongSigsCommit = types.NewCommit(goodVote.Height, goodVote.Round,
			blockID, []types.CommitSig{goodVote.CommitSig(), badVote.CommitSig()})
	}
}
