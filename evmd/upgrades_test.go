package evmd_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
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

func (s *UpgradesTestSuite) applySDK55Upgrade() {
	plan := upgradetypes.Plan{Name: "sdk55", Height: 5}
	s.Require().NoError(s.app.UpgradeKeeper.ScheduleUpgrade(s.ctx, plan))
	s.Require().NoError(s.app.UpgradeKeeper.ApplyUpgrade(s.ctx, plan))
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

	s.applySDK55Upgrade()

	s.requireKeyRotationPool(poolBalance)

	_, err = sk.EndBlocker(mature)
	s.Require().NoError(err)
	_, err = sk.GetUnbondingDelegation(mature, delAddr, valAddr)
	s.Require().ErrorIs(err, stakingtypes.ErrNoUnbondingDelegation, "the upgrade must index the unbonding so it matures")
}

func (s *UpgradesTestSuite) TestSDK55UpgradeMigratesSendSequences() {
	const (
		portID       = "transfer"
		channelID    = "channel-0"
		connectionID = "connection-0"
		sequence     = uint64(7)
	)

	testCases := []struct {
		name          string
		ordering      channeltypes.Order
		hasConnection bool
		aliased       bool
	}{
		{
			name:     "unordered channel also gets its IBC v2 alias",
			ordering: channeltypes.UNORDERED, hasConnection: true, aliased: true,
		},
		{
			name:     "ordered channel only moves its sequence",
			ordering: channeltypes.ORDERED, hasConnection: true, aliased: false,
		},
		{
			name:     "unordered channel without a connection only moves its sequence",
			ordering: channeltypes.UNORDERED, hasConnection: false, aliased: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ibcKeeper := s.app.IBCKeeper

			if tc.hasConnection {
				ibcKeeper.ConnectionKeeper.SetConnection(s.ctx, connectionID, connectiontypes.NewConnectionEnd(
					connectiontypes.OPEN, _tmClientID,
					connectiontypes.NewCounterparty(_tmClientID, connectionID, commitmenttypes.NewMerklePrefix([]byte("ibc"))),
					connectiontypes.GetCompatibleVersions(), 0,
				))
			}
			ibcKeeper.ChannelKeeper.SetChannel(s.ctx, portID, channelID, channeltypes.NewChannel(
				channeltypes.OPEN, tc.ordering, channeltypes.NewCounterparty(portID, "channel-1"), []string{connectionID}, "ics20-1",
			))
			// ibc-go v11.2.0 and older kept the send sequence under the port and channel.
			store := s.ctx.KVStore(s.app.GetKey(ibcexported.StoreKey))
			oldKey := channelv11.NextSequenceSendV1Key(portID, channelID)
			store.Set(oldKey, sdk.Uint64ToBigEndian(sequence))
			_, found := ibcKeeper.ChannelKeeper.GetNextSequenceSend(s.ctx, portID, channelID)
			s.Require().False(found, "the sequence must be unreadable before the upgrade")

			s.applySDK55Upgrade()

			got, found := ibcKeeper.ChannelKeeper.GetNextSequenceSend(s.ctx, portID, channelID)
			s.Require().True(found)
			s.Require().Equal(sequence, got)
			s.Require().False(store.Has(oldKey), "the old key must be removed")

			clientID, aliased := ibcKeeper.ChannelKeeperV2.GetClientForAlias(s.ctx, channelID)
			s.Require().Equal(tc.aliased, aliased)
			if tc.aliased {
				s.Require().Equal(_tmClientID, clientID)
			}
			_, hasCounterparty := ibcKeeper.ClientV2Keeper.GetClientCounterparty(s.ctx, channelID)
			s.Require().Equal(tc.aliased, hasCounterparty)
		})
	}
}

func (s *UpgradesTestSuite) TestSDK55UpgradeMigratesFeeAllowanceQueue() {
	testCases := []struct {
		name  string
		value []byte
	}{
		// Written by the v1 -> v2 migration and the standalone feegrant module.
		{"empty value", []byte{}},
		{"encoded true", []byte{1}},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			queue := s.app.FeeGrantKeeper.FeeAllowanceQueue
			entry := collections.Join3(s.ctx.BlockTime(), sdk.AccAddress("grantee"), sdk.AccAddress("granter"))
			key, err := collections.EncodeKeyWithPrefix(feegrant.FeeAllowanceQueueKeyPrefix, queue.KeyCodec(), entry)
			s.Require().NoError(err)
			s.ctx.KVStore(s.app.GetKey(feegrant.StoreKey)).Set(key, tc.value)

			vm, err := s.app.UpgradeKeeper.GetModuleVersionMap(s.ctx)
			s.Require().NoError(err)
			vm[feegrant.ModuleName] = 2
			s.Require().NoError(s.app.UpgradeKeeper.SetModuleVersionMap(s.ctx, vm))

			s.applySDK55Upgrade()

			found, err := queue.Get(s.ctx, entry)
			s.Require().NoError(err)
			s.Require().True(found)
		})
	}
}
