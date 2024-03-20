package keeper_test

import (
	"fmt"
	"math/big"
	"testing"

	"cosmossdk.io/store/prefix"
	"github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/suite"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/tests"
	"github.com/evmos/ethermint/testutil"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
)

type StateDBTestSuite struct {
	testutil.EVMTestSuiteWithAccountAndQueryClient
	// for generate test tx
	clientCtx client.Context
	ethSigner ethtypes.Signer
}

func TestStateDBTestSuite(t *testing.T) {
	suite.Run(t, new(StateDBTestSuite))
}

func (suite *StateDBTestSuite) SetupTest() {
	suite.EVMTestSuiteWithAccountAndQueryClient.SetupTest(suite.T())
	encodingConfig := encoding.MakeConfig()
	suite.clientCtx = client.Context{}.WithTxConfig(encodingConfig.TxConfig)
	suite.ethSigner = ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
}

func (suite *StateDBTestSuite) TestCreateAccount() {
	testCases := []struct {
		name     string
		addr     common.Address
		malleate func(vm.StateDB, common.Address)
		callback func(vm.StateDB, common.Address)
	}{
		{
			"reset account (keep balance)",
			suite.Address,
			func(vmdb vm.StateDB, addr common.Address) {
				vmdb.AddBalance(addr, big.NewInt(100))
				suite.Require().NotZero(vmdb.GetBalance(addr).Int64())
			},
			func(vmdb vm.StateDB, addr common.Address) {
				suite.Require().Equal(vmdb.GetBalance(addr).Int64(), int64(100))
			},
		},
		{
			"create account",
			tests.GenerateAddress(),
			func(vmdb vm.StateDB, addr common.Address) {
				suite.Require().False(vmdb.Exist(addr))
			},
			func(vmdb vm.StateDB, addr common.Address) {
				suite.Require().True(vmdb.Exist(addr))
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb, tc.addr)
			vmdb.CreateAccount(tc.addr)
			tc.callback(vmdb, tc.addr)
		})
	}
}

func (suite *StateDBTestSuite) TestAddBalance() {
	testCases := []struct {
		name   string
		amount *big.Int
		isNoOp bool
	}{
		{
			"positive amount",
			big.NewInt(100),
			false,
		},
		{
			"zero amount",
			big.NewInt(0),
			true,
		},
		{
			"negative amount",
			big.NewInt(-1),
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			prev := vmdb.GetBalance(suite.Address)
			vmdb.AddBalance(suite.Address, tc.amount)
			post := vmdb.GetBalance(suite.Address)

			if tc.isNoOp {
				suite.Require().Equal(prev.Int64(), post.Int64())
			} else {
				suite.Require().Equal(new(big.Int).Add(prev, tc.amount).Int64(), post.Int64())
			}
		})
	}
}

func (suite *StateDBTestSuite) TestSubBalance() {
	testCases := []struct {
		name     string
		amount   *big.Int
		malleate func(vm.StateDB)
		isNoOp   bool
	}{
		{
			"positive amount, below zero",
			big.NewInt(100),
			func(vm.StateDB) {},
			true,
		},
		{
			"positive amount, above zero",
			big.NewInt(50),
			func(vmdb vm.StateDB) {
				vmdb.AddBalance(suite.Address, big.NewInt(100))
			},
			false,
		},
		{
			"zero amount",
			big.NewInt(0),
			func(vm.StateDB) {},
			true,
		},
		{
			"negative amount",
			big.NewInt(-1),
			func(vm.StateDB) {},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			prev := vmdb.GetBalance(suite.Address)
			vmdb.SubBalance(suite.Address, tc.amount)
			post := vmdb.GetBalance(suite.Address)

			if tc.isNoOp {
				suite.Require().Equal(prev.Int64(), post.Int64())
			} else {
				suite.Require().Equal(new(big.Int).Sub(prev, tc.amount).Int64(), post.Int64())
			}
		})
	}
}

func (suite *StateDBTestSuite) TestGetNonce() {
	testCases := []struct {
		name          string
		address       common.Address
		expectedNonce uint64
		malleate      func(vm.StateDB)
	}{
		{
			"account not found",
			tests.GenerateAddress(),
			0,
			func(vm.StateDB) {},
		},
		{
			"existing account",
			suite.Address,
			1,
			func(vmdb vm.StateDB) {
				vmdb.SetNonce(suite.Address, 1)
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			nonce := vmdb.GetNonce(tc.address)
			suite.Require().Equal(tc.expectedNonce, nonce)
		})
	}
}

func (suite *StateDBTestSuite) TestSetNonce() {
	testCases := []struct {
		name     string
		address  common.Address
		nonce    uint64
		malleate func()
	}{
		{
			"new account",
			tests.GenerateAddress(),
			10,
			func() {},
		},
		{
			"existing account",
			suite.Address,
			99,
			func() {},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			vmdb.SetNonce(tc.address, tc.nonce)
			nonce := vmdb.GetNonce(tc.address)
			suite.Require().Equal(tc.nonce, nonce)
		})
	}
}

func (suite *StateDBTestSuite) TestGetCodeHash() {
	addr := tests.GenerateAddress()
	baseAcc := &authtypes.BaseAccount{Address: sdk.AccAddress(addr.Bytes()).String()}
	baseAcc.AccountNumber = suite.App.AccountKeeper.NextAccountNumber(suite.Ctx)
	suite.App.AccountKeeper.SetAccount(suite.Ctx, baseAcc)

	testCases := []struct {
		name     string
		address  common.Address
		expHash  common.Hash
		malleate func(vm.StateDB)
	}{
		{
			"account not found",
			tests.GenerateAddress(),
			common.Hash{},
			func(vm.StateDB) {},
		},
		{
			"account not EthAccount type, EmptyCodeHash",
			addr,
			common.BytesToHash(types.EmptyCodeHash),
			func(vm.StateDB) {},
		},
		{
			"existing account",
			suite.Address,
			crypto.Keccak256Hash([]byte("codeHash")),
			func(vmdb vm.StateDB) {
				vmdb.SetCode(suite.Address, []byte("codeHash"))
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			hash := vmdb.GetCodeHash(tc.address)
			suite.Require().Equal(tc.expHash, hash)
		})
	}
}

func (suite *StateDBTestSuite) TestSetCode() {
	addr := tests.GenerateAddress()
	baseAcc := &authtypes.BaseAccount{Address: sdk.AccAddress(addr.Bytes()).String()}
	baseAcc.AccountNumber = suite.App.AccountKeeper.NextAccountNumber(suite.Ctx)
	suite.App.AccountKeeper.SetAccount(suite.Ctx, baseAcc)

	testCases := []struct {
		name    string
		address common.Address
		code    []byte
		isNoOp  bool
	}{
		{
			"account not found",
			tests.GenerateAddress(),
			[]byte("code"),
			false,
		},
		{
			"account not EthAccount type",
			addr,
			nil,
			true,
		},
		{
			"existing account",
			suite.Address,
			[]byte("code"),
			false,
		},
		{
			"existing account, code deleted from store",
			suite.Address,
			nil,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			prev := vmdb.GetCode(tc.address)
			vmdb.SetCode(tc.address, tc.code)
			post := vmdb.GetCode(tc.address)

			if tc.isNoOp {
				suite.Require().Equal(prev, post)
			} else {
				suite.Require().Equal(tc.code, post)
			}

			suite.Require().Equal(len(post), vmdb.GetCodeSize(tc.address))
		})
	}
}

func (suite *StateDBTestSuite) TestKeeperSetCode() {
	addr := tests.GenerateAddress()
	baseAcc := &authtypes.BaseAccount{Address: sdk.AccAddress(addr.Bytes()).String()}
	baseAcc.AccountNumber = suite.App.AccountKeeper.NextAccountNumber(suite.Ctx)
	suite.App.AccountKeeper.SetAccount(suite.Ctx, baseAcc)

	testCases := []struct {
		name     string
		codeHash []byte
		code     []byte
	}{
		{
			"set code",
			[]byte("codeHash"),
			[]byte("this is the code"),
		},
		{
			"delete code",
			[]byte("codeHash"),
			nil,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.App.EvmKeeper.SetCode(suite.Ctx, tc.codeHash, tc.code)
			key := suite.App.GetKey(types.StoreKey)
			store := prefix.NewStore(suite.Ctx.KVStore(key), types.KeyPrefixCode)
			code := store.Get(tc.codeHash)

			suite.Require().Equal(tc.code, code)
		})
	}
}

func (suite *StateDBTestSuite) TestRefund() {
	testCases := []struct {
		name      string
		malleate  func(vm.StateDB)
		expRefund uint64
		expPanic  bool
	}{
		{
			"success - add and subtract refund",
			func(vmdb vm.StateDB) {
				vmdb.AddRefund(11)
			},
			1,
			false,
		},
		{
			"fail - subtract amount > current refund",
			func(vm.StateDB) {
			},
			0,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			if tc.expPanic {
				suite.Require().Panics(func() { vmdb.SubRefund(10) })
			} else {
				vmdb.SubRefund(10)
				suite.Require().Equal(tc.expRefund, vmdb.GetRefund())
			}
		})
	}
}

func (suite *StateDBTestSuite) TestState() {
	testCases := []struct {
		name       string
		key, value common.Hash
	}{
		{
			"set state - delete from store",
			common.BytesToHash([]byte("key")),
			common.Hash{},
		},
		{
			"set state - update value",
			common.BytesToHash([]byte("key")),
			common.BytesToHash([]byte("value")),
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			vmdb.SetState(suite.Address, tc.key, tc.value)
			value := vmdb.GetState(suite.Address, tc.key)
			suite.Require().Equal(tc.value, value)
		})
	}
}

func (suite *StateDBTestSuite) TestCommittedState() {
	key := common.BytesToHash([]byte("key"))
	value1 := common.BytesToHash([]byte("value1"))
	value2 := common.BytesToHash([]byte("value2"))

	vmdb := suite.StateDB()
	vmdb.SetState(suite.Address, key, value1)
	vmdb.Commit()

	vmdb = suite.StateDB()
	vmdb.SetState(suite.Address, key, value2)
	tmp := vmdb.GetState(suite.Address, key)
	suite.Require().Equal(value2, tmp)
	tmp = vmdb.GetCommittedState(suite.Address, key)
	suite.Require().Equal(value1, tmp)
	vmdb.Commit()

	vmdb = suite.StateDB()
	tmp = vmdb.GetCommittedState(suite.Address, key)
	suite.Require().Equal(value2, tmp)
}

func (suite *StateDBTestSuite) TestSuicide() {
	code := []byte("code")
	db := suite.StateDB()
	// Add code to account
	db.SetCode(suite.Address, code)
	suite.Require().Equal(code, db.GetCode(suite.Address))
	// Add state to account
	for i := 0; i < 5; i++ {
		db.SetState(suite.Address, common.BytesToHash([]byte(fmt.Sprintf("key%d", i))), common.BytesToHash([]byte(fmt.Sprintf("value%d", i))))
	}

	suite.Require().NoError(db.Commit())
	db = suite.StateDB()

	// Generate 2nd address
	privkey, _ := ethsecp256k1.GenerateKey()
	key, err := privkey.ToECDSA()
	suite.Require().NoError(err)
	addr2 := crypto.PubkeyToAddress(key.PublicKey)

	// Add code and state to account 2
	db.SetCode(addr2, code)
	suite.Require().Equal(code, db.GetCode(addr2))
	for i := 0; i < 5; i++ {
		db.SetState(addr2, common.BytesToHash([]byte(fmt.Sprintf("key%d", i))), common.BytesToHash([]byte(fmt.Sprintf("value%d", i))))
	}

	// Call Suicide
	suite.Require().Equal(true, db.Suicide(suite.Address))

	// Check suicided is marked
	suite.Require().Equal(true, db.HasSuicided(suite.Address))

	// Commit state
	suite.Require().NoError(db.Commit())
	db = suite.StateDB()

	// Check code is deleted
	suite.Require().Nil(db.GetCode(suite.Address))
	// Check state is deleted
	var storage types.Storage
	suite.App.EvmKeeper.ForEachStorage(suite.Ctx, suite.Address, func(key, value common.Hash) bool {
		storage = append(storage, types.NewState(key, value))
		return true
	})
	suite.Require().Equal(0, len(storage))

	// Check account is deleted
	suite.Require().Equal(common.Hash{}, db.GetCodeHash(suite.Address))

	// Check code is still present in addr2 and suicided is false
	suite.Require().NotNil(db.GetCode(addr2))
	suite.Require().Equal(false, db.HasSuicided(addr2))
}

func (suite *StateDBTestSuite) TestExist() {
	testCases := []struct {
		name     string
		address  common.Address
		malleate func(vm.StateDB)
		exists   bool
	}{
		{"success, account exists", suite.Address, func(vm.StateDB) {}, true},
		{"success, has suicided", suite.Address, func(vmdb vm.StateDB) {
			vmdb.Suicide(suite.Address)
		}, true},
		{"success, account doesn't exist", tests.GenerateAddress(), func(vm.StateDB) {}, false},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			suite.Require().Equal(tc.exists, vmdb.Exist(tc.address))
		})
	}
}

func (suite *StateDBTestSuite) TestEmpty() {
	testCases := []struct {
		name     string
		address  common.Address
		malleate func(vm.StateDB)
		empty    bool
	}{
		{"empty, account exists", suite.Address, func(vm.StateDB) {}, true},
		{
			"not empty, positive balance",
			suite.Address,
			func(vmdb vm.StateDB) { vmdb.AddBalance(suite.Address, big.NewInt(100)) },
			false,
		},
		{"empty, account doesn't exist", tests.GenerateAddress(), func(vm.StateDB) {}, true},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			suite.Require().Equal(tc.empty, vmdb.Empty(tc.address))
		})
	}
}

func (suite *StateDBTestSuite) TestSnapshot() {
	key := common.BytesToHash([]byte("key"))
	value1 := common.BytesToHash([]byte("value1"))
	value2 := common.BytesToHash([]byte("value2"))

	testCases := []struct {
		name     string
		malleate func(vm.StateDB)
	}{
		{"simple revert", func(vmdb vm.StateDB) {
			revision := vmdb.Snapshot()
			suite.Require().Zero(revision)

			vmdb.SetState(suite.Address, key, value1)
			suite.Require().Equal(value1, vmdb.GetState(suite.Address, key))

			vmdb.RevertToSnapshot(revision)

			// reverted
			suite.Require().Equal(common.Hash{}, vmdb.GetState(suite.Address, key))
		}},
		{"nested snapshot/revert", func(vmdb vm.StateDB) {
			revision1 := vmdb.Snapshot()
			suite.Require().Zero(revision1)

			vmdb.SetState(suite.Address, key, value1)

			revision2 := vmdb.Snapshot()

			vmdb.SetState(suite.Address, key, value2)
			suite.Require().Equal(value2, vmdb.GetState(suite.Address, key))

			vmdb.RevertToSnapshot(revision2)
			suite.Require().Equal(value1, vmdb.GetState(suite.Address, key))

			vmdb.RevertToSnapshot(revision1)
			suite.Require().Equal(common.Hash{}, vmdb.GetState(suite.Address, key))
		}},
		{"jump revert", func(vmdb vm.StateDB) {
			revision1 := vmdb.Snapshot()
			vmdb.SetState(suite.Address, key, value1)
			vmdb.Snapshot()
			vmdb.SetState(suite.Address, key, value2)
			vmdb.RevertToSnapshot(revision1)
			suite.Require().Equal(common.Hash{}, vmdb.GetState(suite.Address, key))
		}},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			vmdb := suite.StateDB()
			tc.malleate(vmdb)
		})
	}
}

func (suite *StateDBTestSuite) createTestTx(msg *types.MsgEthereumTx, priv cryptotypes.PrivKey) authsigning.Tx {
	option, err := codectypes.NewAnyWithValue(&types.ExtensionOptionsEthereumTx{})
	suite.Require().NoError(err)

	txBuilder := suite.clientCtx.TxConfig.NewTxBuilder()
	builder, ok := txBuilder.(authtx.ExtensionOptionsTxBuilder)
	suite.Require().True(ok)

	builder.SetExtensionOptions(option)

	err = msg.Sign(suite.ethSigner, tests.NewSigner(priv))
	suite.Require().NoError(err)

	err = txBuilder.SetMsgs(msg)
	suite.Require().NoError(err)

	return txBuilder.GetTx()
}

func (suite *StateDBTestSuite) TestAddLog() {
	chainID := big.NewInt(9000)
	addr, privKey := tests.NewAddrKey()
	msg := types.NewTx(chainID, 0, &suite.Address, big.NewInt(1), 100000, big.NewInt(1), nil, nil, []byte("test"), nil)
	msg.From = addr.Bytes()

	tx := suite.createTestTx(msg, privKey)
	msg, _ = tx.GetMsgs()[0].(*types.MsgEthereumTx)
	txHash := msg.AsTransaction().Hash()

	msg2 := types.NewTx(chainID, 1, &suite.Address, big.NewInt(1), 100000, big.NewInt(1), nil, nil, []byte("test"), nil)
	msg2.From = addr.Bytes()

	tx2 := suite.createTestTx(msg2, privKey)
	msg2, _ = tx2.GetMsgs()[0].(*types.MsgEthereumTx)

	msg3 := types.NewTx(chainID, 0, &suite.Address, big.NewInt(1), 100000, nil, big.NewInt(1), big.NewInt(1), []byte("test"), nil)
	msg3.From = addr.Bytes()

	tx3 := suite.createTestTx(msg3, privKey)
	msg3, _ = tx3.GetMsgs()[0].(*types.MsgEthereumTx)
	txHash3 := msg3.AsTransaction().Hash()

	msg4 := types.NewTx(chainID, 1, &suite.Address, big.NewInt(1), 100000, nil, big.NewInt(1), big.NewInt(1), []byte("test"), nil)
	msg4.From = addr.Bytes()

	tx4 := suite.createTestTx(msg4, privKey)
	msg4, _ = tx4.GetMsgs()[0].(*types.MsgEthereumTx)

	testCases := []struct {
		name        string
		hash        common.Hash
		log, expLog *ethtypes.Log // pre and post populating log fields
		malleate    func(vm.StateDB)
	}{
		{
			"tx hash from message",
			txHash,
			&ethtypes.Log{
				Address: addr,
				Topics:  make([]common.Hash, 0),
			},
			&ethtypes.Log{
				Address: addr,
				Topics:  make([]common.Hash, 0),
			},
			func(vm.StateDB) {},
		},
		{
			"dynamicfee tx hash from message",
			txHash3,
			&ethtypes.Log{
				Address: addr,
				Topics:  make([]common.Hash, 0),
			},
			&ethtypes.Log{
				Address: addr,
				Topics:  make([]common.Hash, 0),
			},
			func(vm.StateDB) {},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			vmdb := statedb.New(suite.Ctx, suite.App.EvmKeeper, statedb.NewTxConfig(
				common.BytesToHash(suite.Ctx.HeaderHash()),
				tc.hash,
				0, 0,
			))
			tc.malleate(vmdb)

			vmdb.AddLog(tc.log)
			logs := vmdb.Logs()
			suite.Require().Equal(1, len(logs))
			suite.Require().Equal(tc.expLog, logs[0])
		})
	}
}

func (suite *StateDBTestSuite) TestAddAddressToAccessList() {
	testCases := []struct {
		name string
		addr common.Address
	}{
		{"new address", suite.Address},
		{"existing address", suite.Address},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			vmdb.AddAddressToAccessList(tc.addr)
			addrOk := vmdb.AddressInAccessList(tc.addr)
			suite.Require().True(addrOk, tc.addr.Hex())
		})
	}
}

// FIXME skip for now
func (suite *StateDBTestSuite) _TestForEachStorage() {
	var storage types.Storage

	testCase := []struct {
		name      string
		malleate  func(vm.StateDB)
		callback  func(key, value common.Hash) (stop bool)
		expValues []common.Hash
	}{
		{
			"aggregate state",
			func(vmdb vm.StateDB) {
				for i := 0; i < 5; i++ {
					vmdb.SetState(suite.Address, common.BytesToHash([]byte(fmt.Sprintf("key%d", i))), common.BytesToHash([]byte(fmt.Sprintf("value%d", i))))
				}
			},
			func(key, value common.Hash) bool {
				storage = append(storage, types.NewState(key, value))
				return true
			},
			[]common.Hash{
				common.BytesToHash([]byte("value0")),
				common.BytesToHash([]byte("value1")),
				common.BytesToHash([]byte("value2")),
				common.BytesToHash([]byte("value3")),
				common.BytesToHash([]byte("value4")),
			},
		},
		{
			"filter state",
			func(vmdb vm.StateDB) {
				vmdb.SetState(suite.Address, common.BytesToHash([]byte("key")), common.BytesToHash([]byte("value")))
				vmdb.SetState(suite.Address, common.BytesToHash([]byte("filterkey")), common.BytesToHash([]byte("filtervalue")))
			},
			func(key, value common.Hash) bool {
				if value == common.BytesToHash([]byte("filtervalue")) {
					storage = append(storage, types.NewState(key, value))
					return false
				}
				return true
			},
			[]common.Hash{
				common.BytesToHash([]byte("filtervalue")),
			},
		},
	}

	for _, tc := range testCase {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			err := vmdb.ForEachStorage(suite.Address, tc.callback)
			suite.Require().NoError(err)
			suite.Require().Equal(len(tc.expValues), len(storage), fmt.Sprintf("Expected values:\n%v\nStorage Values\n%v", tc.expValues, storage))

			vals := make([]common.Hash, len(storage))
			for i := range storage {
				vals[i] = common.HexToHash(storage[i].Value)
			}

			// TODO: not sure why Equals fails
			suite.Require().ElementsMatch(tc.expValues, vals)
		})
		storage = types.Storage{}
	}
}

func (suite *StateDBTestSuite) TestSetBalance() {
	amount := big.NewInt(-10)

	testCases := []struct {
		name     string
		addr     common.Address
		malleate func()
		expErr   bool
	}{
		{
			"address without funds - invalid amount",
			suite.Address,
			func() {},
			true,
		},
		{
			"mint to address",
			suite.Address,
			func() {
				amount = big.NewInt(100)
			},
			false,
		},
		{
			"burn from address",
			suite.Address,
			func() {
				amount = big.NewInt(60)
			},
			false,
		},
		{
			"address with funds - invalid amount",
			suite.Address,
			func() {
				amount = big.NewInt(-10)
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.malleate()
			err := suite.App.EvmKeeper.SetBalance(suite.Ctx, tc.addr, amount, types.DefaultEVMDenom)
			if tc.expErr {
				suite.Require().Error(err)
			} else {
				balance := suite.App.EvmKeeper.GetEVMDenomBalance(suite.Ctx, tc.addr)
				suite.Require().NoError(err)
				suite.Require().Equal(amount, balance)
			}
		})
	}
}

func (suite *StateDBTestSuite) TestDeleteAccount() {
	supply := big.NewInt(100)
	testCases := []struct {
		name   string
		addr   func() common.Address
		expErr bool
	}{
		{
			"remove address",
			func() common.Address {
				return suite.Address
			},
			false,
		},
		{
			"remove unexistent address - returns nil error",
			func() common.Address {
				return common.HexToAddress("unexistent_address")
			},
			false,
		},
		{
			"remove deployed contract",
			func() common.Address {
				return suite.EVMTestSuiteWithAccountAndQueryClient.DeployTestContract(
					suite.T(),
					suite.Address,
					supply,
					false,
				)
			},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			err := suite.App.EvmKeeper.DeleteAccount(suite.Ctx, tc.addr())
			if tc.expErr {
				suite.Require().Error(err)
			} else {
				suite.Require().NoError(err)
				balance := suite.App.EvmKeeper.GetEVMDenomBalance(suite.Ctx, tc.addr())
				suite.Require().Equal(new(big.Int), balance)
			}
		})
	}
}
