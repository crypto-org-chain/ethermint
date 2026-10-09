package keeper_test

import (
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/types"
)

func (suite *KeeperTestSuite) TestMigrate8to9() {
	store := suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey))
	// the default genesis already enabled compact storage; start from a pre-v9 store
	store.Delete(types.KeyPrefixCompactStorage)

	suite.Require().NoError(evmkeeper.NewMigrator(*suite.App.EvmKeeper, nil).Migrate8to9(suite.Ctx))
	suite.Require().True(suite.App.EvmKeeper.IsStorageCompact(suite.Ctx))
	suite.Require().Equal(types.KeyPrefixStorage, store.Get(types.KeyPrefixStorageSweep))
}
