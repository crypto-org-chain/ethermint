package types

import (
	"math/big"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var (
	testChainID   = big.NewInt(9000)
	testAddress   = common.HexToAddress("0x1234567890123456789012345678901234567890")
	testBlockHash = common.HexToHash("0xa9d32b77fbfe2f9310b9eb8a29138b95ca3da6b04a4432e1d14c360644a9b8c7")
	testSigner    keyring.Signer
	testFromAddr  common.Address
)

func init() {
	privKey, _ := ethsecp256k1.GenerateKey()
	testSigner = tests.NewSigner(privKey)
	testFromAddr = common.BytesToAddress(privKey.PubKey().Address().Bytes())
}

func buildLegacyTx(t *testing.T) *evmtypes.MsgEthereumTx {
	tx := evmtypes.NewTx(
		testChainID,
		0,
		&testAddress,
		big.NewInt(1000),
		100000,
		big.NewInt(1000000000),
		nil,
		nil,
		nil,
		nil,
	)
	tx.From = testFromAddr.Bytes()

	err := tx.Sign(ethtypes.LatestSignerForChainID(testChainID), testSigner)
	require.NoError(t, err)

	return tx
}

func buildDynamicFeeTx(t *testing.T) *evmtypes.MsgEthereumTx {
	tx := evmtypes.NewTx(
		testChainID,
		1,
		&testAddress,
		big.NewInt(2000),
		120000,
		nil,
		big.NewInt(2000000000),
		big.NewInt(1000000000),
		[]byte("test data"),
		&ethtypes.AccessList{},
	)
	tx.From = testFromAddr.Bytes()

	err := tx.Sign(ethtypes.LatestSignerForChainID(testChainID), testSigner)
	require.NoError(t, err)

	return tx
}

func buildSetCodeTx(t *testing.T) *evmtypes.MsgEthereumTx {
	auth := ethtypes.SetCodeAuthorization{
		ChainID: *uint256.MustFromBig(testChainID),
		Address: testAddress,
		Nonce:   1,
		V:       uint8(27),
		R:       *uint256.NewInt(1),
		S:       *uint256.NewInt(1),
	}

	setCodeTx := &ethtypes.SetCodeTx{
		ChainID:    uint256.MustFromBig(testChainID),
		Nonce:      2,
		GasTipCap:  uint256.NewInt(1000000000),
		GasFeeCap:  uint256.NewInt(2000000000),
		Gas:        100000,
		To:         testAddress,
		Value:      uint256.NewInt(3000),
		Data:       []byte("setcode data"),
		AccessList: ethtypes.AccessList{},
		AuthList:   []ethtypes.SetCodeAuthorization{auth},
		V:          uint256.NewInt(1),
		R:          uint256.NewInt(1),
		S:          uint256.NewInt(1),
	}

	ethTx := ethtypes.NewTx(setCodeTx)
	msgEthereumTx := &evmtypes.MsgEthereumTx{}
	err := msgEthereumTx.FromSignedEthereumTx(ethTx, ethtypes.LatestSignerForChainID(testChainID))
	require.NoError(t, err)

	msgEthereumTx.From = testFromAddr.Bytes()
	return msgEthereumTx
}

func TestNewRPCTransaction(t *testing.T) {
	testCases := []struct {
		name           string
		setupTx        func() *evmtypes.MsgEthereumTx
		blockHash      common.Hash
		blockNumber    uint64
		index          uint64
		baseFee        *big.Int
		chainID        *big.Int
		expectError    bool
		validateResult func(t *testing.T, result *RPCTransaction)
	}{
		{
			name:        "Legacy transaction - pending",
			setupTx:     func() *evmtypes.MsgEthereumTx { return buildLegacyTx(t) },
			blockHash:   common.Hash{},
			blockNumber: 0,
			index:       0,
			baseFee:     nil,
			chainID:     testChainID,
			expectError: false,
			validateResult: func(t *testing.T, result *RPCTransaction) {
				require.Equal(t, hexutil.Uint64(ethtypes.LegacyTxType), result.Type)
				require.Equal(t, testFromAddr, result.From)
				require.Equal(t, &testAddress, result.To)
				require.Equal(t, (*hexutil.Big)(big.NewInt(1000)), result.Value)
				require.Equal(t, hexutil.Uint64(100000), result.Gas)
				require.Equal(t, (*hexutil.Big)(big.NewInt(1000000000)), result.GasPrice)
				require.Nil(t, result.BlockHash)
				require.Nil(t, result.BlockNumber)
				require.Nil(t, result.TransactionIndex)
				require.Nil(t, result.Accesses)
				require.Nil(t, result.GasFeeCap)
				require.Nil(t, result.GasTipCap)
			},
		},
		{
			name:        "Legacy transaction - mined",
			setupTx:     func() *evmtypes.MsgEthereumTx { return buildLegacyTx(t) },
			blockHash:   testBlockHash,
			blockNumber: 100,
			index:       5,
			baseFee:     big.NewInt(500000000),
			chainID:     testChainID,
			expectError: false,
			validateResult: func(t *testing.T, result *RPCTransaction) {
				require.Equal(t, hexutil.Uint64(ethtypes.LegacyTxType), result.Type)
				require.Equal(t, &testBlockHash, result.BlockHash)
				require.Equal(t, (*hexutil.Big)(big.NewInt(100)), result.BlockNumber)
				idx := hexutil.Uint64(5)
				require.Equal(t, &idx, result.TransactionIndex)
			},
		},
		{
			name:        "Dynamic fee transaction - pending",
			setupTx:     func() *evmtypes.MsgEthereumTx { return buildDynamicFeeTx(t) },
			blockHash:   common.Hash{},
			blockNumber: 0,
			index:       0,
			baseFee:     nil,
			chainID:     testChainID,
			expectError: false,
			validateResult: func(t *testing.T, result *RPCTransaction) {
				require.Equal(t, hexutil.Uint64(ethtypes.DynamicFeeTxType), result.Type)
				require.Equal(t, testFromAddr, result.From)
				require.Equal(t, &testAddress, result.To)
				require.Equal(t, (*hexutil.Big)(big.NewInt(2000)), result.Value)
				require.Equal(t, hexutil.Uint64(120000), result.Gas)
				require.Equal(t, (*hexutil.Big)(big.NewInt(2000000000)), result.GasFeeCap)
				require.Equal(t, (*hexutil.Big)(big.NewInt(1000000000)), result.GasTipCap)
				require.NotNil(t, result.Accesses)
				require.NotNil(t, result.YParity)
				require.Equal(t, hexutil.Bytes([]byte("test data")), result.Input)
			},
		},
		{
			name:        "Dynamic fee transaction - mined with baseFee",
			setupTx:     func() *evmtypes.MsgEthereumTx { return buildDynamicFeeTx(t) },
			blockHash:   testBlockHash,
			blockNumber: 200,
			index:       3,
			baseFee:     big.NewInt(500000000),
			chainID:     testChainID,
			expectError: false,
			validateResult: func(t *testing.T, result *RPCTransaction) {
				require.Equal(t, hexutil.Uint64(ethtypes.DynamicFeeTxType), result.Type)
				expectedPrice := big.NewInt(1500000000)
				require.Equal(t, (*hexutil.Big)(expectedPrice), result.GasPrice)
				require.Equal(t, &testBlockHash, result.BlockHash)
			},
		},
		{
			name:        "SetCode transaction - pending",
			setupTx:     func() *evmtypes.MsgEthereumTx { return buildSetCodeTx(t) },
			blockHash:   common.Hash{},
			blockNumber: 0,
			index:       0,
			baseFee:     nil,
			chainID:     testChainID,
			expectError: false,
			validateResult: func(t *testing.T, result *RPCTransaction) {
				require.Equal(t, hexutil.Uint64(ethtypes.SetCodeTxType), result.Type)
				require.Equal(t, testFromAddr, result.From)
				require.Equal(t, &testAddress, result.To)
				require.Equal(t, (*hexutil.Big)(big.NewInt(3000)), result.Value)
				require.Equal(t, hexutil.Uint64(100000), result.Gas)
				require.Equal(t, (*hexutil.Big)(big.NewInt(2000000000)), result.GasFeeCap)
				require.Equal(t, (*hexutil.Big)(big.NewInt(1000000000)), result.GasTipCap)
				require.NotNil(t, result.Accesses)
				require.NotNil(t, result.YParity)
				require.NotNil(t, result.AuthorizationList)
				require.Len(t, result.AuthorizationList, 1)
				auth := result.AuthorizationList[0]
				require.Equal(t, testAddress, auth.Address)
				require.Equal(t, *uint256.MustFromBig(testChainID), auth.ChainID)
				require.Equal(t, uint64(1), auth.Nonce)
				require.Equal(t, uint8(27), auth.V)
				require.Equal(t, *uint256.NewInt(1), auth.R)
				require.Equal(t, *uint256.NewInt(1), auth.S)
				require.Equal(t, hexutil.Bytes([]byte("setcode data")), result.Input)
			},
		},
		{
			name:        "SetCode transaction - mined with baseFee",
			setupTx:     func() *evmtypes.MsgEthereumTx { return buildSetCodeTx(t) },
			blockHash:   testBlockHash,
			blockNumber: 300,
			index:       7,
			baseFee:     big.NewInt(800000000),
			chainID:     testChainID,
			expectError: false,
			validateResult: func(t *testing.T, result *RPCTransaction) {
				require.Equal(t, hexutil.Uint64(ethtypes.SetCodeTxType), result.Type)
				expectedPrice := big.NewInt(1800000000)
				require.Equal(t, (*hexutil.Big)(expectedPrice), result.GasPrice)
				require.Equal(t, &testBlockHash, result.BlockHash)
				require.NotNil(t, result.AuthorizationList)
				require.Len(t, result.AuthorizationList, 1)
				auth := result.AuthorizationList[0]
				require.Equal(t, testAddress, auth.Address)
				require.Equal(t, *uint256.MustFromBig(testChainID), auth.ChainID)
				require.Equal(t, uint64(1), auth.Nonce)
				require.Equal(t, uint8(27), auth.V)
				require.Equal(t, *uint256.NewInt(1), auth.R)
				require.Equal(t, *uint256.NewInt(1), auth.S)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.setupTx()

			result, err := NewRPCTransaction(
				msg,
				tc.blockHash,
				tc.blockNumber,
				0,
				tc.index,
				tc.baseFee,
				tc.chainID,
			)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				require.NotEmpty(t, result.Hash)
				require.NotNil(t, result.V)
				require.NotNil(t, result.R)
				require.NotNil(t, result.S)

				if tc.validateResult != nil {
					tc.validateResult(t, result)
				}
			}
		})
	}
}

func TestNewRPCTransaction_BlockTimestamp(t *testing.T) {
	t.Parallel()

	blockHash := common.HexToHash("0xaabb")
	const blockTime = uint64(1234567890)
	msg := buildLegacyTx(t)

	t.Run("set when in block", func(t *testing.T) {
		result, err := NewRPCTransaction(msg, blockHash, 10, blockTime, 0, nil, testChainID)
		require.NoError(t, err)
		require.NotNil(t, result.BlockTimestamp)
		require.Equal(t, hexutil.Uint64(blockTime), *result.BlockTimestamp)
	})

	t.Run("nil for pending tx", func(t *testing.T) {
		result, err := NewRPCTransaction(msg, common.Hash{}, 0, 0, 0, nil, testChainID)
		require.NoError(t, err)
		require.Nil(t, result.BlockTimestamp)
	})

	t.Run("nil for zero block time", func(t *testing.T) {
		result, err := NewRPCTransaction(msg, blockHash, 10, 0, 0, nil, testChainID)
		require.NoError(t, err)
		require.Nil(t, result.BlockTimestamp, "zero blockTime must not produce a bogus timestamp")
	})
}
