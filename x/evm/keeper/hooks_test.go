package keeper_test

import (
	"errors"
	"math/big"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/suite"

	"github.com/evmos/ethermint/testutil"
	"github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
)

type HookTestSuite struct {
	testutil.EVMTestSuiteWithAccount
}

func TestGenesisTestSuite(t *testing.T) {
	suite.Run(t, new(HookTestSuite))
}

// LogRecordHook records all the logs
type LogRecordHook struct {
	Logs []*ethtypes.Log
}

func (dh *LogRecordHook) PostTxProcessing(ctx sdk.Context, msg core.Message, receipt *ethtypes.Receipt) error {
	dh.Logs = receipt.Logs
	return nil
}

// FailureHook always fail
type FailureHook struct{}

func (dh FailureHook) PostTxProcessing(ctx sdk.Context, msg core.Message, receipt *ethtypes.Receipt) error {
	return errors.New("post tx processing failed")
}

func (suite *HookTestSuite) TestEvmHooks() {
	testCases := []struct {
		msg       string
		setupHook func() types.EvmHooks
		expFunc   func(hook types.EvmHooks, result error)
	}{
		{
			"log collect hook",
			func() types.EvmHooks {
				return &LogRecordHook{}
			},
			func(hook types.EvmHooks, result error) {
				suite.Require().NoError(result)
				suite.Require().Equal(1, len((hook.(*LogRecordHook).Logs)))
			},
		},
		{
			"always fail hook",
			func() types.EvmHooks {
				return &FailureHook{}
			},
			func(hook types.EvmHooks, result error) {
				suite.Require().Error(result)
			},
		},
	}

	for _, tc := range testCases {
		suite.EVMTestSuiteWithAccount.SetupTest()
		hook := tc.setupHook()
		suite.App.EvmKeeper.SetHooks(keeper.NewMultiEvmHooks(hook))

		k := suite.App.EvmKeeper
		txHash := common.BigToHash(big.NewInt(1))
		vmdb := statedb.New(suite.Ctx, k, statedb.NewTxConfig(
			common.BytesToHash(suite.Ctx.HeaderHash().Bytes()),
			txHash,
			0,
			0,
		))

		vmdb.AddLog(&ethtypes.Log{
			Topics:  []common.Hash{},
			Address: suite.Address,
		})
		logs := vmdb.Logs()
		receipt := &ethtypes.Receipt{
			TxHash: txHash,
			Logs:   logs,
		}
		result := k.PostTxProcessing(suite.Ctx, core.Message{}, receipt)

		tc.expFunc(hook, result)
	}
}
