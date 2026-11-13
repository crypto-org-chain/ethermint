package evmd_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	connectiontypes "github.com/cosmos/ibc-go/v11/modules/core/03-connection/types"
	channelv11 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/migrations/v11"
	channeltypes "github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
	commitmenttypes "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types"
	ibcexported "github.com/cosmos/ibc-go/v11/modules/core/exported"

	"github.com/evmos/ethermint/upgrades/sdk55"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

var _keyRotationPoolAddr = authtypes.NewModuleAddress(stakingtypes.KeyRotationFeePoolName)

type UpgradesTestSuite struct {
	appTestSuite
}

func TestUpgradesTestSuite(t *testing.T) {
	suite.Run(t, new(UpgradesTestSuite))
}

// Genesis creates the pool; an upgraded chain starts without it.
func (s *UpgradesTestSuite) removeKeyRotationPool() {
	s.app.AccountKeeper.RemoveAccount(s.ctx, s.app.AccountKeeper.GetAccount(s.ctx, _keyRotationPoolAddr))
}

// Anyone can create a plain account at the pool address before the upgrade, e.g. by an EVM transfer.
func (s *UpgradesTestSuite) squatKeyRotationPool() sdk.Coins {
	s.removeKeyRotationPool()
	coins := sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(7)))
	s.Require().NoError(s.app.BankKeeper.MintCoins(s.ctx, evmtypes.ModuleName, coins))
	s.Require().NoError(s.app.BankKeeper.SendCoins(s.ctx, authtypes.NewModuleAddress(evmtypes.ModuleName), _keyRotationPoolAddr, coins))
	_, isModule := s.app.AccountKeeper.GetAccount(s.ctx, _keyRotationPoolAddr).(sdk.ModuleAccountI)
	s.Require().False(isModule)
	return coins
}

func (s *UpgradesTestSuite) requireKeyRotationPool(balance sdk.Coins) {
	name := stakingtypes.KeyRotationFeePoolName
	macc, ok := s.app.AccountKeeper.GetAccount(s.ctx, _keyRotationPoolAddr).(sdk.ModuleAccountI)
	s.Require().True(ok, "must be a module account")
	s.Require().Equal(name, macc.GetName())
	s.Require().Equal(balance.String(), s.app.BankKeeper.GetAllBalances(s.ctx, _keyRotationPoolAddr).String(), "balance must be kept")
	s.Require().NoError(s.app.BankKeeper.BurnCoins(s.ctx, name, balance), "the pool must accept module transfers")
}

func (s *UpgradesTestSuite) TestEnsureModuleAccount() {
	testCases := []struct {
		name        string
		malleate    func() sdk.Coins
		keepsAccNum bool
	}{
		{
			name: "no account yet",
			malleate: func() sdk.Coins {
				s.removeKeyRotationPool()
				return sdk.NewCoins()
			},
			keepsAccNum: false,
		},
		{
			name:        "plain account squatting the address",
			malleate:    s.squatKeyRotationPool,
			keepsAccNum: true,
		},
		{
			name: "module account already exists",
			malleate: func() sdk.Coins {
				return sdk.NewCoins()
			},
			keepsAccNum: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			balance := tc.malleate()
			var accNum uint64
			if tc.keepsAccNum {
				accNum = s.app.AccountKeeper.GetAccount(s.ctx, _keyRotationPoolAddr).GetAccountNumber()
			}

			sdk55.EnsureModuleAccount(s.ctx, s.app.AccountKeeper, stakingtypes.KeyRotationFeePoolName)

			s.requireKeyRotationPool(balance)
			if tc.keepsAccNum {
				s.Require().Equal(accNum, s.app.AccountKeeper.GetAccount(s.ctx, _keyRotationPoolAddr).GetAccountNumber(), "account number must be kept")
			}
		})
	}
}

func (s *UpgradesTestSuite) TestSDK55Upgrade() {
	sk := s.app.StakingKeeper

	vals, err := sk.GetAllValidators(s.ctx)
	s.Require().NoError(err)
	valAddr, err := sdk.ValAddressFromBech32(vals[0].GetOperator())
	s.Require().NoError(err)
	dels, err := sk.GetValidatorDelegations(s.ctx, valAddr)
	s.Require().NoError(err)
	delAddr := sdk.MustAccAddressFromBech32(dels[0].DelegatorAddress)

	completion, _, err := sk.Undelegate(s.ctx, delAddr, valAddr, sdkmath.LegacyNewDecWithPrec(5, 1))
	s.Require().NoError(err)

	// Mirror state written by a pre-index SDK binary: the queue entry without its index.
	store := s.ctx.KVStore(s.app.GetKey(stakingtypes.StoreKey))
	store.Delete(stakingtypes.UBDQueuePendingSlotsKey)
	store.Delete(stakingtypes.RedelegationQueuePendingSlotsKey)
	store.Delete(stakingtypes.ValidatorQueuePendingSlotsKey)

	mature := s.ctx.WithBlockTime(completion.Add(time.Second)).WithBlockHeight(6)
	cacheCtx, _ := mature.CacheContext()
	_, err = sk.EndBlocker(cacheCtx)
	s.Require().NoError(err)
	_, err = sk.GetUnbondingDelegation(cacheCtx, delAddr, valAddr)
	s.Require().NoError(err, "without the index the unbonding must stay pending")

	poolBalance := s.squatKeyRotationPool()

	// An IBC channel opened before ibc-go v11.2.1 keeps its send sequence under the v1 key.
	const port, channel, connection, clientID, nextSeq = "transfer", "channel-0", "connection-0", _tmClientID, uint64(5)
	s.app.IBCKeeper.ConnectionKeeper.SetConnection(s.ctx, connection, connectiontypes.NewConnectionEnd(
		connectiontypes.OPEN, clientID,
		connectiontypes.NewCounterparty("07-tendermint-3", "connection-3", commitmenttypes.NewMerklePrefix([]byte("ibc"))),
		[]*connectiontypes.Version{connectiontypes.DefaultIBCVersion}, 0,
	))
	s.app.IBCKeeper.ChannelKeeper.SetChannel(s.ctx, port, channel, channeltypes.NewChannel(
		channeltypes.OPEN, channeltypes.UNORDERED, channeltypes.NewCounterparty(port, "channel-7"), []string{connection}, "ics20-1",
	))
	s.ctx.KVStore(s.app.GetKey(ibcexported.StoreKey)).Set(channelv11.NextSequenceSendV1Key(port, channel), sdk.Uint64ToBigEndian(nextSeq))
	_, found := s.app.IBCKeeper.ChannelKeeper.GetNextSequenceSend(s.ctx, port, channel)
	s.Require().False(found, "ibc-go v11.2.1 no longer reads the v1 key")

	plan := upgradetypes.Plan{Name: "sdk55", Height: 5}
	s.Require().NoError(s.app.UpgradeKeeper.ScheduleUpgrade(s.ctx, plan))
	s.Require().NoError(s.app.UpgradeKeeper.ApplyUpgrade(s.ctx, plan))

	s.requireKeyRotationPool(poolBalance)

	seq, found := s.app.IBCKeeper.ChannelKeeper.GetNextSequenceSend(s.ctx, port, channel)
	s.Require().True(found, "the upgrade must move the send sequence, or transfers on the channel fail")
	s.Require().Equal(nextSeq, seq)
	s.Require().NoError(sdk55.MigrateChannelSendSequences(
		s.ctx, runtime.NewKVStoreService(s.app.GetKey(ibcexported.StoreKey)), s.app.AppCodec(), s.app.IBCKeeper,
	))
	seq, _ = s.app.IBCKeeper.ChannelKeeper.GetNextSequenceSend(s.ctx, port, channel)
	s.Require().Equal(nextSeq, seq, "a second run must keep the send sequence")
	aliasedClient, found := s.app.IBCKeeper.ChannelKeeperV2.GetClientForAlias(s.ctx, channel)
	s.Require().True(found, "an OPEN UNORDERED channel must get an IBC v2 alias")
	s.Require().Equal(clientID, aliasedClient)
	counterparty, found := s.app.IBCKeeper.ClientV2Keeper.GetClientCounterparty(s.ctx, channel)
	s.Require().True(found)
	s.Require().Equal("channel-7", counterparty.ClientId)

	_, err = sk.EndBlocker(mature)
	s.Require().NoError(err)
	_, err = sk.GetUnbondingDelegation(mature, delAddr, valAddr)
	s.Require().ErrorIs(err, stakingtypes.ErrNoUnbondingDelegation, "the upgrade must index the unbonding so it matures")
}
