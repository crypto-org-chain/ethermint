package keeper_test

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/x/evm/types"
)

func (suite *KeeperTestSuite) TestMigrate8to9() {
	k := suite.App.EvmKeeper
	store := suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey))
	// start from pre-v9 params
	suite.UpdateEVMParams(suite.T(), func(p *types.Params) { p.CompactStorage = false })
	// config cached earlier in the upgrade block
	_, err := k.EVMBlockConfig(suite.Ctx, k.ChainID())
	suite.Require().NoError(err)
	suite.Require().False(k.IsStorageCompactForTest(suite.Ctx))

	addr := common.BigToAddress(big.NewInt(1))
	zeroKey, keptKey := common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2))
	keptValue := common.BigToHash(big.NewInt(7))
	store.Set(types.StateKey(addr, zeroKey), make([]byte, common.HashLength))
	store.Set(types.StateKey(addr, keptKey), keptValue.Bytes())

	fromVM := suite.App.ModuleManager.GetVersionMap()
	fromVM[types.ModuleName] = 8
	_, err = suite.App.ModuleManager.RunMigrations(suite.Ctx, suite.App.Configurator(), fromVM)
	suite.Require().NoError(err)
	suite.Require().True(k.IsStorageCompactForTest(suite.Ctx))
	suite.Require().Equal(types.KeyPrefixStorage, store.Get(types.KeyPrefixStorageSweep))

	suite.Require().NoError(suite.App.EvmKeeper.BeginBlock(suite.Ctx))
	suite.Require().Nil(store.Get(types.StateKey(addr, zeroKey)))
	suite.Require().Equal(keptValue.Bytes(), store.Get(types.StateKey(addr, keptKey)))
	suite.Require().Nil(store.Get(types.KeyPrefixStorageSweep))

	vmdb := suite.StateDB()
	vmdb.SetState(addr, keptKey, common.BigToHash(big.NewInt(8)))
	suite.Require().NoError(vmdb.Commit())
	suite.Require().Equal([]byte{8}, store.Get(types.StateKey(addr, keptKey)))
}
