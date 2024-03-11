package testutil

import (
	"encoding/json"
	"math/big"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/server/config"
	"github.com/evmos/ethermint/tests"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
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
	suite.SetupTestWithCb(suite.T(), nil)
}

func (suite *BaseTestSuite) SetupTestWithCb(
	t require.TestingT,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
) {
	suite.SetupTestWithCbAndOpts(t, patch, nil)
}

func (suite *BaseTestSuite) SetupTestWithCbAndOpts(
	_ require.TestingT,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
	appOptions simtestutil.AppOptionsMap,
) {
	checkTx := false
	suite.App = app.SetupWithOpts(checkTx, patch, appOptions)
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
	PrivKey     *ethsecp256k1.PrivKey
	Signer      keyring.Signer
	ConsAddress sdk.ConsAddress
	ConsPubKey  cryptotypes.PubKey
}

func (suite *BaseTestSuiteWithAccount) SetupTest(t require.TestingT) {
	suite.SetupTestWithCb(t, nil)
}

func (suite *BaseTestSuiteWithAccount) SetupTestWithCb(
	t require.TestingT,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
) {
	suite.SetupTestWithCbAndOpts(t, patch, nil)
}

func (suite *BaseTestSuiteWithAccount) SetupTestWithCbAndOpts(
	t require.TestingT,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
	appOptions simtestutil.AppOptionsMap,
) {
	suite.setupAccount(t)
	suite.BaseTestSuite.SetupTestWithCbAndOpts(t, patch, appOptions)
	suite.postSetupValidator(t)
}

func (suite *BaseTestSuiteWithAccount) setupAccount(t require.TestingT) {
	// account key, use a constant account to keep unit test deterministic.
	ecdsaPriv, err := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	require.NoError(t, err)
	priv := &ethsecp256k1.PrivKey{
		Key: crypto.FromECDSA(ecdsaPriv),
	}
	pubKey := priv.PubKey()
	suite.Address = common.BytesToAddress(pubKey.Address().Bytes())
	suite.Signer = tests.NewSigner(priv)
	// consensus key
	priv, err = ethsecp256k1.GenerateKey()
	suite.ConsPubKey = priv.PubKey()
	require.NoError(t, err)
	suite.ConsAddress = sdk.ConsAddress(suite.ConsPubKey.Address())
}

func (suite *BaseTestSuiteWithAccount) postSetupValidator(t require.TestingT) stakingtypes.Validator {
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

func (suite *BaseTestSuiteWithAccount) GenerateKey() (*ethsecp256k1.PrivKey, sdk.AccAddress) {
	address, priv := tests.NewAddrKey()
	return priv.(*ethsecp256k1.PrivKey), sdk.AccAddress(address.Bytes())
}

func (suite *BaseTestSuiteWithAccount) getNonce(addressBytes []byte) uint64 {
	return suite.App.EvmKeeper.GetNonce(
		suite.Ctx,
		common.BytesToAddress(addressBytes),
	)
}

func (suite *BaseTestSuiteWithAccount) BuildEthTx(
	to *common.Address,
	gasLimit uint64,
	gasPrice *big.Int,
	gasFeeCap *big.Int,
	gasTipCap *big.Int,
	accesses *ethtypes.AccessList,
	privKey *ethsecp256k1.PrivKey,
) *evmtypes.MsgEthereumTx {
	chainID := suite.App.EvmKeeper.ChainID()
	adr := privKey.PubKey().Address()
	from := common.BytesToAddress(adr.Bytes())
	nonce := suite.getNonce(from.Bytes())
	data := make([]byte, 0)
	msgEthereumTx := evmtypes.NewTx(
		chainID,
		nonce,
		to,
		nil,
		gasLimit,
		gasPrice,
		gasFeeCap,
		gasTipCap,
		data,
		accesses,
	)
	msgEthereumTx.From = from.Bytes()
	return msgEthereumTx
}

func (suite *BaseTestSuiteWithAccount) prepareEthTx(msgEthereumTx *evmtypes.MsgEthereumTx, privKey *ethsecp256k1.PrivKey) []byte {
	ethSigner := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
	encodingConfig := encoding.MakeConfig(app.ModuleBasics)
	option, err := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
	suite.Require().NoError(err)

	txBuilder := encodingConfig.TxConfig.NewTxBuilder()
	builder, ok := txBuilder.(authtx.ExtensionOptionsTxBuilder)
	suite.Require().True(ok)
	builder.SetExtensionOptions(option)

	err = msgEthereumTx.Sign(ethSigner, tests.NewSigner(privKey))
	suite.Require().NoError(err)

	err = txBuilder.SetMsgs(msgEthereumTx)
	suite.Require().NoError(err)

	txData, err := evmtypes.UnpackTxData(msgEthereumTx.Data)
	suite.Require().NoError(err)

	evmDenom := suite.App.EvmKeeper.GetParams(suite.Ctx).EvmDenom
	fees := sdk.Coins{{Denom: evmDenom, Amount: sdk.NewIntFromBigInt(txData.Fee())}}
	builder.SetFeeAmount(fees)
	builder.SetGasLimit(msgEthereumTx.GetGas())

	// bz are bytes to be broadcasted over the network
	bz, err := encodingConfig.TxConfig.TxEncoder()(txBuilder.GetTx())
	suite.Require().NoError(err)

	return bz
}

func (suite *BaseTestSuiteWithAccount) CheckEthTx(
	msgEthereumTx *evmtypes.MsgEthereumTx,
	privKey *ethsecp256k1.PrivKey,
) abci.ResponseCheckTx {
	bz := suite.prepareEthTx(msgEthereumTx, privKey)
	req := abci.RequestCheckTx{Tx: bz}
	res := suite.App.BaseApp.CheckTx(req)
	return res
}

func (suite *BaseTestSuiteWithAccount) DeliverEthTx(
	msgEthereumTx *evmtypes.MsgEthereumTx,
	privKey *ethsecp256k1.PrivKey,
) abci.ResponseDeliverTx {
	bz := suite.prepareEthTx(msgEthereumTx, privKey)
	req := abci.RequestDeliverTx{Tx: bz}
	res := suite.App.BaseApp.DeliverTx(req)
	return res
}

// Commit and begin new block
func (suite *BaseTestSuiteWithAccount) Commit() {
	_ = suite.App.Commit()
	header := suite.Ctx.BlockHeader()
	header.Height++
	suite.App.BeginBlock(abci.RequestBeginBlock{
		Header: header,
	})
	// update ctx
	suite.Ctx = suite.App.BaseApp.NewContext(false, header)
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
	suite.SetupTestWithCb(suite.T(), nil)
}

func (suite *BaseTestSuiteWithFeeMarketQueryClient) SetupTestWithCb(
	t require.TestingT,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
) {
	suite.BaseTestSuite.SetupTestWithCb(t, patch)
	suite.feemarketQueryClientTrait.Setup(&suite.BaseTestSuite)
}

type EVMTestSuiteWithAccountAndQueryClient struct {
	BaseTestSuiteWithAccount
	evmQueryClientTrait
}

func (suite *EVMTestSuiteWithAccountAndQueryClient) SetupTest(t require.TestingT) {
	suite.SetupTestWithCb(t, nil)
}

func (suite *EVMTestSuiteWithAccountAndQueryClient) SetupTestWithCb(
	t require.TestingT,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
) {
	suite.BaseTestSuiteWithAccount.SetupTestWithCb(t, patch)
	suite.evmQueryClientTrait.Setup(&suite.BaseTestSuite)
}

// DeployTestContract deploy a test erc20 contract and returns the contract address
func (suite *EVMTestSuiteWithAccountAndQueryClient) DeployTestContract(
	t require.TestingT,
	owner common.Address,
	supply *big.Int,
	enableFeemarket bool,
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

// Commit and begin new block
func (suite *EVMTestSuiteWithAccountAndQueryClient) Commit() {
	suite.BaseTestSuiteWithAccount.Commit()
	queryHelper := baseapp.NewQueryServerTestHelper(suite.Ctx, suite.App.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, suite.App.EvmKeeper)
	suite.EvmQueryClient = types.NewQueryClient(queryHelper)
}

func (suite *EVMTestSuiteWithAccountAndQueryClient) EvmDenom() string {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	rsp, _ := suite.EvmQueryClient.Params(ctx, &types.QueryParamsRequest{})
	return rsp.Params.EvmDenom
}

type FeeMarketTestSuiteWithAccountAndQueryClient struct {
	BaseTestSuiteWithAccount
	feemarketQueryClientTrait
}

func (suite *FeeMarketTestSuiteWithAccountAndQueryClient) SetupTest(t require.TestingT) {
	suite.SetupTestWithCb(t, nil)
}

func (suite *FeeMarketTestSuiteWithAccountAndQueryClient) SetupTestWithCb(
	t require.TestingT,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
) {
	suite.setupAccount(t)
	suite.BaseTestSuite.SetupTestWithCb(t, patch)
	validator := suite.postSetupValidator(t)
	validator = stakingkeeper.TestingUpdateValidator(suite.App.StakingKeeper, suite.Ctx, validator, true)
	err := suite.App.StakingKeeper.Hooks().AfterValidatorCreated(suite.Ctx, validator.GetOperator())
	require.NoError(t, err)
	err = suite.App.StakingKeeper.SetValidatorByConsAddr(suite.Ctx, validator)
	require.NoError(t, err)
	suite.App.StakingKeeper.SetValidator(suite.Ctx, validator)
	suite.feemarketQueryClientTrait.Setup(&suite.BaseTestSuite)
}
