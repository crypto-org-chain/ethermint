package eth

import (
	"errors"
	"math/big"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"github.com/evmos/ethermint/rpc/backend"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

type testBackend struct {
	backend.EVMBackend
	baseFee *big.Int
	err     error
}

func (b testBackend) NextBaseFee() (*big.Int, error) {
	return b.baseFee, b.err
}

func TestPublicAPIBaseFee(t *testing.T) {
	baseFee := big.NewInt(123)
	baseFeeErr := errors.New("base fee error")

	testCases := []struct {
		name    string
		backend testBackend
		expBase *big.Int
		expErr  error
	}{
		{
			name: "returns backend error",
			backend: testBackend{
				err: baseFeeErr,
			},
			expErr: baseFeeErr,
		},
		{
			name: "returns zero when next base fee is nil",
			backend: testBackend{
				baseFee: nil,
			},
			expBase: new(big.Int),
		},
		{
			name: "returns next block base fee",
			backend: testBackend{
				baseFee: baseFee,
			},
			expBase: baseFee,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api := NewPublicAPI(log.NewNopLogger(), tc.backend)

			res, err := api.BaseFee()
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
				require.Nil(t, res)
				return
			}

			require.NoError(t, err)
			require.Equal(t, (*hexutil.Big)(tc.expBase), res)
		})
	}
}

func TestBlockNrOrHashOrLatest(t *testing.T) {
	t.Run("nil defaults to latest", func(t *testing.T) {
		result := blockNrOrHashOrLatest(nil)
		require.NotNil(t, result.BlockNumber)
		require.Equal(t, rpctypes.EthLatestBlockNumber, *result.BlockNumber)
		require.Nil(t, result.BlockHash)
	})

	t.Run("non-nil passes through unchanged", func(t *testing.T) {
		bn := rpctypes.EthEarliestBlockNumber
		input := &rpctypes.BlockNumberOrHash{BlockNumber: &bn}
		result := blockNrOrHashOrLatest(input)
		require.NotNil(t, result.BlockNumber)
		require.Equal(t, rpctypes.EthEarliestBlockNumber, *result.BlockNumber)
	})

	t.Run("explicit latest passes through", func(t *testing.T) {
		bn := rpctypes.EthLatestBlockNumber
		input := &rpctypes.BlockNumberOrHash{BlockNumber: &bn}
		result := blockNrOrHashOrLatest(input)
		require.NotNil(t, result.BlockNumber)
		require.Equal(t, rpctypes.EthLatestBlockNumber, *result.BlockNumber)
	})

	t.Run("block hash passes through", func(t *testing.T) {
		hash := common.HexToHash("0x579917054e325746fda5c3ee431d73d26255bc4e10b51163862368629ae19739")
		input := &rpctypes.BlockNumberOrHash{BlockHash: &hash}
		result := blockNrOrHashOrLatest(input)
		require.NotNil(t, result.BlockHash)
		require.Equal(t, hash, *result.BlockHash)
		require.Nil(t, result.BlockNumber)
	})
}

type txLogsBackend struct {
	backend.EVMBackend
	blockRes     *tmrpctypes.ResultBlockResults
	blockTime    uint64
	blockTimeErr error
}

func (b txLogsBackend) GetTxByEthHash(common.Hash) (*ethermint.TxResult, error) {
	return &ethermint.TxResult{Height: b.blockRes.Height}, nil
}

func (b txLogsBackend) TendermintBlockResultByNumber(*int64) (*tmrpctypes.ResultBlockResults, error) {
	return b.blockRes, nil
}

func (b txLogsBackend) BlockTimestamp(int64) (uint64, error) {
	return b.blockTime, b.blockTimeErr
}

func TestGetTransactionLogs(t *testing.T) {
	const blockTime = uint64(1_700_000_000)
	headerErr := errors.New("header not found")
	oneLog := []*evmtypes.Log{{Address: common.HexToAddress("0x1").Hex()}}
	// An event log with no attributes decodes to zero logs.
	noLogEvents := []abci.Event{{Type: evmtypes.EventTypeTxLog}}

	testCases := []struct {
		name         string
		logs         []*evmtypes.Log
		events       []abci.Event
		blockTimeErr error
		expLogs      int
		expErr       error
	}{
		{"logs carry the block timestamp", oneLog, nil, nil, 1, nil},
		{"header error is returned, not hidden as empty logs", oneLog, nil, headerErr, 0, headerErr},
		// The header error proves the lookup is skipped when there is nothing to stamp.
		{"tx without logs skips the header read", nil, noLogEvents, headerErr, 0, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rsp, err := codectypes.NewAnyWithValue(&evmtypes.MsgEthereumTxResponse{Logs: tc.logs})
			require.NoError(t, err)
			data, err := proto.Marshal(&sdk.TxMsgData{MsgResponses: []*codectypes.Any{rsp}})
			require.NoError(t, err)
			api := NewPublicAPI(log.NewNopLogger(), txLogsBackend{
				blockRes: &tmrpctypes.ResultBlockResults{
					Height:     5,
					TxsResults: []*abci.ExecTxResult{{Data: data, Events: tc.events}},
				},
				blockTime:    blockTime,
				blockTimeErr: tc.blockTimeErr,
			})

			logs, err := api.GetTransactionLogs(common.Hash{})
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, logs, tc.expLogs)
			for _, l := range logs {
				require.Equal(t, uint64(5), l.BlockNumber)
				require.Equal(t, blockTime, l.BlockTimestamp)
			}
		})
	}
}
