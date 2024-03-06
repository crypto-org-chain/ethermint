package testutil

import (
	"time"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type EVMTestSuite struct {
	suite.Suite

	Ctx sdk.Context
	App *app.EthermintApp
}

func (suite *EVMTestSuite) SetupTest() {
	suite.SetupTestWithCb(nil)
}

func (suite *EVMTestSuite) SetupTestWithCb(patch func(*app.EthermintApp, app.GenesisState) app.GenesisState) {
	checkTx := false
	suite.App = app.Setup(checkTx, patch)
	suite.Ctx = suite.App.BaseApp.NewContext(checkTx, tmproto.Header{
		Height:  1,
		ChainID: app.ChainID,
		Time:    time.Now().UTC(),
	})
}
func (suite *EVMTestSuite) StateDB() *statedb.StateDB {
	return statedb.New(suite.Ctx, suite.App.EvmKeeper, statedb.NewEmptyTxConfig(common.BytesToHash(suite.Ctx.HeaderHash().Bytes())))
}

type EVMTestSuiteWithAccount struct {
	EVMTestSuite
	Address common.Address
	Priv    *ethsecp256k1.PrivKey
}

func (suite *EVMTestSuiteWithAccount) SetupTest() {
	suite.EVMTestSuite.SetupTest()
	suite.SetupAccount()
}

func (suite *EVMTestSuiteWithAccount) SetupTestWithCb(patch func(*app.EthermintApp, app.GenesisState) app.GenesisState) {
	suite.EVMTestSuite.SetupTestWithCb(patch)
	suite.SetupAccount()
}

func (suite *EVMTestSuiteWithAccount) SetupAccount() {
	// account key, use a constant account to keep unit test deterministic.
	ecdsaPriv, err := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	require.NoError(suite.T(), err)
	suite.Priv = &ethsecp256k1.PrivKey{
		Key: crypto.FromECDSA(ecdsaPriv),
	}
	suite.Address = common.BytesToAddress(suite.Priv.PubKey().Address().Bytes())
}
