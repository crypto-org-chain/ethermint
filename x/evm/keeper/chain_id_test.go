package keeper_test

import "math/big"

func (suite *KeeperTestSuite) TestWithChainIDStringIdempotent() {
	suite.SetupTest()

	want := suite.App.EvmKeeper.ChainID()
	suite.Require().NotNil(want)

	// Re-setting the same value must not panic and must leave the ID unchanged.
	suite.Require().NotPanics(func() {
		suite.App.EvmKeeper.WithChainIDString("ethermint_9000-1")
	})
	suite.Require().Equal(0, want.Cmp(suite.App.EvmKeeper.ChainID()))

	// A different value must still be rejected.
	suite.Require().Panics(func() {
		suite.App.EvmKeeper.WithChainIDString("ethermint_8888-1")
	})
	// The original ID must survive the rejected change.
	suite.Require().Equal(0, big.NewInt(9000).Cmp(suite.App.EvmKeeper.ChainID()))
}
