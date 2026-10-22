package backend

import (
	"time"

	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
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
	blockTime := time.Unix(1_700_000_000, 0)
	expLogs := evmtypes.LogsToEthereum(logs)
	expLogs[0].BlockTimestamp = uint64(blockTime.Unix())

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
				resBlock, _ := RegisterBlockByHash(client, hash, bz)
				resBlock.Block.Time = blockTime
				RegisterBlockResultsWithEventLog(client, ethrpc.BlockNumber(1).Int64())
			},
			common.BytesToHash(block.Hash()),
			[][]*ethtypes.Log{expLogs},
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

func (suite *BackendTestSuite) TestGetLogsByHeight() {
	height := int64(1)
	blockTime := time.Unix(1_700_000_000, 0)
	expLogs := evmtypes.LogsToEthereum([]*evmtypes.Log{{Data: []byte("data")}})
	expLogs[0].BlockNumber = 1
	expLogs[0].BlockTimestamp = uint64(blockTime.Unix())

	testCases := []struct {
		name         string
		registerMock func()
		expLogs      [][]*ethtypes.Log
		expPass      bool
	}{
		{
			"fail - error getting block results",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockResultsError(client, height)
			},
			nil,
			false,
		},
		{
			"fail - error getting header",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				_, err := RegisterBlockResultsWithEventLog(client, height)
				suite.Require().NoError(err)
				RegisterHeaderError(client, &height)
			},
			nil,
			false,
		},
		{
			"fail - header not found",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				_, err := RegisterBlockResultsWithEventLog(client, height)
				suite.Require().NoError(err)
				client.On("Header", ethrpc.ContextWithHeight(height), mock.AnythingOfType("*int64")).
					Return(&tmrpctypes.ResultHeader{}, nil)
			},
			nil,
			false,
		},
		{
			"success - logs carry the header time",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				_, err := RegisterBlockResultsWithEventLog(client, height)
				suite.Require().NoError(err)
				client.On("Header", ethrpc.ContextWithHeight(height), mock.AnythingOfType("*int64")).
					Return(&tmrpctypes.ResultHeader{Header: &tmtypes.Header{Height: height, Time: blockTime}}, nil)
			},
			[][]*ethtypes.Log{expLogs},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()

			tc.registerMock()
			logs, err := suite.backend.GetLogsByHeight(&height)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expLogs, logs)
			} else {
				suite.Require().Error(err)
			}
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
