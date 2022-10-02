package execute

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/state"
	"github.com/tendermint/tendermint/types"
)

//-----------------------------------------------------
// Validate block

func validateBlock(s state.State, block *types.Block) error {
	// Validate internal consistency.
	if err := block.ValidateBasic(); err != nil {
		return err
	}

	// Validate basic info.
	if block.Version.App != s.Version.Consensus.App ||
		block.Version.Block != s.Version.Consensus.Block {
		return fmt.Errorf("wrong Block.Header.Version. Expected %v, got %v",
			s.Version.Consensus,
			block.Version,
		)
	}
	if block.ChainID != s.ChainID {
		return fmt.Errorf("wrong Block.Header.ChainID. Expected %v, got %v",
			s.ChainID,
			block.ChainID,
		)
	}
	if s.LastBlockHeight == 0 && block.Height != s.InitialHeight {
		return fmt.Errorf("wrong Block.Header.Height. Expected %v for initial block, got %v",
			block.Height, s.InitialHeight)
	}
	if s.LastBlockHeight > 0 && block.Height != s.LastBlockHeight+1 {
		return fmt.Errorf("wrong Block.Header.Height. Expected %v, got %v",
			s.LastBlockHeight+1,
			block.Height,
		)
	}
	// Validate prev block info.
	if !block.LastBlockID.Equals(s.LastBlockID) {
		return fmt.Errorf("wrong Block.Header.LastBlockID.  Expected %v, got %v",
			s.LastBlockID,
			block.LastBlockID,
		)
	}

	// Validate app info
	if !bytes.Equal(block.AppHash, s.AppHash) {
		return fmt.Errorf("wrong Block.Header.AppHash.  Expected %X, got %v",
			s.AppHash,
			block.AppHash,
		)
	}
	hashCP := types.HashConsensusParams(s.ConsensusParams)
	if !bytes.Equal(block.ConsensusHash, hashCP) {
		return fmt.Errorf("wrong Block.Header.ConsensusHash.  Expected %X, got %v",
			hashCP,
			block.ConsensusHash,
		)
	}
	if !bytes.Equal(block.LastResultsHash, s.LastResultsHash) {
		return fmt.Errorf("wrong Block.Header.LastResultsHash.  Expected %X, got %v",
			s.LastResultsHash,
			block.LastResultsHash,
		)
	}
	if !bytes.Equal(block.ValidatorsHash, s.Validators.Hash()) {
		return fmt.Errorf("wrong Block.Header.ValidatorsHash.  Expected %X, got %v",
			s.Validators.Hash(),
			block.ValidatorsHash,
		)
	}
	if !bytes.Equal(block.NextValidatorsHash, s.NextValidators.Hash()) {
		return fmt.Errorf("wrong Block.Header.NextValidatorsHash.  Expected %X, got %v",
			s.NextValidators.Hash(),
			block.NextValidatorsHash,
		)
	}

	// Validate block LastCommit.
	if block.Height == s.InitialHeight {
		if len(block.LastCommit.Signatures) != 0 {
			return errors.New("initial block can't have LastCommit signatures")
		}
	} else {
		// LastCommit.Signatures length is checked in VerifyCommit.
		if err := s.LastValidators.VerifyCommit(
			s.ChainID, s.LastBlockID, block.Height-1, block.LastCommit); err != nil {
			return err
		}
	}

	// NOTE: We can't actually verify it's the right proposer because we don't
	// know what round the block was first proposed. So just check that it's
	// a legit address and a known validator.
	if len(block.ProposerAddress) != crypto.AddressSize {
		return fmt.Errorf("expected ProposerAddress size %d, got %d",
			crypto.AddressSize,
			len(block.ProposerAddress),
		)
	}
	if !s.Validators.HasAddress(block.ProposerAddress) {
		return fmt.Errorf("block.Header.ProposerAddress %X is not a validator",
			block.ProposerAddress,
		)
	}

	// Validate block Time
	switch {
	case block.Height > s.InitialHeight:
		if !block.Time.After(s.LastBlockTime) {
			return fmt.Errorf("block time %v not greater than last block time %v",
				block.Time,
				s.LastBlockTime,
			)
		}
		medianTime := state.MedianTime(block.LastCommit, s.LastValidators)
		if !block.Time.Equal(medianTime) {
			return fmt.Errorf("invalid block time. Expected %v, got %v",
				medianTime,
				block.Time,
			)
		}

	case block.Height == s.InitialHeight:
		genesisTime := s.LastBlockTime
		if !block.Time.Equal(genesisTime) {
			return fmt.Errorf("block time %v is not equal to genesis time %v",
				block.Time,
				genesisTime,
			)
		}

	default:
		return fmt.Errorf("block height %v lower than initial height %v",
			block.Height, s.InitialHeight)
	}

	// Check evidence doesn't exceed the limit amount of bytes.
	if max, got := s.ConsensusParams.Evidence.MaxBytes, block.Evidence.ByteSize(); got > max {
		return types.NewErrEvidenceOverflow(max, got)
	}

	return nil
}
