package backend

import (
	"time"

	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	ethrpc "github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/mock"
)

func (suite *BackendTestSuite) TestGetLogs() {
	_, bz := suite.buildEthereumTx()
	block := tmtypes.MakeBlock(1, []tmtypes.Tx{bz}, nil, nil)
	logs := make([]*evmtypes.Log, 0, 1)
	logs = append(logs, &evmtypes.Log{
		Data:        []byte("data"),
		BlockNumber: 1,
	})

	testCases := []struct {
		name         string
		registerMock func(hash common.Hash)
		blockHash    common.Hash
		expLogs      [][]*ethtypes.Log
		expPass      bool
	}{
		{
			"fail - no block with that hash",
			func(hash common.Hash) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockByHashNotFound(client, hash, bz)
			},
			common.Hash{},
			nil,
			false,
		},
		{
			"fail - error fetching block by hash",
			func(hash common.Hash) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockByHashError(client, hash, bz)
			},
			common.Hash{},
			nil,
			false,
		},
		{
			"fail - error getting block results",
			func(hash common.Hash) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockByHash(client, hash, bz)
				RegisterBlockResultsError(client, 1)
			},
			common.Hash{},
			nil,
			false,
		},
		{
			"success - getting logs with block hash",
			func(hash common.Hash) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockByHash(client, hash, bz)
				RegisterBlockResultsWithEventLog(client, ethrpc.BlockNumber(1).Int64())
			},
			common.BytesToHash(block.Hash()),
			[][]*ethtypes.Log{evmtypes.LogsToEthereum(logs)},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()

			tc.registerMock(tc.blockHash)
			logs, err := suite.backend.GetLogs(tc.blockHash)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expLogs, logs)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetLogsBlockTimestamp() {
	const height = int64(1)
	blockTime := time.Unix(1700000000, 0)
	block := &tmtypes.Block{Header: tmtypes.Header{Height: height, Time: blockTime}}
	hash := common.BytesToHash(block.Hash())

	suite.Run("GetLogs", func() {
		suite.SetupTest()
		client := suite.backend.clientCtx.Client.(*mocks.Client)
		client.On("BlockByHash", ethrpc.ContextWithHeight(1), hash.Bytes()).
			Return(&tmrpctypes.ResultBlock{Block: block}, nil)
		_, err := RegisterBlockResultsWithEventLog(client, height)
		suite.Require().NoError(err)

		logs, err := suite.backend.GetLogs(hash)
		suite.Require().NoError(err)
		suite.Require().Len(logs, 1)
		suite.Require().Len(logs[0], 1)
		suite.Require().Equal(uint64(blockTime.Unix()), logs[0][0].BlockTimestamp)
	})

	suite.Run("GetLogsByHeight", func() {
		suite.SetupTest()
		client := suite.backend.clientCtx.Client.(*mocks.Client)
		_, err := RegisterBlockResultsWithEventLog(client, height)
		suite.Require().NoError(err)
		client.On("Header", ethrpc.ContextWithHeight(height), mock.AnythingOfType("*int64")).
			Return(&tmrpctypes.ResultHeader{Header: &block.Header}, nil)

		h := height
		logs, err := suite.backend.GetLogsByHeight(&h)
		suite.Require().NoError(err)
		suite.Require().Len(logs, 1)
		suite.Require().Len(logs[0], 1)
		suite.Require().Equal(uint64(blockTime.Unix()), logs[0][0].BlockTimestamp)
	})

	suite.Run("GetLogsByHeight without logs skips the header", func() {
		suite.SetupTest()
		client := suite.backend.clientCtx.Client.(*mocks.Client)
		_, err := RegisterBlockResults(client, height)
		suite.Require().NoError(err)

		h := height
		logs, err := suite.backend.GetLogsByHeight(&h)
		suite.Require().NoError(err)
		suite.Require().Len(logs, 1)
		suite.Require().Empty(logs[0])
		client.AssertNotCalled(suite.T(), "Header", mock.Anything, mock.Anything)
	})
}

func (suite *BackendTestSuite) TestBlockTimeByHeight() {
	blockTime := time.Unix(1700000000, 0)

	testCases := []struct {
		name         string
		height       int64
		registerMock func(height int64)
		expTime      uint64
	}{
		{
			"pass - block time of the header",
			1,
			func(height int64) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				client.On("Header", ethrpc.ContextWithHeight(height), &height).
					Return(&tmrpctypes.ResultHeader{Header: &tmtypes.Header{Height: height, Time: blockTime}}, nil)
			},
			uint64(blockTime.Unix()),
		},
		{
			"pass - zero when the header can't be fetched",
			1,
			func(height int64) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterHeaderError(client, &height)
			},
			0,
		},
		{
			"pass - zero when the header is not found",
			1,
			func(height int64) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterHeaderNotFound(client, height)
			},
			0,
		},
		{
			"pass - zero for a non-positive height, rather than the latest block time",
			0,
			func(int64) {},
			0,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()

			tc.registerMock(tc.height)
			suite.Require().Equal(tc.expTime, suite.backend.BlockTimeByHeight(tc.height))
		})
	}
}

func (suite *BackendTestSuite) TestBlockTimesByRange() {
	blockTime := func(height int64) time.Time { return time.Unix(1700000000+height, 0) }

	testCases := []struct {
		name         string
		minHeight    int64
		maxHeight    int64
		registerMock func()
		expTimes     map[int64]uint64
	}{
		{
			"pass - block times of the headers",
			1,
			2,
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				client.On("BlockchainInfo", mock.Anything, int64(1), int64(2)).
					Return(&tmrpctypes.ResultBlockchainInfo{BlockMetas: []*tmtypes.BlockMeta{
						{Header: tmtypes.Header{Height: 2, Time: blockTime(2)}},
						{Header: tmtypes.Header{Height: 1, Time: blockTime(1)}},
					}}, nil)
			},
			map[int64]uint64{1: uint64(blockTime(1).Unix()), 2: uint64(blockTime(2).Unix())},
		},
		{
			"pass - empty when the headers can't be fetched",
			1,
			2,
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				client.On("BlockchainInfo", mock.Anything, int64(1), int64(2)).
					Return(nil, errortypes.ErrInvalidRequest)
			},
			nil,
		},
		{
			"pass - empty for a range of non-positive heights",
			-1,
			0,
			func() {},
			nil,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()

			tc.registerMock()
			suite.Require().Equal(tc.expTimes, suite.backend.BlockTimesByRange(tc.minHeight, tc.maxHeight))
		})
	}
}

func (suite *BackendTestSuite) TestBloomStatus() {
	testCases := []struct {
		name         string
		registerMock func()
		expResult    uint64
		expPass      bool
	}{
		{
			"pass - returns the BloomBitsBlocks and the number of processed sections maintained",
			func() {},
			4096,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()

			tc.registerMock()
			bloom, _ := suite.backend.BloomStatus()

			if tc.expPass {
				suite.Require().Equal(tc.expResult, bloom)
			}
		})
	}
}
