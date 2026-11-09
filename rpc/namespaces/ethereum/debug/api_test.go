package debug

import (
	"errors"
	"testing"

	"cosmossdk.io/log/v2"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/rpc/backend"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

type testBackend struct {
	backend.EVMBackend
	rawTx hexutil.Bytes
	err   error
	hash  common.Hash
}

func (b *testBackend) GetRawTransactionByHash(txHash common.Hash) (hexutil.Bytes, error) {
	b.hash = txHash
	return b.rawTx, b.err
}

var errTraced = errors.New("traced")

func (b *testBackend) TraceTransaction(common.Hash, *rpctypes.TraceConfig) (interface{}, error) {
	return nil, errTraced
}

func (b *testBackend) TraceCall(evmtypes.TransactionArgs, rpctypes.BlockNumberOrHash, *rpctypes.TraceConfig) (interface{}, error) {
	return nil, errTraced
}

func (b *testBackend) TendermintBlockByNumber(rpctypes.BlockNumber) (*tmrpctypes.ResultBlock, error) {
	return nil, errTraced
}

func (b *testBackend) TendermintBlockByHash(common.Hash) (*tmrpctypes.ResultBlock, error) {
	return nil, errTraced
}

func TestNewAPITraceSlots(t *testing.T) {
	testCases := []struct {
		name                string
		maxConcurrentTraces int
		expCap              int
	}{
		{"no cap by default", 0, 0},
		{"cap from config", 3, 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := server.NewDefaultContext()
			ctx.Viper.Set("json-rpc.max-concurrent-traces", tc.maxConcurrentTraces)

			api := NewAPI(ctx, &testBackend{})
			require.Equal(t, tc.expCap, cap(api.traceSlots))
		})
	}
}

func TestAPITraceConcurrencyCap(t *testing.T) {
	traceMethods := []struct {
		name  string
		trace func(api *API) error
	}{
		{"debug_traceTransaction", func(api *API) error {
			_, err := api.TraceTransaction(common.Hash{}, nil)
			return err
		}},
		{"debug_traceBlockByNumber", func(api *API) error {
			_, err := api.TraceBlockByNumber(1, nil)
			return err
		}},
		{"debug_traceBlockByHash", func(api *API) error {
			_, err := api.TraceBlockByHash(common.Hash{}, nil)
			return err
		}},
		{"debug_traceCall", func(api *API) error {
			_, err := api.TraceCall(evmtypes.TransactionArgs{}, rpctypes.BlockNumberOrHash{}, nil)
			return err
		}},
	}
	testCases := []struct {
		name       string
		traceSlots chan struct{}
		busySlots  int
		expErr     error
	}{
		{"no cap", nil, 0, errTraced},
		{"free slot", make(chan struct{}, 2), 1, errTraced},
		{"all slots busy", make(chan struct{}, 2), 2, errTooManyTraces},
	}

	for _, method := range traceMethods {
		for _, tc := range testCases {
			t.Run(method.name+", "+tc.name, func(t *testing.T) {
				api := &API{
					logger:     log.NewNopLogger(),
					backend:    &testBackend{},
					handler:    new(HandlerT),
					traceSlots: tc.traceSlots,
				}
				for i := 0; i < tc.busySlots; i++ {
					api.traceSlots <- struct{}{}
				}

				require.ErrorIs(t, method.trace(api), tc.expErr)
				// a finished or rejected call leaves the busy slots as they were
				require.Len(t, api.traceSlots, tc.busySlots)
				for i := 0; i < tc.busySlots; i++ {
					<-api.traceSlots
				}
			})
		}
	}
}

func TestAPIGetRawTransaction(t *testing.T) {
	txHash := common.HexToHash("0x1234")
	rawTx := hexutil.Bytes{0x01, 0x02, 0x03}
	rawTxErr := errors.New("raw tx error")

	testCases := []struct {
		name   string
		rawTx  hexutil.Bytes
		err    error
		expErr error
	}{
		{
			name:  "returns backend raw transaction bytes",
			rawTx: rawTx,
		},
		{
			name:   "returns backend error",
			err:    rawTxErr,
			expErr: rawTxErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &testBackend{
				rawTx: tc.rawTx,
				err:   tc.err,
			}
			api := &API{
				logger:  log.NewNopLogger(),
				backend: backend,
				handler: new(HandlerT),
			}

			res, err := api.GetRawTransaction(txHash)

			require.Equal(t, txHash, backend.hash)
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
				require.Nil(t, res)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.rawTx, res)
		})
	}
}
