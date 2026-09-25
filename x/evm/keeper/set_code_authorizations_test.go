package keeper_test

import (
	"errors"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
)

var delegationTarget = common.HexToAddress("0x000000000000000000000000000000000000abcd")

func (suite *StateDBTestSuite) newAuthorityKey() (common.Address, *ethtypes.SetCodeAuthorization) {
	suite.T().Helper()
	key, err := crypto.GenerateKey()
	suite.Require().NoError(err)
	addr := crypto.PubkeyToAddress(key.PublicKey)

	auth, err := ethtypes.SignSetCode(key, ethtypes.SetCodeAuthorization{
		Address: delegationTarget,
		Nonce:   0,
	})
	suite.Require().NoError(err)
	return addr, &auth
}

func (suite *StateDBTestSuite) setBaseAccount(addr common.Address) {
	suite.T().Helper()
	acc := authtypes.NewBaseAccount(sdk.AccAddress(addr.Bytes()), nil, 0, 0)
	acc.AccountNumber = suite.App.AccountKeeper.NextAccountNumber(suite.Ctx, acc)
	suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)
}

func (suite *StateDBTestSuite) setVestingAccount(addr common.Address) {
	suite.T().Helper()
	base := authtypes.NewBaseAccount(sdk.AccAddress(addr.Bytes()), nil, 0, 0)
	vacc, err := vestingtypes.NewContinuousVestingAccount(
		base,
		sdk.NewCoins(sdk.NewInt64Coin(types.DefaultEVMDenom, 1_000_000)),
		time.Now().Unix(),
		time.Now().Add(365*24*time.Hour).Unix(),
	)
	suite.Require().NoError(err)
	vacc.AccountNumber = suite.App.AccountKeeper.NextAccountNumber(suite.Ctx, vacc)
	suite.App.AccountKeeper.SetAccount(suite.Ctx, vacc)
}

func (suite *StateDBTestSuite) account(addr common.Address) sdk.AccountI {
	return suite.App.AccountKeeper.GetAccount(suite.Ctx, sdk.AccAddress(addr.Bytes()))
}

func (suite *StateDBTestSuite) TestSetCodeAuthorizationBaseAccountAuthority() {
	addr, auth := suite.newAuthorityKey()
	suite.setBaseAccount(addr)

	db := suite.StateDB()
	authority, err := suite.App.EvmKeeper.ApplyAuthorizationForTest(suite.Ctx, auth, db)
	suite.Require().NoError(err)
	suite.Require().Equal(addr, authority)
	suite.Require().NoError(db.Commit())

	suite.Require().Equal(
		ethtypes.AddressToDelegation(delegationTarget),
		suite.StateDB().GetCode(addr),
	)
	suite.Require().IsType(&ethermint.EthAccount{}, suite.account(addr))
	suite.Require().Equal(uint64(1), suite.account(addr).GetSequence())
}

func (suite *StateDBTestSuite) TestSetCodeAuthorizationVestingAuthorityRejected() {
	addr, auth := suite.newAuthorityKey()
	suite.setVestingAccount(addr)

	db := suite.StateDB()
	_, err := suite.App.EvmKeeper.ApplyAuthorizationForTest(suite.Ctx, auth, db)
	suite.Require().ErrorIs(err, keeper.ErrAuthorityCannotStoreCodeHash)
	suite.Require().NoError(db.Commit())

	suite.Require().Empty(suite.StateDB().GetCode(addr))
	suite.Require().IsType(&vestingtypes.ContinuousVestingAccount{}, suite.account(addr))
	suite.Require().Equal(uint64(0), suite.account(addr).GetSequence())
}

func (suite *StateDBTestSuite) TestSetAccountConvertsBaseAccountOnlyWithCode() {
	code := ethtypes.AddressToDelegation(delegationTarget)
	codeHash := crypto.Keccak256Hash(code)

	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	suite.setBaseAccount(addr)
	before := suite.account(addr).GetAccountNumber()

	suite.Require().NoError(suite.App.EvmKeeper.SetAccount(suite.Ctx, addr, statedb.Account{
		Nonce:    3,
		CodeHash: types.EmptyCodeHash,
	}))
	suite.Require().IsType(&authtypes.BaseAccount{}, suite.account(addr))
	suite.Require().Equal(uint64(3), suite.account(addr).GetSequence())

	suite.Require().NoError(suite.App.EvmKeeper.SetAccount(suite.Ctx, addr, statedb.Account{
		Nonce:    4,
		CodeHash: codeHash.Bytes(),
	}))
	converted := suite.account(addr)
	suite.Require().IsType(&ethermint.EthAccount{}, converted)
	suite.Require().Equal(uint64(4), converted.GetSequence())
	suite.Require().Equal(before, converted.GetAccountNumber())
	suite.Require().Equal(codeHash, converted.(ethermint.EthAccountI).GetCodeHash())
}

func (suite *StateDBTestSuite) TestSetAccountRejectsCodeHashOnVestingAccount() {
	code := ethtypes.AddressToDelegation(delegationTarget)
	codeHash := crypto.Keccak256Hash(code)

	addr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	suite.setVestingAccount(addr)

	suite.Require().NoError(suite.App.EvmKeeper.SetAccount(suite.Ctx, addr, statedb.Account{
		Nonce:    1,
		CodeHash: types.EmptyCodeHash,
	}))
	suite.Require().IsType(&vestingtypes.ContinuousVestingAccount{}, suite.account(addr))

	err := suite.App.EvmKeeper.SetAccount(suite.Ctx, addr, statedb.Account{
		Nonce:    2,
		CodeHash: codeHash.Bytes(),
	})
	suite.Require().Error(err)
	suite.Require().True(errors.Is(err, types.ErrInvalidAccount))
}

func (suite *StateDBTestSuite) TestContractDeploymentOntoVestingAccountFails() {
	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")
	suite.setVestingAccount(addr)

	db := suite.StateDB()
	db.SetCode(addr, []byte{0x60, 0x00, 0x60, 0x00, 0xfd}, 0)
	db.SetNonce(addr, 1, 0)

	suite.Require().Error(db.Commit())
	suite.Require().Empty(suite.StateDB().GetCode(addr))
}
