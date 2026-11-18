package legacyparams_test

import (
	"testing"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/types/legacyparams"
	"github.com/stretchr/testify/suite"
)

type SubspaceTestSuite struct {
	suite.Suite
	key *storetypes.KVStoreKey
	ctx sdk.Context
}

func TestSubspaceTestSuite(t *testing.T) {
	suite.Run(t, new(SubspaceTestSuite))
}

func (s *SubspaceTestSuite) SetupTest() {
	s.key = storetypes.NewKVStoreKey("params")
	s.ctx = sdktestutil.DefaultContext(s.key, storetypes.NewTransientStoreKey("transient_params"))
}

func (s *SubspaceTestSuite) TestGetIfExists() {
	testCases := []struct {
		name        string
		subspace    func() legacyparams.Subspace
		stored      map[string]string
		expDenom    string
		expHeight   int64
		expectPanic bool
	}{
		{
			name:      "decodes stored values under the subspace prefix",
			subspace:  func() legacyparams.Subspace { return legacyparams.NewSubspace(s.key, "evm") },
			stored:    map[string]string{"evm/Denom": `"aphoton"`, "evm/Height": `"7"`},
			expDenom:  "aphoton",
			expHeight: 7,
		},
		{
			name:      "missing values keep their defaults",
			subspace:  func() legacyparams.Subspace { return legacyparams.NewSubspace(s.key, "evm") },
			stored:    map[string]string{"evm/Denom": `"aphoton"`},
			expDenom:  "aphoton",
			expHeight: -1,
		},
		{
			name:      "other subspaces are not read",
			subspace:  func() legacyparams.Subspace { return legacyparams.NewSubspace(s.key, "evm") },
			stored:    map[string]string{"feemarket/Denom": `"aphoton"`},
			expDenom:  "default",
			expHeight: -1,
		},
		{
			name:      "zero subspace reads nothing",
			subspace:  func() legacyparams.Subspace { return legacyparams.Subspace{} },
			stored:    map[string]string{"evm/Denom": `"aphoton"`},
			expDenom:  "default",
			expHeight: -1,
		},
		{
			name:        "undecodable value panics",
			subspace:    func() legacyparams.Subspace { return legacyparams.NewSubspace(s.key, "evm") },
			stored:      map[string]string{"evm/Height": `not json`},
			expectPanic: true,
		},
	}
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			for k, v := range tc.stored {
				s.ctx.KVStore(s.key).Set([]byte(k), []byte(v))
			}
			denom, height := "default", int64(-1)
			read := func() {
				tc.subspace().GetIfExists(s.ctx,
					legacyparams.Pair{Key: []byte("Denom"), Value: &denom},
					legacyparams.Pair{Key: []byte("Height"), Value: &height},
				)
			}
			if tc.expectPanic {
				s.Require().Panics(read)
				return
			}
			read()
			s.Require().Equal(tc.expDenom, denom)
			s.Require().Equal(tc.expHeight, height)
		})
	}
}
