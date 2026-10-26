package ante_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	errorsmod "cosmossdk.io/errors"
	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/mempool"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/holiman/uint256"
	"google.golang.org/protobuf/proto"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	"github.com/ethereum/go-ethereum/core"
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
		{
			"sender delegated via EIP-7702 is still an EOA",
			tx,
			func() {
				vmdb.SetCode(addr, ethtypes.AddressToDelegation(tests.GenerateAddress()), 0)
				vmdb.AddBalance(addr, uint256.NewInt(1000000), tracing.BalanceChangeTransfer)
			},
			true,
			true,
		},
		{
			"sender with non-delegation code is rejected",
			tx,
			func() {
				vmdb.SetCode(addr, []byte{0x60, 0x00, 0x60, 0x00, 0xfd}, 0)
			},
			true,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb = suite.StateDB()
			tc.malleate()
			suite.Require().NoError(vmdb.Commit())

			accountGetter := ante.NewCachedAccountGetter(suite.ctx, suite.app.AccountKeeper)
			err := ante.VerifyEthAccount(suite.ctx.WithIsCheckTx(tc.checkTx), tc.tx, suite.app.EvmKeeper, evmtypes.DefaultEVMDenom, accountGetter)

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
			err := ante.CheckAndSetEthSenderNonce(suite.ctx.WithIsReCheckTx(tc.reCheckTx), tc.tx, suite.app.AccountKeeper, false, accountGetter, nil)

			if tc.expPass {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *AnteTestSuite) TestEthTxReplacement() {
	suite.SetupTest()
	addr, privKey := tests.NewAddrKey()
	acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, addr.Bytes())
	// The pooled tx at nonce 0 and a later one were already admitted.
	suite.Require().NoError(acc.SetSequence(2))
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)
	suite.app.EvmKeeper.SetBalance(suite.ctx, addr, *uint256.NewInt(1e18), evmtypes.DefaultEVMDenom)

	newTx := func(gasPrice int64) sdk.Tx {
		msg := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 100000, big.NewInt(gasPrice), nil, nil, nil, nil)
		msg.From = addr.Bytes()
		return suite.CreateTestTx(msg, privKey, 1, false)
	}
	pooledTx, replacement := newTx(1e10), newTx(2e10)

	for _, tc := range []struct {
		name     string
		ctx      sdk.Context
		simulate bool
		pooled   bool
		expPass  bool
	}{
		{"CheckTx replaces a pooled tx", suite.ctx.WithIsCheckTx(true), false, true, true},
		{"CheckTx rejects an unpooled nonce", suite.ctx.WithIsCheckTx(true), false, false, false},
		{"Simulate accepts a pooled nonce", suite.ctx.WithIsCheckTx(true).WithExecMode(sdk.ExecModeSimulate), true, true, true},
		{"ReCheckTx rejects a pooled nonce", suite.ctx.WithIsReCheckTx(true), false, true, false},
		{"FinalizeBlock rejects a pooled nonce", suite.ctx.WithIsCheckTx(false).WithExecMode(sdk.ExecModeFinalize), false, true, false},
	} {
		suite.Run(tc.name, func() {
			if tc.pooled {
				suite.Require().NoError(suite.txPool.Insert(suite.ctx, pooledTx))
				defer func() { suite.Require().NoError(suite.txPool.Remove(pooledTx)) }()
			}
			_, err := suite.anteHandler(tc.ctx, replacement, tc.simulate)
			if !tc.expPass {
				suite.Require().ErrorIs(err, errortypes.ErrInvalidSequence)
				return
			}
			suite.Require().NoError(err)
			suite.Require().Equal(uint64(2), suite.app.AccountKeeper.GetAccount(tc.ctx, addr.Bytes()).GetSequence())
		})
	}

	// The ante lets the replacement through; the app pool's rule then requires a 10% fee bump.
	suite.Run("app pool requires a fee bump", func() {
		bumped := newTx(1.1e10)
		suite.Require().NoError(suite.txPool.Insert(suite.ctx, pooledTx))
		defer func() { suite.Require().NoError(suite.txPool.Remove(bumped)) }()

		suite.Require().ErrorIs(suite.txPool.Insert(suite.ctx, newTx(1.09e10)), mempool.ErrTxReplacement)
		suite.Require().NoError(suite.txPool.Insert(suite.ctx, bumped))
		pooled, ok := suite.txPool.GetTx(sdk.AccAddress(addr.Bytes()).String(), 0)
		suite.Require().True(ok)
		suite.Require().Equal(bumped, pooled)
	})

	for _, tc := range []struct {
		name    string
		balance uint64
	}{
		{"resending the pooled tx reports it as already pooled", 1e18},
		// checkState already paid the pooled tx's fee, so the balance checks alone would fail the resend.
		{"resending the pooled tx with the balance spent reports it as already pooled", 0},
	} {
		suite.Run(tc.name, func() {
			suite.Require().NoError(suite.txPool.Insert(suite.ctx, pooledTx))
			defer func() { suite.Require().NoError(suite.txPool.Remove(pooledTx)) }()
			suite.app.EvmKeeper.SetBalance(suite.ctx, addr, *uint256.NewInt(tc.balance), evmtypes.DefaultEVMDenom)
			defer suite.app.EvmKeeper.SetBalance(suite.ctx, addr, *uint256.NewInt(1e18), evmtypes.DefaultEVMDenom)

			_, err := suite.anteHandler(suite.ctx.WithIsCheckTx(true), pooledTx, false)
			suite.Require().ErrorIs(err, errortypes.ErrTxInMempoolCache)
		})
	}

	// The slow PrepareProposal path runs the ante inside SelectBy, which holds the pool lock.
	suite.Run("PrepareProposal ante inside the pool callback doesn't deadlock", func() {
		suite.Require().NoError(suite.txPool.Insert(suite.ctx, pooledTx))

		ctx := suite.ctx.WithIsCheckTx(false).WithExecMode(sdk.ExecModePrepareProposal)
		done := make(chan error, 1)
		go suite.txPool.SelectBy(ctx, nil, func(sdk.Tx) bool {
			_, err := suite.anteHandler(ctx, replacement, false)
			done <- err
			return false
		})
		select {
		case err := <-done:
			suite.Require().ErrorIs(err, errortypes.ErrInvalidSequence)
		case <-time.After(5 * time.Second):
			// No pool cleanup: the blocked SelectBy still holds the lock.
			suite.FailNow("ante blocked on the pool lock")
		}
		suite.Require().NoError(suite.txPool.Remove(pooledTx))
	})
}

// Each step runs through the app's CheckTx, so the ante, the pool insert and the error codes are covered together.
func (suite *AnteTestSuite) TestCheckTxEthTxReplacement() {
	suite.SetupTest()
	from := sdk.AccAddress(suite.priv.PubKey().Address().Bytes())
	newTx := func(nonce uint64, gasPrice int64) sdk.Tx {
		msg := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), nonce, big.NewInt(10), 100000, big.NewInt(gasPrice), nil, nil, nil, nil)
		msg.From = from
		return suite.CreateTestTx(msg, suite.priv, 1, false)
	}
	pooled, bumped := newTx(0, 1e10), newTx(0, 1.1e10)

	for _, step := range []struct {
		name   string
		tx     sdk.Tx
		expErr *errorsmod.Error // nil for success
		expTx  sdk.Tx           // pooled at nonce 0 afterwards
	}{
		{"admit", pooled, nil, pooled},
		{"resend", pooled, errortypes.ErrTxInMempoolCache, pooled},
		{"under the fee bump", newTx(0, 1.09e10), mempool.ErrTxReplacement, pooled},
		{"fee bump replaces", bumped, nil, bumped},
		// The replacement left the sequence at 1, so the next nonce is admitted.
		{"next nonce", newTx(1, 1e10), nil, bumped},
	} {
		bz, err := suite.clientCtx.TxConfig.TxEncoder()(step.tx)
		suite.Require().NoError(err)
		res, err := suite.app.CheckTx(&abci.RequestCheckTx{Tx: bz, Type: abci.CheckTxType_New})
		suite.Require().NoError(err)
		if step.expErr == nil {
			suite.Require().Equal(abci.CodeTypeOK, res.Code, "%s: %s", step.name, res.Log)
		} else {
			suite.Require().Equal(step.expErr.Codespace(), res.Codespace, "%s: %s", step.name, res.Log)
			suite.Require().Equal(step.expErr.ABCICode(), res.Code, "%s: %s", step.name, res.Log)
		}
		// The pool holds the decoded tx, so compare by hash.
		cur, ok := suite.txPool.GetTx(from.String(), 0)
		suite.Require().True(ok, step.name)
		suite.Require().Equal(step.expTx.GetMsgs()[0].(*evmtypes.MsgEthereumTx).Hash(), cur.GetMsgs()[0].(*evmtypes.MsgEthereumTx).Hash(), step.name)
	}
}

// The default and eth signer adapters must key one account's cosmos and eth txs alike, or the
// eth tx is admitted at the cosmos tx's nonce instead of rejected as an invalid nonce.
func (suite *AnteTestSuite) TestCheckTxEthTxAtPooledCosmosNonce() {
	suite.SetupTest()
	from := sdk.AccAddress(suite.priv.PubKey().Address().Bytes())
	cosmosTx, err := suite.createTx(suite.priv, &banktypes.MsgSend{
		FromAddress: from.String(),
		ToAddress:   from.String(),
		Amount:      sdk.NewCoins(sdk.NewInt64Coin(evmtypes.DefaultEVMDenom, 1)),
	})
	suite.Require().NoError(err)
	// Pooled directly, so checkState's sequence stays at the pooled nonce, as after a Commit reset.
	suite.Require().NoError(suite.txPool.Insert(suite.ctx, cosmosTx))
	_, ok := suite.txPool.GetTx(from.String(), 0)
	suite.Require().True(ok)

	msg := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(10), 100000, big.NewInt(1e10), nil, nil, nil, nil)
	msg.From = from
	bz, err := suite.clientCtx.TxConfig.TxEncoder()(suite.CreateTestTx(msg, suite.priv, 1, false))
	suite.Require().NoError(err)
	res, err := suite.app.CheckTx(&abci.RequestCheckTx{Tx: bz, Type: abci.CheckTxType_New})
	suite.Require().NoError(err)
	suite.Require().Equal(errortypes.ErrInvalidSequence.Codespace(), res.Codespace, res.Log)
	suite.Require().Equal(errortypes.ErrInvalidSequence.ABCICode(), res.Code, res.Log)
}

func (suite *AnteTestSuite) TestEthAnteHandlerBatchRules() {
	suite.SetupTest()
	addrA, privA := tests.NewAddrKey()
	msgA := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(1), 100000, big.NewInt(1e10), nil, nil, nil, nil)
	msgA.From = addrA.Bytes()
	msgB := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 0, big.NewInt(1), 100000, big.NewInt(1e10), nil, nil, nil, nil)
	msgB.From = tests.GenerateAddress().Bytes()
	builder := suite.CreateTestTxBuilder(msgA, privA, 1, false)
	suite.Require().NoError(builder.SetMsgs(msgA, msgB))

	for _, tc := range []struct {
		name     string
		ctx      sdk.Context
		simulate bool
		rejected bool
	}{
		{"CheckTx", suite.ctx.WithIsCheckTx(true), false, true},
		{"ReCheckTx", suite.ctx.WithIsCheckTx(true).WithIsReCheckTx(true), false, false},
		{"Simulate", suite.ctx.WithIsCheckTx(true).WithExecMode(sdk.ExecModeSimulate), true, false},
		{"FinalizeBlock", suite.ctx.WithIsCheckTx(false).WithExecMode(sdk.ExecModeFinalize), false, false},
	} {
		suite.Run(tc.name, func() {
			_, err := suite.anteHandler(tc.ctx, builder.GetTx(), tc.simulate)
			// Other checks may still fail in execution; only the batch rules are under test.
			isBatchErr := err != nil && strings.Contains(err.Error(), "must share a sender")
			suite.Require().Equal(tc.rejected, isBatchErr, "err: %v", err)
		})
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

	dynamicFeeTx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), tx2GasLimit,
		nil, // gasPrice
		new(big.Int).Add(baseFee, big.NewInt(evmtypes.DefaultPriorityReduction.Int64()*2)), // gasFeeCap
		evmtypes.DefaultPriorityReduction.BigInt(),                                         // gasTipCap
		nil, &ethtypes.AccessList{{Address: addr, StorageKeys: nil}})
	dynamicFeeTx.From = addr.Bytes()
	dynamicFeeTxPriority := int64(1)

	// Each message stays at or below the EIP-7825 MaxTxGas cap, but their sum exceeds the
	// block gas limit, so this still exercises the block-gas-limit check rather than the cap check.
	var overBlockGasLimitMsgs []sdk.Msg
	var overBlockGasLimitSum uint64
	for nonce := uint64(1); overBlockGasLimitSum <= blockGasLimit; nonce++ {
		msg := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), nonce, big.NewInt(10), params.MaxTxGas, gasPrice, nil, nil, nil, &ethtypes.AccessList{{Address: addr, StorageKeys: nil}})
		msg.From = addr.Bytes()
		overBlockGasLimitMsgs = append(overBlockGasLimitMsgs, msg)
		overBlockGasLimitSum += params.MaxTxGas
	}

	maxGasLimitTx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), math.MaxUint64, gasPrice, nil, nil, nil, &ethtypes.AccessList{{Address: addr, StorageKeys: nil}})
	maxGasLimitTx.From = addr.Bytes()

	overMaxTxGasLimit := params.MaxTxGas + 1
	overMaxTxGasTx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), overMaxTxGasLimit, gasPrice, nil, nil, nil, &ethtypes.AccessList{{Address: addr, StorageKeys: nil}})
	overMaxTxGasTx.From = addr.Bytes()

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
		errIs       error
	}{
		{"invalid transaction type", &invalidTx{}, math.MaxUint64, func() {}, false, false, 0, nil, nil},
		{
			"sender not found",
			evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil),
			math.MaxUint64,
			func() {},
			false, false,
			0,
			nil,
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
			nil,
		},
		{
			"gas limit above EIP-7825 MaxTxGas cap",
			overMaxTxGasTx,
			math.MaxUint64,
			func() {},
			false, false,
			0,
			fmt.Errorf("cap: %d, tx: %d", params.MaxTxGas, overMaxTxGasLimit),
			core.ErrGasLimitTooHigh,
		},
		{
			"not enough balance for fees",
			tx2,
			math.MaxUint64,
			func() {},
			false, false,
			0,
			nil,
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
			nil,
		},
		{
			"gas limit above block gas limit",
			&multiTx{Msgs: overBlockGasLimitMsgs},
			math.MaxUint64,
			func() {
				// Fund enough to cover fees for every message but the last, which trips the
				// block-gas-limit check before its own fee gets deducted.
				perMsgCost := uint256.NewInt(0).Mul(uint256.NewInt(params.MaxTxGas), uint256.MustFromBig(gasPrice))
				totalCost := uint256.NewInt(0).Mul(perMsgCost, uint256.NewInt(uint64(len(overBlockGasLimitMsgs))))
				vmdb.AddBalance(addr, totalCost, tracing.BalanceChangeTransfer)
			},
			false, false,
			0,
			fmt.Errorf("tx gas (%d) exceeds block gas limit (%d)", overBlockGasLimitSum, blockGasLimit),
			nil,
		},
		{
			"first message exceeds MaxTxGas cap in multi-msg tx",
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
			fmt.Errorf("cap: %d, tx: %d", params.MaxTxGas, maxGasLimitTx.GetGas()),
			core.ErrGasLimitTooHigh,
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
			nil,
		},
		{
			"gas limit above EIP-7825 MaxTxGas cap on ReCheckTx",
			overMaxTxGasTx,
			math.MaxUint64,
			func() {
				suite.ctx = suite.ctx.WithIsReCheckTx(true)
			},
			false, false,
			0,
			fmt.Errorf("cap: %d, tx: %d", params.MaxTxGas, overMaxTxGasLimit),
			core.ErrGasLimitTooHigh,
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
				if tc.errIs != nil {
					suite.Require().ErrorIs(err, tc.errIs)
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
					_ = ante.CheckAndSetEthSenderNonce(suite.ctx, tc.tx, suite.app.AccountKeeper, false, accountGetter, nil)
				})
				return
			}

			err := ante.CheckAndSetEthSenderNonce(suite.ctx, tc.tx, suite.app.AccountKeeper, false, accountGetter, nil)

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
