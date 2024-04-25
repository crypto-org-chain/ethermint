package testutil

import (
	"encoding/binary"
	"encoding/json"
	"math/big"
	"time"

	coreheader "cosmossdk.io/core/header"
	storetypes "cosmossdk.io/store/types"
	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
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

func (suite *BaseTestSuite) MintFeeCollectorVirtual(coins sdk.Coins) {
	// add some virtual balance to the fee collector for refunding
	addVirtualCoins(
		suite.Ctx.ObjectStore(suite.App.GetStoreKey(banktypes.ObjectStoreKey)),
		suite.Ctx.TxIndex(),
		authtypes.NewModuleAddress(authtypes.FeeCollectorName),
		coins,
	)
}

func addVirtualCoins(store storetypes.ObjKVStore, txIndex int, addr sdk.AccAddress, amt sdk.Coins) {
	key := make([]byte, len(addr)+8)
	copy(key, addr)
	binary.BigEndian.PutUint64(key[len(addr):], uint64(txIndex))

	var coins sdk.Coins
	value := store.Get(key)
	if value != nil {
		coins = value.(sdk.Coins)
	}
	coins = coins.Add(amt...)
	store.Set(key, coins)
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
	suite.Ctx = suite.App.NewUncachedContext(checkTx, tmproto.Header{
		Height:  1,
		ChainID: app.ChainID,
		Time:    time.Now().UTC(),
	}).WithChainID(app.ChainID)
}

func (suite *BaseTestSuite) StateDB() *statedb.StateDB {
	return statedb.New(suite.Ctx, suite.App.EvmKeeper, statedb.NewEmptyTxConfig(common.BytesToHash(suite.Ctx.HeaderHash())))
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
	suite.SetupAccount(t)
	suite.BaseTestSuite.SetupTestWithCbAndOpts(t, patch, appOptions)
	suite.PostSetupValidator(t)
}

func (suite *BaseTestSuiteWithAccount) SetupAccount(t require.TestingT) {
	// account key, use a constant account to keep unit test deterministic.
	ecdsaPriv, err := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	require.NoError(t, err)
	suite.PrivKey = &ethsecp256k1.PrivKey{
		Key: crypto.FromECDSA(ecdsaPriv),
	}
	pubKey := suite.PrivKey.PubKey()
	suite.Address = common.BytesToAddress(pubKey.Address().Bytes())
	suite.Signer = tests.NewSigner(suite.PrivKey)
	// consensus key
	priv, err := ethsecp256k1.GenerateKey()
	suite.ConsPubKey = priv.PubKey()
	require.NoError(t, err)
	suite.ConsAddress = sdk.ConsAddress(suite.ConsPubKey.Address())
}

func (suite *BaseTestSuiteWithAccount) PostSetupValidator(t require.TestingT) stakingtypes.Validator {
	suite.Ctx = suite.Ctx.WithProposer(suite.ConsAddress)
	acc := &ethermint.EthAccount{
		BaseAccount: authtypes.NewBaseAccount(sdk.AccAddress(suite.Address.Bytes()), nil, 0, 0),
		CodeHash:    common.BytesToHash(crypto.Keccak256(nil)).String(),
	}
	acc.AccountNumber = suite.App.AccountKeeper.NextAccountNumber(suite.Ctx)
	suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)
	valAddr := sdk.ValAddress(suite.Address.Bytes())
	validator, err := stakingtypes.NewValidator(valAddr.String(), suite.ConsPubKey, stakingtypes.Description{})
	require.NoError(t, err)
	err = suite.App.StakingKeeper.SetValidatorByConsAddr(suite.Ctx, validator)
	require.NoError(t, err)
	err = suite.App.StakingKeeper.SetValidator(suite.Ctx, validator)
	require.NoError(t, err)
	return validator
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
) *types.MsgEthereumTx {
	chainID := suite.App.EvmKeeper.ChainID()
	adr := privKey.PubKey().Address()
	from := common.BytesToAddress(adr.Bytes())
	nonce := suite.getNonce(from.Bytes())
	data := make([]byte, 0)
	msgEthereumTx := types.NewTx(
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

func (suite *BaseTestSuiteWithAccount) PrepareEthTx(msgEthereumTx *types.MsgEthereumTx, privKey *ethsecp256k1.PrivKey) []byte {
	ethSigner := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
	err := msgEthereumTx.Sign(ethSigner, tests.NewSigner(privKey))
	suite.Require().NoError(err)

	txConfig := suite.App.TxConfig()
	evmDenom := suite.App.EvmKeeper.GetParams(suite.Ctx).EvmDenom

	tx, err := msgEthereumTx.BuildTx(txConfig.NewTxBuilder(), evmDenom)
	suite.Require().NoError(err)

	// bz are bytes to be broadcasted over the network
	bz, err := txConfig.TxEncoder()(tx)
	suite.Require().NoError(err)

	return bz
}

func (suite *BaseTestSuiteWithAccount) CheckTx(tx []byte) abci.ResponseCheckTx {
	res, err := suite.App.CheckTx(&abci.RequestCheckTx{Tx: tx})
	if err != nil {
		panic(err)
	}
	return *res
}

func (suite *BaseTestSuiteWithAccount) DeliverTx(tx []byte) *abci.ExecTxResult {
	txs := [][]byte{tx}
	height := suite.App.LastBlockHeight() + 1
	res, err := suite.App.FinalizeBlock(&abci.RequestFinalizeBlock{
		ProposerAddress: suite.ConsAddress,
		Height:          height,
		Txs:             txs,
	})
	if err != nil {
		panic(err)
	}
	results := res.GetTxResults()
	if len(results) != 1 {
		panic("must have one result")
	}
	return results[0]
}

// Commit and begin new block
func (suite *BaseTestSuiteWithAccount) Commit(t require.TestingT) {
	jumpTime := time.Second * 0
	_, err := suite.App.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: suite.Ctx.BlockHeight(),
		Time:   suite.Ctx.BlockTime(),
	})
	require.NoError(t, err)
	_, err = suite.App.Commit()
	require.NoError(t, err)
	newBlockTime := suite.Ctx.BlockTime().Add(jumpTime)
	header := suite.Ctx.BlockHeader()
	header.Time = newBlockTime
	header.Height++
	// update ctx
	suite.Ctx = suite.App.NewUncachedContext(false, header).WithHeaderInfo(coreheader.Info{
		Height: header.Height,
		Time:   header.Time,
	})
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
	suite.Setup(&suite.BaseTestSuite)
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
	suite.Setup(&suite.BaseTestSuite)
}

// DeployTestContract deploy a test erc20 contract and returns the contract address
func (suite *EVMTestSuiteWithAccountAndQueryClient) DeployTestContract(
	t require.TestingT,
	owner common.Address,
	supply *big.Int,
	enableFeemarket bool,
) common.Address {
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
	res, err := suite.EvmQueryClient.EstimateGas(suite.Ctx, &types.EthCallRequest{
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
	rsp, err := suite.App.EvmKeeper.EthereumTx(suite.Ctx, erc20DeployTx)
	require.NoError(t, err)
	require.Empty(t, rsp.VmError)
	return crypto.CreateAddress(suite.Address, nonce)
}

// Commit and begin new block
func (suite *EVMTestSuiteWithAccountAndQueryClient) Commit(t require.TestingT) {
	suite.BaseTestSuiteWithAccount.Commit(t)
	queryHelper := baseapp.NewQueryServerTestHelper(suite.Ctx, suite.App.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, suite.App.EvmKeeper)
	suite.EvmQueryClient = types.NewQueryClient(queryHelper)
}

func (suite *EVMTestSuiteWithAccountAndQueryClient) EvmDenom() string {
	rsp, _ := suite.EvmQueryClient.Params(suite.Ctx, &types.QueryParamsRequest{})
	return rsp.Params.EvmDenom
}
