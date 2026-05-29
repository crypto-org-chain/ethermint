package types

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

func (suite *TxDataTestSuite) TestTxArgsString() {
	testCases := []struct {
		name           string
		txArgs         TransactionArgs
		expectedString string
	}{
		{
			"empty tx args",
			TransactionArgs{},
			"TransactionArgs{From:<nil>, To:<nil>, Gas:<nil>, Nonce:<nil>, Data:<nil>, Input:<nil>, AccessList:<nil>}",
		},
		{
			"tx args with fields",
			TransactionArgs{
				From:       &suite.addr,
				To:         &suite.addr,
				Gas:        &suite.hexUint64,
				Nonce:      &suite.hexUint64,
				Input:      &suite.hexInputBytes,
				Data:       &suite.hexDataBytes,
				AccessList: &ethtypes.AccessList{},
			},
			fmt.Sprintf("TransactionArgs{From:%v, To:%v, Gas:%v, Nonce:%v, Data:%v, Input:%v, AccessList:%v}",
				&suite.addr,
				&suite.addr,
				&suite.hexUint64,
				&suite.hexUint64,
				&suite.hexDataBytes,
				&suite.hexInputBytes,
				&ethtypes.AccessList{}),
		},
	}
	for _, tc := range testCases {
		outputString := tc.txArgs.String()
		suite.Require().Equal(outputString, tc.expectedString)
	}
}

func (suite *TxDataTestSuite) TestConvertTxArgsEthTx() {
	testCases := []struct {
		name   string
		txArgs TransactionArgs
	}{
		{
			"empty tx args",
			TransactionArgs{},
		},
		{
			"no nil args",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				GasPrice:             &suite.hexBigInt,
				MaxFeePerGas:         &suite.hexBigInt,
				MaxPriorityFeePerGas: &suite.hexBigInt,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
				Data:                 &suite.hexDataBytes,
				Input:                &suite.hexInputBytes,
				AccessList:           &ethtypes.AccessList{{Address: suite.addr, StorageKeys: []common.Hash{{0}}}},
				ChainID:              &suite.hexBigInt,
			},
		},
		{
			"max fee per gas nil, but access list not nil",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				GasPrice:             &suite.hexBigInt,
				MaxFeePerGas:         nil,
				MaxPriorityFeePerGas: &suite.hexBigInt,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
				Data:                 &suite.hexDataBytes,
				Input:                &suite.hexInputBytes,
				AccessList:           &ethtypes.AccessList{{Address: suite.addr, StorageKeys: []common.Hash{{0}}}},
				ChainID:              &suite.hexBigInt,
			},
		},
	}
	for _, tc := range testCases {
		res := tc.txArgs.ToTransaction()
		suite.Require().NotNil(res)
	}
}

func (suite *TxDataTestSuite) TestToMessageEVM() {
	testCases := []struct {
		name         string
		txArgs       TransactionArgs
		globalGasCap uint64
		baseFee      *big.Int
		expError     bool
	}{
		{
			"empty tx args",
			TransactionArgs{},
			uint64(0),
			nil,
			false,
		},
		{
			"specify gasPrice and (maxFeePerGas or maxPriorityFeePerGas)",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				GasPrice:             &suite.hexBigInt,
				MaxFeePerGas:         &suite.hexBigInt,
				MaxPriorityFeePerGas: &suite.hexBigInt,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
				Data:                 &suite.hexDataBytes,
				Input:                &suite.hexInputBytes,
				AccessList:           &ethtypes.AccessList{{Address: suite.addr, StorageKeys: []common.Hash{{0}}}},
				ChainID:              &suite.hexBigInt,
			},
			uint64(0),
			nil,
			true,
		},
		{
			"non-1559 execution, zero gas cap",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				GasPrice:             &suite.hexBigInt,
				MaxFeePerGas:         nil,
				MaxPriorityFeePerGas: nil,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
				Data:                 &suite.hexDataBytes,
				Input:                &suite.hexInputBytes,
				AccessList:           &ethtypes.AccessList{{Address: suite.addr, StorageKeys: []common.Hash{{0}}}},
				ChainID:              &suite.hexBigInt,
			},
			uint64(0),
			nil,
			false,
		},
		{
			"non-1559 execution, nonzero gas cap",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				GasPrice:             &suite.hexBigInt,
				MaxFeePerGas:         nil,
				MaxPriorityFeePerGas: nil,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
				Data:                 &suite.hexDataBytes,
				Input:                &suite.hexInputBytes,
				AccessList:           &ethtypes.AccessList{{Address: suite.addr, StorageKeys: []common.Hash{{0}}}},
				ChainID:              &suite.hexBigInt,
			},
			uint64(1),
			nil,
			false,
		},
		{
			"1559-type execution, nil gas price",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				GasPrice:             nil,
				MaxFeePerGas:         &suite.hexBigInt,
				MaxPriorityFeePerGas: &suite.hexBigInt,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
				Data:                 &suite.hexDataBytes,
				Input:                &suite.hexInputBytes,
				AccessList:           &ethtypes.AccessList{{Address: suite.addr, StorageKeys: []common.Hash{{0}}}},
				ChainID:              &suite.hexBigInt,
			},
			uint64(1),
			suite.bigInt,
			false,
		},
		{
			"1559-type execution, non-nil gas price",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				GasPrice:             &suite.hexBigInt,
				MaxFeePerGas:         nil,
				MaxPriorityFeePerGas: nil,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
				Data:                 &suite.hexDataBytes,
				Input:                &suite.hexInputBytes,
				AccessList:           &ethtypes.AccessList{{Address: suite.addr, StorageKeys: []common.Hash{{0}}}},
				ChainID:              &suite.hexBigInt,
			},
			uint64(1),
			suite.bigInt,
			false,
		},
	}
	for _, tc := range testCases {
		res, err := tc.txArgs.ToMessage(tc.globalGasCap, tc.baseFee)

		if tc.expError {
			suite.Require().NotNil(err)
		} else {
			suite.Require().Nil(err)
			suite.Require().NotNil(res)
		}
	}
}

func (suite *TxDataTestSuite) TestGetFrom() {
	testCases := []struct {
		name       string
		txArgs     TransactionArgs
		expAddress common.Address
	}{
		{
			"empty from field",
			TransactionArgs{},
			common.Address{},
		},
		{
			"non-empty from field",
			TransactionArgs{
				From: &suite.addr,
			},
			suite.addr,
		},
	}
	for _, tc := range testCases {
		retrievedAddress := tc.txArgs.GetFrom()
		suite.Require().Equal(retrievedAddress, tc.expAddress)
	}
}

func (suite *TxDataTestSuite) TestGetData() {
	testCases := []struct {
		name           string
		txArgs         TransactionArgs
		expectedOutput []byte
	}{
		{
			"empty input and data fields",
			TransactionArgs{
				Data:  nil,
				Input: nil,
			},
			nil,
		},
		{
			"empty input field, non-empty data field",
			TransactionArgs{
				Data:  &suite.hexDataBytes,
				Input: nil,
			},
			[]byte("data"),
		},
		{
			"non-empty input and data fields",
			TransactionArgs{
				Data:  &suite.hexDataBytes,
				Input: &suite.hexInputBytes,
			},
			[]byte("input"),
		},
	}
	for _, tc := range testCases {
		retrievedData := tc.txArgs.GetData()
		suite.Require().Equal(retrievedData, tc.expectedOutput)
	}
}

func (suite *TxDataTestSuite) TestMaxGasCap() {
	testCases := []struct {
		name           string
		globalGasCap   uint64
		txArgs         TransactionArgs
		expectedOutput uint64
	}{
		{
			"globalGasCap is below default gas",
			25000000,
			TransactionArgs{
				Gas:   nil,
				Input: nil,
			},
			25000000,
		},
		{
			"globalGasCap is above default gas",
			math.MaxInt64,
			TransactionArgs{
				Gas:   nil,
				Input: nil,
			},
			100000000,
		},
		{
			"globalGasCap is zero",
			0,
			TransactionArgs{
				Gas:   nil,
				Input: nil,
			},
			100000000,
		},
	}
	for _, tc := range testCases {
		res, err := tc.txArgs.ToMessage(tc.globalGasCap, nil)
		suite.Require().Nil(err)
		suite.Require().Equal(res.GasLimit, tc.expectedOutput)
	}
}

// ---------------------------------------------------------------------------
// ToSimMessage
// ---------------------------------------------------------------------------

func (suite *TxDataTestSuite) TestToSimMessage() {
	chainID := big.NewInt(9001)
	baseFee := big.NewInt(1e9)

	testCases := []struct {
		name      string
		txArgs    TransactionArgs
		baseFee   *big.Int
		skipNonce bool
		expError  bool
	}{
		{
			"empty args, no basefee, skip nonce",
			TransactionArgs{},
			nil,
			true,
			false,
		},
		{
			"both gasPrice and maxFeePerGas specified",
			TransactionArgs{
				GasPrice:     &suite.hexBigInt,
				MaxFeePerGas: &suite.hexBigInt,
			},
			nil,
			false,
			true,
		},
		{
			"non-1559: gasPrice specified",
			TransactionArgs{
				From:     &suite.addr,
				To:       &suite.addr,
				Gas:      &suite.hexUint64,
				GasPrice: &suite.hexBigInt,
				Value:    &suite.hexBigInt,
				Nonce:    &suite.hexUint64,
			},
			nil,
			false,
			false,
		},
		{
			"1559: gasPrice specified with basefee (legacy conversion)",
			TransactionArgs{
				From:     &suite.addr,
				To:       &suite.addr,
				Gas:      &suite.hexUint64,
				GasPrice: &suite.hexBigInt,
				Value:    &suite.hexBigInt,
				Nonce:    &suite.hexUint64,
			},
			baseFee,
			false,
			false,
		},
		{
			"1559: maxFeePerGas and maxPriorityFeePerGas",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				MaxFeePerGas:         &suite.hexBigInt,
				MaxPriorityFeePerGas: &suite.hexBigInt,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
			},
			baseFee,
			true,
			false,
		},
		{
			"1559: nil MaxFeePerGas and nil MaxPriorityFeePerGas with basefee",
			TransactionArgs{
				From:  &suite.addr,
				To:    &suite.addr,
				Nonce: &suite.hexUint64,
			},
			baseFee,
			true,
			false,
		},
		{
			"with access list and authorization list",
			TransactionArgs{
				From:                 &suite.addr,
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				MaxFeePerGas:         &suite.hexBigInt,
				MaxPriorityFeePerGas: &suite.hexBigInt,
				AccessList:           &ethtypes.AccessList{{Address: suite.addr}},
				ChainID:              (*hexutil.Big)(chainID),
				Nonce:                &suite.hexUint64,
			},
			baseFee,
			false,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			msg, err := tc.txArgs.ToSimMessage(tc.baseFee, tc.skipNonce)
			if tc.expError {
				suite.Require().Error(err)
			} else {
				suite.Require().NoError(err)
				suite.Require().NotNil(msg)
				suite.Require().Equal(tc.skipNonce, msg.SkipNonceChecks)
				suite.Require().True(msg.SkipTransactionChecks)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CallDefaults
// ---------------------------------------------------------------------------

func (suite *TxDataTestSuite) TestCallDefaults() {
	chainID := big.NewInt(9001)
	wrongChainID := big.NewInt(1)
	baseFee := big.NewInt(1e9)

	testCases := []struct {
		name      string
		txArgs    TransactionArgs
		globalGas uint64
		baseFee   *big.Int
		chainID   *big.Int
		expError  bool
		expErrMsg string
	}{
		{
			"empty args - nil chainID set from provided",
			TransactionArgs{},
			0,
			nil,
			chainID,
			false,
			"",
		},
		{
			"both gasPrice and maxFeePerGas error",
			TransactionArgs{
				GasPrice:     &suite.hexBigInt,
				MaxFeePerGas: &suite.hexBigInt,
			},
			0,
			nil,
			chainID,
			true,
			"both gasPrice and",
		},
		{
			"chainID mismatch error",
			TransactionArgs{
				ChainID: (*hexutil.Big)(wrongChainID),
			},
			0,
			nil,
			chainID,
			true,
			"chainId does not match",
		},
		{
			"nil gas with globalGasCap 0 - gets MaxUint64/2",
			TransactionArgs{},
			0,
			nil,
			chainID,
			false,
			"",
		},
		{
			"nil gas with globalGasCap > 0",
			TransactionArgs{},
			25_000_000,
			nil,
			chainID,
			false,
			"",
		},
		{
			"gas exceeds globalGasCap - capped",
			TransactionArgs{
				Gas: (*hexutil.Uint64)(func() *uint64 { g := uint64(50_000_000); return &g }()),
			},
			25_000_000,
			nil,
			chainID,
			false,
			"",
		},
		{
			"no baseFee - sets GasPrice to zero",
			TransactionArgs{},
			0,
			nil,
			chainID,
			false,
			"",
		},
		{
			"with baseFee - sets MaxFeePerGas and MaxPriorityFeePerGas",
			TransactionArgs{},
			0,
			baseFee,
			chainID,
			false,
			"",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			err := tc.txArgs.CallDefaults(tc.globalGas, tc.baseFee, tc.chainID)
			if tc.expError {
				suite.Require().Error(err)
				if tc.expErrMsg != "" {
					suite.Require().Contains(err.Error(), tc.expErrMsg)
				}
			} else {
				suite.Require().NoError(err)
				// Gas should always be set
				suite.Require().NotNil(tc.txArgs.Gas)
				// Nonce, Value should be set
				suite.Require().NotNil(tc.txArgs.Nonce)
				suite.Require().NotNil(tc.txArgs.Value)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToEthTransaction
// ---------------------------------------------------------------------------

func (suite *TxDataTestSuite) TestToEthTransaction() {
	chainID := big.NewInt(9001)

	testCases := []struct {
		name   string
		txArgs TransactionArgs
	}{
		{
			"legacy tx - default case",
			TransactionArgs{
				To:       &suite.addr,
				Gas:      &suite.hexUint64,
				GasPrice: &suite.hexBigInt,
				Value:    &suite.hexBigInt,
				Nonce:    &suite.hexUint64,
			},
		},
		{
			"access list tx",
			TransactionArgs{
				To:         &suite.addr,
				Gas:        &suite.hexUint64,
				GasPrice:   &suite.hexBigInt,
				Value:      &suite.hexBigInt,
				Nonce:      &suite.hexUint64,
				AccessList: &ethtypes.AccessList{{Address: suite.addr}},
				ChainID:    (*hexutil.Big)(chainID),
			},
		},
		{
			"dynamic fee tx",
			TransactionArgs{
				To:                   &suite.addr,
				Gas:                  &suite.hexUint64,
				MaxFeePerGas:         &suite.hexBigInt,
				MaxPriorityFeePerGas: &suite.hexBigInt,
				Value:                &suite.hexBigInt,
				Nonce:                &suite.hexUint64,
				ChainID:              (*hexutil.Big)(chainID),
			},
		},
		{
			"nil gas and nonce",
			TransactionArgs{
				To: &suite.addr,
			},
		},
		{
			"contract creation (nil To)",
			TransactionArgs{
				Gas:      &suite.hexUint64,
				GasPrice: &suite.hexBigInt,
				Value:    &suite.hexBigInt,
				Nonce:    &suite.hexUint64,
				Data:     &suite.hexDataBytes,
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			tx := tc.txArgs.ToEthTransaction()
			suite.Require().NotNil(tx)
		})
	}
}

// ---------------------------------------------------------------------------
// ToTransaction — SetCodeTx branch
// ---------------------------------------------------------------------------

func (suite *TxDataTestSuite) TestToTransactionSetCode() {
	chainID := big.NewInt(9001)
	nonce := hexutil.Uint64(1)
	gas := hexutil.Uint64(21000)
	val := hexutil.Big(*big.NewInt(0))
	maxFee := hexutil.Big(*big.NewInt(2e9))
	maxTip := hexutil.Big(*big.NewInt(1e8))

	authList := []ethtypes.SetCodeAuthorization{
		{Address: suite.addr},
	}

	args := TransactionArgs{
		To:                   &suite.addr,
		ChainID:              (*hexutil.Big)(chainID),
		Nonce:                &nonce,
		Gas:                  &gas,
		Value:                &val,
		MaxFeePerGas:         &maxFee,
		MaxPriorityFeePerGas: &maxTip,
		AuthorizationList:    authList,
	}
	tx := args.ToTransaction()
	suite.Require().NotNil(tx)
}

// ---------------------------------------------------------------------------
// ToEthTransaction — SetCodeTx branch
// ---------------------------------------------------------------------------

func (suite *TxDataTestSuite) TestToEthTransactionSetCode() {
	chainID := big.NewInt(9001)
	nonce := hexutil.Uint64(1)
	gas := hexutil.Uint64(21000)
	val := hexutil.Big(*big.NewInt(0))
	maxFee := hexutil.Big(*big.NewInt(2e9))
	maxTip := hexutil.Big(*big.NewInt(1e8))

	authList := []ethtypes.SetCodeAuthorization{
		{Address: suite.addr},
	}

	args := TransactionArgs{
		To:                   &suite.addr,
		ChainID:              (*hexutil.Big)(chainID),
		Nonce:                &nonce,
		Gas:                  &gas,
		Value:                &val,
		MaxFeePerGas:         &maxFee,
		MaxPriorityFeePerGas: &maxTip,
		AuthorizationList:    authList,
	}
	tx := args.ToEthTransaction()
	suite.Require().NotNil(tx)
	suite.Require().Equal(uint8(ethtypes.SetCodeTxType), tx.Type())
}
