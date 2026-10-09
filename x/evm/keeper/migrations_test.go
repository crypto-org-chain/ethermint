package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/testutil"
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/suite"
)

type MigrateTestSuite struct {
	testutil.BaseTestSuite
}

func TestMigrateTestSuite(t *testing.T) {
	suite.Run(t, new(MigrateTestSuite))
}

func (suite *MigrateTestSuite) TestMigrations() {
	migrator := evmkeeper.NewMigrator(*suite.App.EvmKeeper, nil)

	testCases := []struct {
		name        string
		malleate    func()
		migrateFunc func(ctx sdk.Context) error
		postCheck   func()
	}{
		{
			name: "Run Migrate8to9",
			malleate: func() {
				// the default genesis already enabled compact storage; start from a pre-v9 store
				store := suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey))
				store.Delete(types.KeyPrefixCompactStorage)
				store.Delete(types.KeyPrefixStorageSweep)
			},
			migrateFunc: migrator.Migrate8to9,
			postCheck: func() {
				store := suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey))
				suite.Require().True(suite.App.EvmKeeper.IsStorageCompact(suite.Ctx))
				suite.Require().Equal(types.KeyPrefixStorage, store.Get(types.KeyPrefixStorageSweep))
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			tc.malleate()
			suite.Require().NoError(tc.migrateFunc(suite.Ctx))
			tc.postCheck()
		})
	}
}
