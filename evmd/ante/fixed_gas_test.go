package ante_test

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cosmosante "github.com/evmos/ethermint/ante/cosmos"
	utiltx "github.com/evmos/ethermint/testutil/tx"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func (suite *AnteTestSuite) TestFixedGasConsumedDecorator() {
	decorator := cosmosante.NewFixedGasConsumedDecorator()

	addr := sdk.AccAddress(suite.priv.PubKey().Address().Bytes())

	valAddr := sdk.ValAddress(addr)

	valPubKey := ed25519.GenPrivKey().PubKey()

	testCases := []struct {
		name                string
		msgs                []sdk.Msg
		expectedFixedGas    uint64
		expectFixedGasMeter bool
	}{
		{
			name: "non-whitelisted message - MsgSend",
			msgs: []sdk.Msg{
				banktypes.NewMsgSend(
					addr,
					sdk.AccAddress([]byte("recipient_address")),
					sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(100))),
				),
			},
			expectedFixedGas:    0,
			expectFixedGasMeter: false,
		},
		{
			name: "non-whitelisted message - MsgUpdateParams",
			msgs: []sdk.Msg{
				&stakingtypes.MsgUpdateParams{
					Authority: suite.app.StakingKeeper.GetAuthority(),
					Params:    stakingtypes.DefaultParams(),
				},
			},
			expectedFixedGas:    0,
			expectFixedGasMeter: false,
		},
		{
			name: "non-whitelisted message - MsgDelegate",
			msgs: []sdk.Msg{
				stakingtypes.NewMsgDelegate(
					addr.String(),
					valAddr.String(),
					sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1000)),
				),
			},
			expectedFixedGas:    0,
			expectFixedGasMeter: false,
		},
		{
			name: "non-whitelisted message - MsgEditValidator",
			msgs: []sdk.Msg{
				stakingtypes.NewMsgEditValidator(
					valAddr.String(),
					stakingtypes.NewDescription("moniker", "identity", "website", "security", "details"),
					nil,
					nil,
				),
			},
			expectedFixedGas:    0,
			expectFixedGasMeter: false,
		},
		{
			name: "whitelisted message - MsgCreateValidator",
			msgs: []sdk.Msg{
				func() sdk.Msg {
					msg, _ := stakingtypes.NewMsgCreateValidator(
						valAddr.String(),
						valPubKey,
						sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1000)),
						stakingtypes.NewDescription("moniker", "identity", "website", "security", "details"),
						stakingtypes.NewCommissionRates(
							sdkmath.LegacyNewDecWithPrec(1, 1),
							sdkmath.LegacyNewDecWithPrec(2, 1),
							sdkmath.LegacyNewDecWithPrec(1, 2),
						),
						sdkmath.NewInt(1000),
					)
					return msg
				}(),
			},
			expectedFixedGas:    0,
			expectFixedGasMeter: false,
		},
		{
			name: "non-whitelisted message - MsgCancelUnbondingDelegation",
			msgs: []sdk.Msg{
				stakingtypes.NewMsgCancelUnbondingDelegation(
					addr.String(),
					valAddr.String(),
					1,
					sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1000)),
				),
			},
			expectedFixedGas:    0,
			expectFixedGasMeter: false,
		},
		{
			name: "whitelisted message - MsgUndelegate",
			msgs: []sdk.Msg{
				stakingtypes.NewMsgUndelegate(
					addr.String(),
					valAddr.String(),
					sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1000)),
				),
			},
			expectedFixedGas:    cosmosante.WhitelistedMessages[sdk.MsgTypeURL(&stakingtypes.MsgUndelegate{})],
			expectFixedGasMeter: true,
		},
		{
			name: "whitelisted message - MsgBeginRedelegate",
			msgs: []sdk.Msg{
				stakingtypes.NewMsgBeginRedelegate(
					addr.String(),
					valAddr.String(),
					valAddr.String(),
					sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1000)),
				),
			},
			expectedFixedGas:    cosmosante.WhitelistedMessages[sdk.MsgTypeURL(&stakingtypes.MsgBeginRedelegate{})],
			expectFixedGasMeter: true,
		},
	
		{
			name: "multiple whitelisted messages - MsgBeginRedelegate + MsgUndelegate",
			msgs: []sdk.Msg{
				stakingtypes.NewMsgBeginRedelegate(
					addr.String(),
					valAddr.String(),
					valAddr.String(),
					sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1000)),
				),
				stakingtypes.NewMsgUndelegate(
					addr.String(),
					valAddr.String(),
					sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(500)),
				),
			},
			expectedFixedGas:    cosmosante.WhitelistedMessages[sdk.MsgTypeURL(&stakingtypes.MsgBeginRedelegate{})] + cosmosante.WhitelistedMessages[sdk.MsgTypeURL(&stakingtypes.MsgUndelegate{})],
			expectFixedGasMeter: true,
		},
		{
			name: "mixed messages - whitelisted and non-whitelisted",
			msgs: []sdk.Msg{
				stakingtypes.NewMsgBeginRedelegate(
					addr.String(),
					valAddr.String(),
					valAddr.String(),
					sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1000)),
				),
				banktypes.NewMsgSend(
					addr,
					sdk.AccAddress([]byte("recipient_address")),
					sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(100))),
				),
			},
			expectedFixedGas:    cosmosante.WhitelistedMessages[sdk.MsgTypeURL(&stakingtypes.MsgBeginRedelegate{})], // additional fixed gas consumed should only be for the whitelisted one
			expectFixedGasMeter: true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest()

			args := utiltx.CosmosTxArgs{
				TxCfg:      suite.clientCtx.TxConfig,
				Priv:       suite.priv,
				Gas:        1000000,
				FeeGranter: addr,
				Msgs:       tc.msgs,
			}

			tx, err := utiltx.PrepareCosmosTx(suite.ctx, suite.app, args)
			suite.Require().NoError(err)

			gasLimit := uint64(1000000)
			gasMeter := storetypes.NewGasMeter(gasLimit)
			ctx := suite.ctx.WithGasMeter(gasMeter)

			// Record initial gas consumed (from any setup)
			initialGas := ctx.GasMeter().GasConsumed()

			newCtx, err := decorator.AnteHandle(ctx, tx, false, NextFn)

			suite.Require().NoError(err)

			// Check if fixed gas was consumed correctly
			gasConsumed := newCtx.GasMeter().GasConsumed()

			if tc.expectFixedGasMeter {
				// When fixed gas meter is used, the gas consumed should equal
				// the initial gas plus the expected fixed gas
				expectedTotalGas := initialGas + tc.expectedFixedGas
				suite.Require().Equal(expectedTotalGas, gasConsumed,
					"Expected gas consumed to be %d but got %d", expectedTotalGas, gasConsumed)

				// Verify that the gas meter is now a fixed gas meter
				// by trying to consume gas and checking it doesn't increase
				gasBefore := newCtx.GasMeter().GasConsumed()
				newCtx.GasMeter().ConsumeGas(1000, "test consume")
				gasAfter := newCtx.GasMeter().GasConsumed()
				suite.Require().Equal(gasBefore, gasAfter,
					"Fixed gas meter should not consume additional gas")

				// Verify the gas meter limit equals consumed amount
				suite.Require().Equal(gasConsumed, newCtx.GasMeter().Limit(),
					"Fixed gas meter limit should equal consumed gas")

				// Verify gas meter never reports out of gas
				suite.Require().False(newCtx.GasMeter().IsOutOfGas(),
					"Fixed gas meter should never be out of gas")
				suite.Require().False(newCtx.GasMeter().IsPastLimit(),
					"Fixed gas meter should never be past limit")
			} else {
				// When no fixed gas meter is used, the gas meter should still
				// be able to consume gas normally
				gasBefore := newCtx.GasMeter().GasConsumed()
				testGasAmount := uint64(1000)
				newCtx.GasMeter().ConsumeGas(testGasAmount, "test consume")
				gasAfter := newCtx.GasMeter().GasConsumed()
				suite.Require().Equal(gasBefore+testGasAmount, gasAfter,
					"Regular gas meter should consume gas normally")
			}
		})
	}
}

func (suite *AnteTestSuite) TestFixedGasMeter() {
	testCases := []struct {
		name           string
		consumedGas    uint64
		testOperations func(*AnteTestSuite, storetypes.GasMeter)
	}{
		{
			name:        "GasConsumed returns fixed amount",
			consumedGas: 100000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				s.Require().Equal(uint64(100000), gm.GasConsumed())
			},
		},
		{
			name:        "GasConsumedToLimit returns fixed amount",
			consumedGas: 100000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				s.Require().Equal(uint64(100000), gm.GasConsumedToLimit())
			},
		},
		{
			name:        "GasRemaining returns zero",
			consumedGas: 100000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				s.Require().Equal(uint64(0), gm.GasRemaining())
			},
		},
		{
			name:        "Limit equals consumed gas",
			consumedGas: 250000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				s.Require().Equal(uint64(250000), gm.Limit())
			},
		},
		{
			name:        "ConsumeGas does not change gas consumed",
			consumedGas: 100000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				before := gm.GasConsumed()
				gm.ConsumeGas(50000, "test")
				after := gm.GasConsumed()
				s.Require().Equal(before, after)
			},
		},
		{
			name:        "RefundGas does not change gas consumed",
			consumedGas: 100000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				before := gm.GasConsumed()
				gm.RefundGas(10000, "test")
				after := gm.GasConsumed()
				s.Require().Equal(before, after)
			},
		},
		{
			name:        "IsPastLimit always returns false",
			consumedGas: 100000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				s.Require().False(gm.IsPastLimit())
			},
		},
		{
			name:        "IsOutOfGas always returns false",
			consumedGas: 100000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				s.Require().False(gm.IsOutOfGas())
			},
		},
		{
			name:        "String returns correct identifier",
			consumedGas: 100000,
			testOperations: func(s *AnteTestSuite, gm storetypes.GasMeter) {
				s.Require().Equal("fixedGasMeter", gm.String())
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			gm := cosmosante.NewFixedGasMeter(tc.consumedGas)
			tc.testOperations(suite, gm)
		})
	}
}
