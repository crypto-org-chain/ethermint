package testutil

import (
	"encoding/json"
	"math/big"
	"time"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/server/config"
	"github.com/evmos/ethermint/tests"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BaseTestSuite struct {
	suite.Suite

	Ctx sdk.Context
	App *app.EthermintApp
}

func (suite *BaseTestSuite) SetupTest() {
	suite.SetupTestWithCb(nil)
}

func (suite *BaseTestSuite) SetupTestWithCb(patch func(*app.EthermintApp, app.GenesisState) app.GenesisState) {
	checkTx := false
	suite.App = app.Setup(checkTx, patch)
	suite.Ctx = suite.App.BaseApp.NewContext(checkTx, tmproto.Header{
		Height:  1,
		ChainID: app.ChainID,
		Time:    time.Now().UTC(),
	})
}

func (suite *BaseTestSuite) StateDB() *statedb.StateDB {
	return statedb.New(suite.Ctx, suite.App.EvmKeeper, statedb.NewEmptyTxConfig(common.BytesToHash(suite.Ctx.HeaderHash().Bytes())))
}

type BaseTestSuiteWithAccount struct {
	BaseTestSuite
	Address     common.Address
	PubKey      cryptotypes.PubKey
	Signer      keyring.Signer
	ConsAddress sdk.ConsAddress
	ConsPubKey  cryptotypes.PubKey
}

func (suite *BaseTestSuiteWithAccount) SetupTest() {
	suite.SetupAccount()
	suite.BaseTestSuite.SetupTest()
	suite.PostSetupValidator()
}

func (suite *BaseTestSuiteWithAccount) SetupTestWithCb(patch func(*app.EthermintApp, app.GenesisState) app.GenesisState) {
	suite.SetupAccount()
	suite.BaseTestSuite.SetupTestWithCb(patch)
	suite.PostSetupValidator()
}

func (suite *BaseTestSuiteWithAccount) SetupAccount() {
	t := suite.T()
	// account key, use a constant account to keep unit test deterministic.
	ecdsaPriv, err := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	require.NoError(t, err)
	priv := &ethsecp256k1.PrivKey{
		Key: crypto.FromECDSA(ecdsaPriv),
	}
	suite.PubKey = priv.PubKey()
	suite.Address = common.BytesToAddress(suite.PubKey.Address().Bytes())
	suite.Signer = tests.NewSigner(priv)
	// consensus key
	priv, err = ethsecp256k1.GenerateKey()
	suite.ConsPubKey = priv.PubKey()
	require.NoError(t, err)
	suite.ConsAddress = sdk.ConsAddress(suite.ConsPubKey.Address())
}

func (suite *BaseTestSuiteWithAccount) PostSetupValidator() stakingtypes.Validator {
	t := suite.T()
	suite.Ctx = suite.Ctx.WithProposer(suite.ConsAddress)
	acc := &ethermint.EthAccount{
		BaseAccount: authtypes.NewBaseAccount(sdk.AccAddress(suite.Address.Bytes()), nil, 0, 0),
		CodeHash:    common.BytesToHash(crypto.Keccak256(nil)).String(),
	}
	suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)
	valAddr := sdk.ValAddress(suite.Address.Bytes())
	validator, err := stakingtypes.NewValidator(valAddr, suite.ConsPubKey, stakingtypes.Description{})
	require.NoError(t, err)
	err = suite.App.StakingKeeper.SetValidatorByConsAddr(suite.Ctx, validator)
	require.NoError(t, err)
	suite.App.StakingKeeper.SetValidator(suite.Ctx, validator)
	return validator
}

type evmQueryClientTrait struct {
	EvmQueryClient types.QueryClient
}

func (trait *evmQueryClientTrait) Setup(suite *BaseTestSuite) {
	queryHelper := baseapp.NewQueryServerTestHelper(suite.Ctx, suite.App.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, suite.App.EvmKeeper)
	trait.EvmQueryClient = types.NewQueryClient(queryHelper)
}

type feemarketQueryClientTrait struct {
	FeeMarketQueryClient feemarkettypes.QueryClient
}

func (trait *feemarketQueryClientTrait) Setup(suite *BaseTestSuite) {
	queryHelper := baseapp.NewQueryServerTestHelper(suite.Ctx, suite.App.InterfaceRegistry())
	feemarkettypes.RegisterQueryServer(queryHelper, suite.App.FeeMarketKeeper)
	trait.FeeMarketQueryClient = feemarkettypes.NewQueryClient(queryHelper)
}

type BaseTestSuiteWithFeeMarketQueryClient struct {
	BaseTestSuite
	feemarketQueryClientTrait
}

func (suite *BaseTestSuiteWithFeeMarketQueryClient) SetupTest() {
	suite.BaseTestSuite.SetupTest()
	suite.feemarketQueryClientTrait.Setup(&suite.BaseTestSuite)
}

type EVMTestSuiteWithAccountAndQueryClient struct {
	BaseTestSuiteWithAccount
	evmQueryClientTrait
}

func (suite *EVMTestSuiteWithAccountAndQueryClient) SetupTestWithCb(patch func(*app.EthermintApp, app.GenesisState) app.GenesisState) {
	suite.BaseTestSuiteWithAccount.SetupTestWithCb(patch)
	suite.evmQueryClientTrait.Setup(&suite.BaseTestSuite)
}

func (suite *EVMTestSuiteWithAccountAndQueryClient) SetupQueryClient() {
	suite.evmQueryClientTrait.Setup(&suite.BaseTestSuite)
}

// DeployTestContractWithT deploy a test erc20 contract and returns the contract address
func (suite *EVMTestSuiteWithAccountAndQueryClient) DeployTestContractWithT(
	owner common.Address,
	supply *big.Int,
	enableFeemarket bool,
	t require.TestingT,
) common.Address {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	chainID := suite.App.EvmKeeper.ChainID()
	ctorArgs, err := types.ERC20Contract.ABI.Pack("", owner, supply)
	require.NoError(t, err)
	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
	data := append(types.ERC20Contract.Bin, ctorArgs...) //nolint: gocritic
	args, err := json.Marshal(&types.TransactionArgs{
		From: &suite.Address,
		Data: (*hexutil.Bytes)(&data),
	})
	require.NoError(t, err)
	res, err := suite.EvmQueryClient.EstimateGas(ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          config.DefaultGasCap,
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	require.NoError(t, err)

	var erc20DeployTx *types.MsgEthereumTx
	if enableFeemarket {
		erc20DeployTx = types.NewTxContract(
			chainID,
			nonce,
			nil,     // amount
			res.Gas, // gasLimit
			nil,     // gasPrice
			suite.App.FeeMarketKeeper.GetBaseFee(suite.Ctx),
			big.NewInt(1),
			data,                   // input
			&ethtypes.AccessList{}, // accesses
		)
	} else {
		erc20DeployTx = types.NewTxContract(
			chainID,
			nonce,
			nil,     // amount
			res.Gas, // gasLimit
			nil,     // gasPrice
			nil, nil,
			data, // input
			nil,  // accesses
		)
	}

	erc20DeployTx.From = suite.Address.Bytes()
	err = erc20DeployTx.Sign(ethtypes.LatestSignerForChainID(chainID), suite.Signer)
	require.NoError(t, err)
	rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, erc20DeployTx)
	require.NoError(t, err)
	require.Empty(t, rsp.VmError)
	return crypto.CreateAddress(suite.Address, nonce)
}

func (suite *EVMTestSuiteWithAccountAndQueryClient) DeployTestContract(
	owner common.Address,
	supply *big.Int,
	enableFeemarket bool,
) common.Address {
	return suite.DeployTestContractWithT(owner, supply, enableFeemarket, suite.T())
}

type FeeMarketTestSuiteWithAccountAndQueryClient struct {
	BaseTestSuiteWithAccount
	feemarketQueryClientTrait
}

func (suite *FeeMarketTestSuiteWithAccountAndQueryClient) SetupTest() {
	suite.SetupAccount()
	suite.BaseTestSuite.SetupTestWithCb(nil)
	validator := suite.PostSetupValidator()
	validator = stakingkeeper.TestingUpdateValidator(suite.App.StakingKeeper, suite.Ctx, validator, true)
	err := suite.App.StakingKeeper.Hooks().AfterValidatorCreated(suite.Ctx, validator.GetOperator())
	t := suite.T()
	require.NoError(t, err)
	err = suite.App.StakingKeeper.SetValidatorByConsAddr(suite.Ctx, validator)
	require.NoError(t, err)
	suite.App.StakingKeeper.SetValidator(suite.Ctx, validator)
	suite.feemarketQueryClientTrait.Setup(&suite.BaseTestSuite)
}
