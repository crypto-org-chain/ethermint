package keeper_test

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/x/evm/types"
)

func (suite *KeeperTestSuite) TestMigrate8to9() {
	store := suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey))
	// the default genesis already enabled compact storage; start from a pre-v9 store
	store.Delete(types.KeyPrefixCompactStorage)

	addr := common.BigToAddress(big.NewInt(1))
	zeroKey, keptKey := common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2))
	keptValue := common.BigToHash(big.NewInt(7))
	store.Set(types.StateKey(addr, zeroKey), make([]byte, common.HashLength))
	store.Set(types.StateKey(addr, keptKey), keptValue.Bytes())

	fromVM := suite.App.ModuleManager.GetVersionMap()
	fromVM[types.ModuleName] = 8
	_, err := suite.App.ModuleManager.RunMigrations(suite.Ctx, suite.App.Configurator(), fromVM)
	suite.Require().NoError(err)
	suite.Require().True(suite.App.EvmKeeper.IsStorageCompact(suite.Ctx))
	suite.Require().Equal(types.KeyPrefixStorage, store.Get(types.KeyPrefixStorageSweep))

	// the sweep deletes the legacy zero slot and leaves the padded non-zero slot as is
	suite.Require().NoError(suite.App.EvmKeeper.BeginBlock(suite.Ctx))
	suite.Require().Nil(store.Get(types.StateKey(addr, zeroKey)))
	suite.Require().Equal(keptValue.Bytes(), store.Get(types.StateKey(addr, keptKey)))
	suite.Require().Nil(store.Get(types.KeyPrefixStorageSweep))

	// later writes use the compact format
	vmdb := suite.StateDB()
	vmdb.SetState(addr, keptKey, common.BigToHash(big.NewInt(8)))
	suite.Require().NoError(vmdb.Commit())
	suite.Require().Equal([]byte{8}, store.Get(types.StateKey(addr, keptKey)))
}
