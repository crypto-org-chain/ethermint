package ante_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/evmos/ethermint/ante/cache"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/holiman/uint256"
	"google.golang.org/protobuf/proto"

	storetypes "cosmossdk.io/store/types"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/tests"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func (suite *AnteTestSuite) TestNewEthAccountVerificationDecorator() {
	addr := tests.GenerateAddress()

	tx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx.From = addr.Bytes()

	var vmdb *statedb.StateDB

	testCases := []struct {
		name     string
		tx       sdk.Tx
		malleate func()
		checkTx  bool
		expPass  bool
	}{
		{"invalid transaction type", &invalidTx{}, func() {}, true, false},
		{
			"not CheckTx still rejects insufficient balance",
			tx,
			func() { vmdb.SetCode(addr, nil, 0) },
			false,
			false,
		},
		{
			"sender not set to msg",
			evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil),
			func() {},
			true,
			false,
		},
		{
			"sender not EOA",
			tx,
			func() {
				// set not as an EOA
				vmdb.SetCode(addr, []byte("1"), 0)
			},
			true,
			false,
		},
		{
			"not enough balance to cover tx cost",
			tx,
			func() {
				// reset back to EOA
				vmdb.SetCode(addr, nil, 0)
			},
			true,
			false,
		},
		{
			"success new account",
			tx,
			func() {
				vmdb.AddBalance(addr, uint256.NewInt(1000000), tracing.BalanceChangeTransfer)
			},
			true,
			true,
		},
		{
			"success existing account",
			tx,
			func() {
				acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
				suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

				vmdb.AddBalance(addr, uint256.NewInt(1000000), tracing.BalanceChangeTransfer)
			},
			true,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb = suite.StateDB()
			tc.malleate()
			suite.Require().NoError(vmdb.Commit())

			accountGetter := ante.NewCachedAccountGetter(suite.ctx, suite.app.AccountKeeper)
			rules := params.Rules{
				IsPrague: false,
			}
			err := ante.VerifyEthAccount(suite.ctx.WithIsCheckTx(tc.checkTx), tc.tx, suite.app.EvmKeeper, evmtypes.DefaultEVMDenom, accountGetter, rules)

			if tc.expPass {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *AnteTestSuite) TestEthNonceVerificationDecorator() {
	suite.SetupTest()

	addr := tests.GenerateAddress()

	tx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx.From = addr.Bytes()

	testCases := []struct {
		name      string
		tx        sdk.Tx
		malleate  func()
		reCheckTx bool
		expPass   bool
	}{
		{"ReCheckTx", &invalidTx{}, func() {}, true, false},
		{"invalid transaction type", &invalidTx{}, func() {}, false, false},
		{"sender account not found", tx, func() {}, false, false},
		{
			"sender nonce missmatch",
			tx,
			func() {
				acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
				suite.app.AccountKeeper.SetAccount(suite.ctx, acc)
			},
			false,
			false,
		},
		{
			"success",
			tx,
			func() {
				acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
				suite.Require().NoError(acc.SetSequence(1))
				suite.app.AccountKeeper.SetAccount(suite.ctx, acc)
			},
			false,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			tc.malleate()
			accountGetter := ante.NewCachedAccountGetter(suite.ctx, suite.app.AccountKeeper)
			_, err := ante.CheckAndSetEthSenderNonce(suite.ctx.WithIsReCheckTx(tc.reCheckTx), tc.tx, suite.app.AccountKeeper, false, accountGetter, cache.NewAnteCache(0))

			if tc.expPass {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

// verifies that CheckAndSetEthSenderNonce returns pending
// entries instead of immediately setting the cache. When pending entries are
// committed (after the full ante chain succeeds), the cache is properly updated
// to enable transaction replacement.
func (suite *AnteTestSuite) TestEthNonceCacheUpdatedDuringCheckTx() {
	suite.SetupTest()

	addr := tests.GenerateAddress()
	tx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx.From = addr.Bytes()

	acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

	ctx := suite.ctx.WithIsCheckTx(true)
	anteCache := cache.NewAnteCache(0)
	accountGetter := ante.NewCachedAccountGetter(ctx, suite.app.AccountKeeper)

	pending, err := ante.CheckAndSetEthSenderNonce(ctx, tx, suite.app.AccountKeeper, false, accountGetter, anteCache)
	suite.Require().NoError(err)

	fromStr := sdk.AccAddress(addr.Bytes()).String()
	suite.Require().False(anteCache.Exists(fromStr, tx.AsTransaction().Nonce()),
		"CheckTx should only stage entries until the ante chain succeeds")

	commitPendingEntries(anteCache, pending)

	suite.Require().True(anteCache.Exists(fromStr, tx.AsTransaction().Nonce()),
		"nonce cache should be updated during CheckTx to enable replacement transactions")
}

// staged entries must be dropped if later ante decorators fail.
func (suite *AnteTestSuite) TestEthNonceCacheLeakOnAnteFailure() {
	suite.SetupTest()

	addr := tests.GenerateAddress()
	tx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx.From = addr.Bytes()

	acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

	ctx := suite.ctx.WithIsCheckTx(true)
	anteCache := cache.NewAnteCache(0)
	accountGetter := ante.NewCachedAccountGetter(ctx, suite.app.AccountKeeper)

	pending, err := ante.CheckAndSetEthSenderNonce(ctx, tx, suite.app.AccountKeeper, false, accountGetter, anteCache)
	suite.Require().NoError(err)

	fromStr := sdk.AccAddress(addr.Bytes()).String()
	suite.Require().False(anteCache.Exists(fromStr, tx.AsTransaction().Nonce()),
		"nonce cache should be cleared when later ante decorators reject the tx")
	suite.Require().Len(pending, 1)
}

// Replacement scenario: once CheckTx stages an entry and the handler commits
// it, a second transaction with the same nonce should bypass validation via
// the cache shortcut.
func (suite *AnteTestSuite) TestEthNonceCacheBypassesValidationOnSecondTx() {
	suite.SetupTest()

	addr := tests.GenerateAddress()
	tx1 := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx1.From = addr.Bytes()

	tx2 := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx2.From = addr.Bytes()

	acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

	ctx := suite.ctx.WithIsCheckTx(true)
	anteCache := cache.NewAnteCache(0)
	accountGetter := ante.NewCachedAccountGetter(ctx, suite.app.AccountKeeper)

	nonce := tx1.AsTransaction().Nonce()
	suite.Require().Equal(nonce, tx2.AsTransaction().Nonce())

	// Stage the first tx; cache must remain untouched
	pending, err := ante.CheckAndSetEthSenderNonce(ctx, tx1, suite.app.AccountKeeper, false, accountGetter, anteCache)
	suite.Require().NoError(err)
	suite.Require().Len(pending, 1)

	fromStr := sdk.AccAddress(addr.Bytes()).String()
	suite.Require().False(anteCache.Exists(fromStr, nonce),
		"cache should not contain nonce until pending entries are committed")

	// Simulate successful ante chain by committing staged entry
	commitPendingEntries(anteCache, pending)
	suite.Require().True(anteCache.Exists(fromStr, nonce))

	// Replacement tx with same nonce should hit the cache shortcut
	accountGetter = ante.NewCachedAccountGetter(ctx, suite.app.AccountKeeper)
	_, err = ante.CheckAndSetEthSenderNonce(ctx, tx2, suite.app.AccountKeeper, false, accountGetter, anteCache)

	suite.Require().NoError(err, "cache shortcut should allow tx replacement when nonce is in cache")
}

// DeliverTx (simulated here by ctx with IsCheckTx=false) should remove any
// pending nonce markers so the cache mirrors committed state.
func (suite *AnteTestSuite) TestEthNonceCacheClearedOnDeliverTx() {
	suite.SetupTest()

	addr := tests.GenerateAddress()
	tx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx.From = addr.Bytes()

	acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

	ctx := suite.ctx.WithIsCheckTx(true)
	anteCache := cache.NewAnteCache(0)
	accountGetter := ante.NewCachedAccountGetter(ctx, suite.app.AccountKeeper)

	pending, err := ante.CheckAndSetEthSenderNonce(ctx, tx, suite.app.AccountKeeper, false, accountGetter, anteCache)
	suite.Require().NoError(err)
	commitPendingEntries(anteCache, pending)

	fromStr := sdk.AccAddress(addr.Bytes()).String()
	suite.Require().True(anteCache.Exists(fromStr, tx.AsTransaction().Nonce()))

	acc = suite.app.AccountKeeper.GetAccount(suite.ctx, addr.Bytes())
	suite.Require().NoError(acc.SetSequence(0))
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

	deliverCtx := suite.ctx.WithIsCheckTx(false)
	deliverAccountGetter := ante.NewCachedAccountGetter(deliverCtx, suite.app.AccountKeeper)

	_, err = ante.CheckAndSetEthSenderNonce(deliverCtx, tx, suite.app.AccountKeeper, false, deliverAccountGetter, anteCache)
	suite.Require().NoError(err)
	suite.Require().False(anteCache.Exists(fromStr, tx.AsTransaction().Nonce()),
		"cache entry should be cleared after DeliverTx")
}

// ReCheckTx should not reinsert the nonce into the cache once a previous
// DeliverTx/cleanup removed it; this test enforces that we keep the cache
// empty after the recheck run.
func (suite *AnteTestSuite) TestEthNonceCacheRecheckDoesNotPollutesCache() {
	suite.SetupTest()

	addr := tests.GenerateAddress()
	tx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx.From = addr.Bytes()

	acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

	ctx := suite.ctx.WithIsCheckTx(true)
	anteCache := cache.NewAnteCache(0)
	accountGetter := ante.NewCachedAccountGetter(ctx, suite.app.AccountKeeper)

	pending, err := ante.CheckAndSetEthSenderNonce(ctx, tx, suite.app.AccountKeeper, false, accountGetter, anteCache)
	suite.Require().NoError(err)
	commitPendingEntries(anteCache, pending)

	fromStr := sdk.AccAddress(addr.Bytes()).String()
	anteCache.Delete(fromStr, tx.AsTransaction().Nonce())

	acc = suite.app.AccountKeeper.GetAccount(suite.ctx, addr.Bytes())
	suite.Require().NoError(acc.SetSequence(0))
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

	recheckCtx := ctx.WithIsReCheckTx(true)
	recheckAccountGetter := ante.NewCachedAccountGetter(recheckCtx, suite.app.AccountKeeper)

	_, err = ante.CheckAndSetEthSenderNonce(recheckCtx, tx, suite.app.AccountKeeper, false, recheckAccountGetter, anteCache)
	suite.Require().NoError(err)

	suite.Require().False(anteCache.Exists(fromStr, tx.AsTransaction().Nonce()),
		"ReCheckTx should not repopulate the cache once the entry was cleared")
}

// Mirrors the production ante handler logic in handler_options.go: staged
// entries are flushed into the shared cache only after the full CheckTx ante
// stack succeeds.
func commitPendingEntries(c *cache.AnteCache, entries []cache.TxNonce) {
	// Tests bypass the actual handler by writing staged entries directly once the
	// simulated ante chain “succeeds”.
	for _, entry := range entries {
		c.Set(entry.Address, entry.Nonce)
	}
}

type multiTx struct {
	Msgs []sdk.Msg
}

func (msg *multiTx) GetMsgs() []sdk.Msg {
	return msg.Msgs
}

func (msg *multiTx) GetMsgsV2() ([]proto.Message, error) {
	return nil, errors.New("not implemented")
}

func (suite *AnteTestSuite) TestEthGasConsumeDecorator() {
	evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
	chainID := suite.app.EvmKeeper.ChainID()
	chainCfg := evmParams.GetChainConfig()
	ethCfg := chainCfg.EthereumConfig(chainID)
	baseFee := suite.app.EvmKeeper.GetBaseFee(suite.ctx, ethCfg)
	rules := ethCfg.Rules(big.NewInt(suite.ctx.BlockHeight()), ethCfg.MergeNetsplitBlock != nil, uint64(suite.ctx.BlockHeader().Time.Unix()))

	addr := tests.GenerateAddress()

	blockGasLimit := ethermint.BlockGasLimit(suite.ctx)
	txGasLimit := uint64(1000)
	tx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), txGasLimit, big.NewInt(1), nil, nil, nil, nil)
	tx.From = addr.Bytes()

	suite.Require().Equal(int64(765625000), baseFee.Int64())

	gasPrice := new(big.Int).Add(baseFee, evmtypes.DefaultPriorityReduction.BigInt())

	tx2GasLimit := uint64(1000000)
	tx2 := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), tx2GasLimit, gasPrice, nil, nil, nil, &ethtypes.AccessList{{Address: addr, StorageKeys: nil}})
	tx2.From = addr.Bytes()
	tx2Priority := int64(1)

	tx3GasLimit := blockGasLimit + uint64(1)
	tx3 := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), tx3GasLimit, gasPrice, nil, nil, nil, &ethtypes.AccessList{{Address: addr, StorageKeys: nil}})

	dynamicFeeTx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), tx2GasLimit,
		nil, // gasPrice
		new(big.Int).Add(baseFee, big.NewInt(evmtypes.DefaultPriorityReduction.Int64()*2)), // gasFeeCap
		evmtypes.DefaultPriorityReduction.BigInt(),                                         // gasTipCap
		nil, &ethtypes.AccessList{{Address: addr, StorageKeys: nil}})
	dynamicFeeTx.From = addr.Bytes()
	dynamicFeeTxPriority := int64(1)

	maxGasLimitTx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), math.MaxUint64, gasPrice, nil, nil, nil, &ethtypes.AccessList{{Address: addr, StorageKeys: nil}})
	maxGasLimitTx.From = addr.Bytes()

	var vmdb *statedb.StateDB

	testCases := []struct {
		name        string
		tx          sdk.Tx
		gasLimit    uint64
		malleate    func()
		expPass     bool
		expPanic    bool
		expPriority int64
		err         error
	}{
		{"invalid transaction type", &invalidTx{}, math.MaxUint64, func() {}, false, false, 0, nil},
		{
			"sender not found",
			evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil),
			math.MaxUint64,
			func() {},
			false, false,
			0,
			nil,
		},
		{
			"gas limit too low",
			tx,
			math.MaxUint64,
			func() {},
			false, false,
			0,
			nil,
		},
		{
			"gas limit above block gas limit",
			tx3,
			math.MaxUint64,
			func() {},
			false, false,
			0,
			nil,
		},
		{
			"not enough balance for fees",
			tx2,
			math.MaxUint64,
			func() {},
			false, false,
			0,
			nil,
		},
		{
			"not enough tx gas",
			tx2,
			0,
			func() {
				vmdb.AddBalance(addr, uint256.NewInt(1000000), tracing.BalanceChangeTransfer)
			},
			false, true,
			0,
			nil,
		},
		{
			"not enough block gas",
			tx2,
			0,
			func() {
				vmdb.AddBalance(addr, uint256.NewInt(1000000), tracing.BalanceChangeTransfer)
				suite.ctx = suite.ctx.WithBlockGasMeter(storetypes.NewGasMeter(1))
			},
			false, true,
			0,
			nil,
		},
		{
			"gas limit overflow",
			&multiTx{
				Msgs: []sdk.Msg{maxGasLimitTx, tx2},
			},
			math.MaxUint64,
			func() {
				limit := uint256.NewInt(math.MaxUint64)
				gasPrice := uint256.MustFromBig(gasPrice)
				balance := uint256.NewInt(0).Mul(limit, gasPrice)

				vmdb.AddBalance(addr, balance, tracing.BalanceChangeTransfer)
			},
			false, false,
			0,
			fmt.Errorf("tx gas (%d) exceeds block gas limit (%d)", maxGasLimitTx.GetGas(), blockGasLimit),
		},
		{
			"success - legacy tx",
			tx2,
			tx2GasLimit, // it's capped
			func() {
				vmdb.AddBalance(addr, uint256.NewInt(1001000000000000), tracing.BalanceChangeTransfer)
				suite.ctx = suite.ctx.WithBlockGasMeter(storetypes.NewGasMeter(10000000000000000000))
			},
			true, false,
			tx2Priority,
			nil,
		},
		{
			"success - dynamic fee tx",
			dynamicFeeTx,
			tx2GasLimit, // it's capped
			func() {
				vmdb.AddBalance(addr, uint256.NewInt(1001000000000000), tracing.BalanceChangeTransfer)
				suite.ctx = suite.ctx.WithBlockGasMeter(storetypes.NewGasMeter(10000000000000000000))
			},
			true, false,
			dynamicFeeTxPriority,
			nil,
		},
		{
			"success - gas limit on gasMeter is set on ReCheckTx mode",
			dynamicFeeTx,
			tx2GasLimit, // it's capped
			func() {
				vmdb.AddBalance(addr, uint256.NewInt(1001000000000000), tracing.BalanceChangeTransfer)
				suite.ctx = suite.ctx.WithIsReCheckTx(true)
			},
			true, false,
			1,
			nil,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb = suite.StateDB()
			tc.malleate()
			suite.Require().NoError(vmdb.Commit())

			if tc.expPanic {
				suite.Require().Panics(func() {
					_, _ = ante.CheckEthGasConsume(
						suite.ctx.WithIsCheckTx(true).WithGasMeter(storetypes.NewGasMeter(1)), tc.tx,
						rules, suite.app.EvmKeeper, baseFee, evmtypes.DefaultEVMDenom,
					)
				})
				return
			}

			ctx, err := ante.CheckEthGasConsume(
				suite.ctx.WithIsCheckTx(true).WithGasMeter(storetypes.NewInfiniteGasMeter()), tc.tx,
				rules, suite.app.EvmKeeper, baseFee, evmtypes.DefaultEVMDenom,
			)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expPriority, ctx.Priority())
			} else {
				if tc.err != nil {
					suite.Require().ErrorContains(err, tc.err.Error())
				} else {
					suite.Require().Error(err)
				}
			}
			suite.Require().Equal(tc.gasLimit, ctx.GasMeter().Limit())
		})
	}
}

func (suite *AnteTestSuite) TestCanTransferDecorator() {
	addr, privKey := tests.NewAddrKey()
	suite.app.FeeMarketKeeper.SetBaseFee(suite.ctx, big.NewInt(100))

	evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
	chainID := suite.app.EvmKeeper.ChainID()
	chainCfg := evmParams.GetChainConfig()
	ethCfg := chainCfg.EthereumConfig(chainID)
	baseFee := suite.app.EvmKeeper.GetBaseFee(suite.ctx, ethCfg)
	rules := ethCfg.Rules(big.NewInt(suite.ctx.BlockHeight()), ethCfg.MergeNetsplitBlock != nil, uint64(suite.ctx.BlockHeader().Time.Unix()))

	tx := evmtypes.NewTxContract(
		suite.app.EvmKeeper.ChainID(),
		1,
		big.NewInt(10),
		1000,
		big.NewInt(150),
		big.NewInt(200),
		nil,
		nil,
		&ethtypes.AccessList{},
	)
	tx2 := evmtypes.NewTxContract(
		suite.app.EvmKeeper.ChainID(),
		1,
		big.NewInt(10),
		1000,
		big.NewInt(150),
		big.NewInt(200),
		nil,
		nil,
		&ethtypes.AccessList{},
	)
	tx3 := evmtypes.NewTxContract(
		suite.app.EvmKeeper.ChainID(),
		1,
		big.NewInt(-10),
		1000,
		big.NewInt(150),
		big.NewInt(200),
		nil,
		nil,
		&ethtypes.AccessList{},
	)

	for _, tx := range []*evmtypes.MsgEthereumTx{tx, tx3} {
		tx.From = addr.Bytes()

		err := tx.Sign(suite.ethSigner, tests.NewSigner(privKey))
		suite.Require().NoError(err)
	}

	var vmdb *statedb.StateDB

	testCases := []struct {
		name     string
		tx       sdk.Tx
		malleate func()
		expPass  bool
	}{
		{"invalid transaction type", &invalidTx{}, func() {}, false},
		{"AsMessage failed", tx2, func() {}, false},
		{"negative value", tx3, func() {}, false},
		{
			"evm CanTransfer failed",
			tx,
			func() {
				acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
				suite.app.AccountKeeper.SetAccount(suite.ctx, acc)
			},
			false,
		},
		{
			"success",
			tx,
			func() {
				acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
				suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

				vmdb.AddBalance(addr, uint256.NewInt(1000000), tracing.BalanceChangeTransfer)
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb = suite.StateDB()
			tc.malleate()
			suite.Require().NoError(vmdb.Commit())

			err := ante.CheckEthCanTransfer(
				suite.ctx.WithIsCheckTx(true), tc.tx,
				baseFee, rules, suite.app.EvmKeeper, &evmParams,
			)

			if tc.expPass {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *AnteTestSuite) TestEthIncrementSenderSequenceDecorator() {
	addr, privKey := tests.NewAddrKey()

	contract := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	contract.From = addr.Bytes()
	err := contract.Sign(suite.ethSigner, tests.NewSigner(privKey))
	suite.Require().NoError(err)

	to := tests.GenerateAddress()
	tx := evmtypes.NewTx(suite.app.EvmKeeper.ChainID(), 0, &to, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx.From = addr.Bytes()
	err = tx.Sign(suite.ethSigner, tests.NewSigner(privKey))
	suite.Require().NoError(err)

	tx2 := evmtypes.NewTx(suite.app.EvmKeeper.ChainID(), 1, &to, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	tx2.From = addr.Bytes()
	err = tx2.Sign(suite.ethSigner, tests.NewSigner(privKey))
	suite.Require().NoError(err)

	testCases := []struct {
		name     string
		tx       sdk.Tx
		malleate func()
		expPass  bool
		expPanic bool
	}{
		{
			"invalid transaction type",
			&invalidTx{},
			func() {},
			false, false,
		},
		{
			"no signers",
			evmtypes.NewTx(suite.app.EvmKeeper.ChainID(), 1, &to, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil),
			func() {},
			false, false,
		},
		{
			"account not set to store",
			tx,
			func() {},
			true, false,
		},
		{
			"success - create contract",
			contract,
			func() {
				acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
				suite.app.AccountKeeper.SetAccount(suite.ctx, acc)
			},
			true, false,
		},
		{
			"success - call",
			tx2,
			func() {},
			true, false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			tc.malleate()
			accountGetter := ante.NewCachedAccountGetter(suite.ctx, suite.app.AccountKeeper)

			if tc.expPanic {
				suite.Require().Panics(func() {
					_, _ = ante.CheckAndSetEthSenderNonce(suite.ctx, tc.tx, suite.app.AccountKeeper, false, accountGetter, cache.NewAnteCache(0))
				})
				return
			}

			_, err := ante.CheckAndSetEthSenderNonce(suite.ctx, tc.tx, suite.app.AccountKeeper, false, accountGetter, cache.NewAnteCache(0))

			if tc.expPass {
				suite.Require().NoError(err)
				msg := tc.tx.(*evmtypes.MsgEthereumTx)

				txData := msg.AsTransaction()
				suite.Require().NotNil(txData)

				nonce := suite.app.EvmKeeper.GetNonce(suite.ctx, addr)
				suite.Require().Equal(txData.Nonce()+1, nonce)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}
