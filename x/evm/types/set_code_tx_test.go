package types

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/tests"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/suite"
)

type SetCodeTxTestSuite struct {
	suite.Suite

	sdkInt         sdkmath.Int
	uint64         uint64
	hexUint64      hexutil.Uint64
	uint256Int     *uint256.Int
	sdkZeroInt     sdkmath.Int
	sdkMinusOneInt sdkmath.Int
	invalidAddr    string
	addr           common.Address
	hexAddr        string
	hexDataBytes   hexutil.Bytes
	hexInputBytes  hexutil.Bytes
}

func (suite *SetCodeTxTestSuite) SetupTest() {
	suite.sdkInt = sdkmath.NewInt(100)
	suite.uint64 = suite.sdkInt.Uint64()
	suite.hexUint64 = hexutil.Uint64(100)
	suite.uint256Int = uint256.NewInt(1)
	suite.sdkZeroInt = sdkmath.ZeroInt()
	suite.sdkMinusOneInt = sdkmath.NewInt(-1)
	suite.invalidAddr = "123456"
	suite.addr = tests.GenerateAddress()
	suite.hexAddr = suite.addr.Hex()
	suite.hexDataBytes = hexutil.Bytes([]byte("data"))
	suite.hexInputBytes = hexutil.Bytes([]byte("input"))
}

func TestSetCodeTxTestSuite(t *testing.T) {
	suite.Run(t, new(SetCodeTxTestSuite))
}

func (suite *SetCodeTxTestSuite) TestNewSetCodeTx() {
	testCases := []struct {
		name string
		tx   *ethtypes.Transaction
	}{
		{
			"non-empty tx",
			ethtypes.NewTx(&ethtypes.SetCodeTx{
				Nonce:      1,
				Data:       []byte("data"),
				Gas:        100,
				Value:      uint256.NewInt(1),
				AccessList: ethtypes.AccessList{},
				AuthList:   []ethtypes.SetCodeAuthorization{},
				To:         suite.addr,
				V:          suite.uint256Int,
				R:          suite.uint256Int,
				S:          suite.uint256Int,
			}),
		},
	}
	for _, tc := range testCases {
		tx, err := newSetCodeTx(tc.tx)
		suite.Require().NoError(err)
		suite.Require().NotEmpty(tx)
		suite.Require().Equal(uint8(4), tx.TxType())
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxAsEthereumData() {
	feeConfig := &ethtypes.SetCodeTx{
		Nonce:      1,
		Data:       []byte("data"),
		Gas:        100,
		Value:      uint256.NewInt(1),
		AccessList: ethtypes.AccessList{},
		To:         suite.addr,
		V:          suite.uint256Int,
		R:          suite.uint256Int,
		S:          suite.uint256Int,
	}

	tx := ethtypes.NewTx(feeConfig)

	SetCodeTx, err := newSetCodeTx(tx)
	suite.Require().NoError(err)

	res := SetCodeTx.AsEthereumData()
	resTx := ethtypes.NewTx(res)

	suite.Require().Equal(feeConfig.Nonce, resTx.Nonce())
	suite.Require().Equal(feeConfig.Data, resTx.Data())
	suite.Require().Equal(feeConfig.Gas, resTx.Gas())
	suite.Require().Equal(feeConfig.Value.ToBig(), resTx.Value())
	suite.Require().Equal(feeConfig.AccessList, resTx.AccessList())
	suite.Require().Equal(feeConfig.To, *resTx.To())
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxCopy() {
	tx := &SetCodeTx{}
	txCopy := tx.Copy()

	suite.Require().Equal(&SetCodeTx{}, txCopy)
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetChainID() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  *big.Int
	}{
		{
			"empty chainID",
			SetCodeTx{
				ChainID: nil,
			},
			nil,
		},
		{
			"non-empty chainID",
			SetCodeTx{
				ChainID: &suite.sdkInt,
			},
			(&suite.sdkInt).BigInt(),
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetChainID()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetAccessList() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  ethtypes.AccessList
	}{
		{
			"empty accesses",
			SetCodeTx{
				Accesses: nil,
			},
			nil,
		},
		{
			"nil",
			SetCodeTx{
				Accesses: NewAccessList(nil),
			},
			nil,
		},
		{
			"non-empty accesses",
			SetCodeTx{
				Accesses: AccessList{
					{
						Address:     suite.hexAddr,
						StorageKeys: []string{},
					},
				},
			},
			ethtypes.AccessList{
				{
					Address:     suite.addr,
					StorageKeys: []common.Hash{},
				},
			},
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetAccessList()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetData() {
	testCases := []struct {
		name string
		tx   SetCodeTx
	}{
		{
			"non-empty transaction",
			SetCodeTx{
				Data: nil,
			},
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetData()

		suite.Require().Equal(tc.tx.Data, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetGas() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  uint64
	}{
		{
			"non-empty gas",
			SetCodeTx{
				GasLimit: suite.uint64,
			},
			suite.uint64,
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetGas()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetGasPrice() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  *big.Int
	}{
		{
			"non-empty gasFeeCap",
			SetCodeTx{
				GasFeeCap: &suite.sdkInt,
			},
			(&suite.sdkInt).BigInt(),
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetGasPrice()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetGasTipCap() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  *big.Int
	}{
		{
			"empty gasTipCap",
			SetCodeTx{
				GasTipCap: nil,
			},
			nil,
		},
		{
			"non-empty gasTipCap",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
			},
			(&suite.sdkInt).BigInt(),
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetGasTipCap()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetGasFeeCap() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  *big.Int
	}{
		{
			"empty gasFeeCap",
			SetCodeTx{
				GasFeeCap: nil,
			},
			nil,
		},
		{
			"non-empty gasFeeCap",
			SetCodeTx{
				GasFeeCap: &suite.sdkInt,
			},
			(&suite.sdkInt).BigInt(),
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetGasFeeCap()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetValue() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  *big.Int
	}{
		{
			"empty amount",
			SetCodeTx{
				Amount: nil,
			},
			nil,
		},
		{
			"non-empty amount",
			SetCodeTx{
				Amount: &suite.sdkInt,
			},
			(&suite.sdkInt).BigInt(),
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetValue()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetNonce() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  uint64
	}{
		{
			"non-empty nonce",
			SetCodeTx{
				Nonce: suite.uint64,
			},
			suite.uint64,
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetNonce()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxGetTo() {
	testCases := []struct {
		name string
		tx   SetCodeTx
		exp  *common.Address
	}{
		{
			"empty suite.address",
			SetCodeTx{
				To: "",
			},
			nil,
		},
		{
			"non-empty suite.address",
			SetCodeTx{
				To: suite.hexAddr,
			},
			&suite.addr,
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.GetTo()

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxSetSignatureValues() {
	testCases := []struct {
		name    string
		chainID *big.Int
		r       *big.Int
		v       *big.Int
		s       *big.Int
	}{
		{
			"empty values",
			nil,
			nil,
			nil,
			nil,
		},
		{
			"non-empty values",
			big.NewInt(1),
			big.NewInt(2),
			big.NewInt(3),
			big.NewInt(4),
		},
	}

	for _, tc := range testCases {
		tx := &SetCodeTx{}
		tx.SetSignatureValues(tc.chainID, tc.v, tc.r, tc.s)

		v, r, s := tx.GetRawSignatureValues()
		chainID := tx.GetChainID()

		suite.Require().Equal(tc.v, v, tc.name)
		suite.Require().Equal(tc.r, r, tc.name)
		suite.Require().Equal(tc.s, s, tc.name)
		suite.Require().Equal(tc.chainID, chainID, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxValidate() {
	testCases := []struct {
		name     string
		tx       SetCodeTx
		expError bool
	}{
		{
			"empty",
			SetCodeTx{},
			true,
		},
		{
			"gas tip cap is nil",
			SetCodeTx{
				GasTipCap: nil,
			},
			true,
		},
		{
			"gas fee cap is nil",
			SetCodeTx{
				GasTipCap: &suite.sdkZeroInt,
			},
			true,
		},
		{
			"gas tip cap is negative",
			SetCodeTx{
				GasTipCap: &suite.sdkMinusOneInt,
				GasFeeCap: &suite.sdkZeroInt,
			},
			true,
		},
		{
			"gas tip cap is negative",
			SetCodeTx{
				GasTipCap: &suite.sdkZeroInt,
				GasFeeCap: &suite.sdkMinusOneInt,
			},
			true,
		},
		{
			"gas fee cap < gas tip cap",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkZeroInt,
			},
			true,
		},
		{
			"amount is negative",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
				Amount:    &suite.sdkMinusOneInt,
			},
			true,
		},
		{
			"to suite.address is invalid",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
				Amount:    &suite.sdkInt,
				To:        suite.invalidAddr,
			},
			true,
		},
		{
			"chain ID not present on SetCode txs",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
				Amount:    &suite.sdkInt,
				To:        suite.hexAddr,
				ChainID:   nil,
			},
			true,
		},
		{
			"to address is empty",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
				Amount:    &suite.sdkInt,
				To:        "",
			},
			true,
		},
		{
			"auth list is empty",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
				Amount:    &suite.sdkInt,
				To:        suite.hexAddr,
				ChainID:   &suite.sdkInt,
			},
			true,
		},
		{
			"no errors",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
				Amount:    &suite.sdkInt,
				To:        suite.hexAddr,
				ChainID:   &suite.sdkInt,
				AuthList: []SetCodeAuthorization{
					{
						ChainID: &suite.sdkInt,
						Address: suite.addr.Hex(),
						Nonce:   suite.uint64,
						V:       []byte{1},
						R:       []byte{2},
						S:       []byte{3},
					},
				},
			},
			false,
		},
	}

	for _, tc := range testCases {
		err := tc.tx.Validate()

		if tc.expError {
			suite.Require().Error(err, tc.name)
			continue
		}

		suite.Require().NoError(err, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxEffectiveGasPrice() {
	testCases := []struct {
		name    string
		tx      SetCodeTx
		baseFee *big.Int
		exp     *big.Int
	}{
		{
			"non-empty dynamic fee tx",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
			},
			(&suite.sdkInt).BigInt(),
			(&suite.sdkInt).BigInt(),
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.EffectiveGasPrice(tc.baseFee)

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxEffectiveFee() {
	testCases := []struct {
		name    string
		tx      SetCodeTx
		baseFee *big.Int
		exp     *big.Int
	}{
		{
			"non-empty dynamic fee tx",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
				GasLimit:  uint64(1),
			},
			(&suite.sdkInt).BigInt(),
			(&suite.sdkInt).BigInt(),
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.EffectiveFee(tc.baseFee)

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxEffectiveCost() {
	testCases := []struct {
		name    string
		tx      SetCodeTx
		baseFee *big.Int
		exp     *big.Int
	}{
		{
			"non-empty dynamic fee tx",
			SetCodeTx{
				GasTipCap: &suite.sdkInt,
				GasFeeCap: &suite.sdkInt,
				GasLimit:  uint64(1),
				Amount:    &suite.sdkZeroInt,
			},
			(&suite.sdkInt).BigInt(),
			(&suite.sdkInt).BigInt(),
		},
	}

	for _, tc := range testCases {
		actual := tc.tx.EffectiveCost(tc.baseFee)

		suite.Require().Equal(tc.exp, actual, tc.name)
	}
}

func (suite *SetCodeTxTestSuite) TestSetCodeTxFeeCost() {
	tx := &SetCodeTx{}
	suite.Require().Panics(func() { tx.Fee() }, "should panic")
	suite.Require().Panics(func() { tx.Cost() }, "should panic")
}
