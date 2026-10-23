package ante_test

import (
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/mempool"

	ante "github.com/evmos/ethermint/evmd/ante"
)

func (suite *AnteTestSuite) TestAnteHandlerRejectsTypedNilTxPool() {
	suite.SetupTest()
	var pool *mempool.PriorityNonceMempool[int64]
	_, err := ante.NewAnteHandler(ante.HandlerOptions{
		AccountKeeper:   suite.app.AccountKeeper,
		BankKeeper:      suite.app.BankKeeper,
		SignModeHandler: suite.clientCtx.TxConfig.SignModeHandler(),
		FeeMarketKeeper: suite.app.FeeMarketKeeper,
		EvmKeeper:       suite.app.EvmKeeper,
		TxPool:          pool,
	})
	suite.Require().ErrorIs(err, errortypes.ErrLogic)
}
