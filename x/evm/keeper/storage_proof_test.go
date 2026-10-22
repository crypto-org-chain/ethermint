package keeper_test

import (
	"context"
	"math/big"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/cosmos/cosmos-sdk/store/v2/rootmulti"
	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/x/evm/types"
)

func (suite *StateDBTestSuite) TestStorageProofs() {
	value := common.BigToHash(big.NewInt(0x0102))
	legacyKey := common.BytesToHash([]byte("legacy"))
	compactKey := common.BytesToHash([]byte("compact"))
	zeroedKey := common.BytesToHash([]byte("zeroed"))
	commitState := func(key, v common.Hash) {
		vmdb := suite.StateDB()
		vmdb.SetState(suite.Address, key, v)
		suite.Require().NoError(vmdb.Commit())
	}

	suite.setCompactStorage(false)
	commitState(legacyKey, value)
	suite.setCompactStorage(true)
	commitState(compactKey, value)
	commitState(zeroedKey, common.BigToHash(big.NewInt(1)))
	commitState(zeroedKey, common.Hash{})
	// proofs need height > 1
	suite.Commit(suite.T())
	suite.Commit(suite.T())

	commit := suite.App.LastCommitID()
	prt := rootmulti.DefaultProofRuntime()

	testCases := []struct {
		name     string
		key      common.Hash
		expValue []byte // nil means the proof must show the key is absent
	}{
		{"legacy slot proves its 32-byte value", legacyKey, value.Bytes()},
		{"compact slot proves its trimmed value", compactKey, []byte{0x01, 0x02}},
		{"zeroed slot proves absence", zeroedKey, nil},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			storeKey := types.StateKey(suite.Address, tc.key)
			res, err := suite.App.Query(context.Background(), &abci.RequestQuery{
				Path:   "/store/evm/key",
				Data:   storeKey,
				Height: commit.Version,
				Prove:  true,
			})
			suite.Require().NoError(err)
			suite.Require().Zero(res.Code, res.Log)
			suite.Require().Equal(tc.expValue, res.Value)

			keyPath := merkle.KeyPath{}.
				AppendKey([]byte(types.StoreKey), merkle.KeyEncodingURL).
				AppendKey(storeKey, merkle.KeyEncodingURL).
				String()
			if tc.expValue == nil {
				suite.Require().NoError(prt.VerifyAbsence(res.ProofOps, commit.Hash, keyPath))
				return
			}
			suite.Require().NoError(prt.VerifyValue(res.ProofOps, commit.Hash, keyPath, tc.expValue))
			// a verifier that left-pads the value to 32 bytes only matches legacy slots
			padded := common.LeftPadBytes(tc.expValue, common.HashLength)
			if len(tc.expValue) != common.HashLength {
				suite.Require().Error(prt.VerifyValue(res.ProofOps, commit.Hash, keyPath, padded))
			}
		})
	}
}
