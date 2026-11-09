package debug

import (
	"errors"
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/rpc/backend"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	srvflags "github.com/evmos/ethermint/server/flags"
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
	return &tmrpctypes.ResultBlock{Block: &tmtypes.Block{}}, nil
}

func (b *testBackend) TendermintBlockByHash(common.Hash) (*tmrpctypes.ResultBlock, error) {
	return &tmrpctypes.ResultBlock{Block: &tmtypes.Block{}}, nil
}

func (b *testBackend) TraceBlock(rpctypes.BlockNumber, *rpctypes.TraceConfig, *tmrpctypes.ResultBlock) ([]*evmtypes.TxTraceResult, error) {
	return nil, errTraced
}

// blockingBackend holds every TraceTransaction call until release is closed.
type blockingBackend struct {
	testBackend
	started chan struct{}
	release chan struct{}
}

func (b *blockingBackend) TraceTransaction(common.Hash, *rpctypes.TraceConfig) (interface{}, error) {
	b.started <- struct{}{}
	<-b.release
	return nil, errTraced
}

func TestAPITraceHoldsSlotWhileRunning(t *testing.T) {
	backend := &blockingBackend{started: make(chan struct{}, 2), release: make(chan struct{})}
	api := &API{logger: log.NewNopLogger(), backend: backend, handler: new(HandlerT), traceSlots: make(chan struct{}, 1)}
	trace := func() <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := api.TraceTransaction(common.Hash{}, nil)
			done <- err
		}()
		return done
	}

	first := trace()
	<-backend.started
	select {
	case err := <-trace():
		require.ErrorIs(t, err, errTooManyTraces)
	case <-time.After(time.Second):
		close(backend.release)
		t.Fatal("a second trace ran while the only slot was busy")
	}

	close(backend.release)
	require.ErrorIs(t, <-first, errTraced)
	require.Empty(t, api.traceSlots)
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
			ctx.Viper.Set(srvflags.JSONRPCMaxConcurrentTraces, tc.maxConcurrentTraces)

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
		expCode    int
	}{
		{"no cap", nil, 0, errTraced, 0},
		{"free slot", make(chan struct{}, 2), 1, errTraced, 0},
		{"all slots busy", make(chan struct{}, 2), 2, errTooManyTraces, -32005},
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

				err := method.trace(api)
				require.ErrorIs(t, err, tc.expErr)
				if tc.expCode != 0 {
					var rpcErr interface{ ErrorCode() int }
					require.ErrorAs(t, err, &rpcErr)
					require.Equal(t, tc.expCode, rpcErr.ErrorCode())
				}
				// a finished or rejected call leaves the busy slots as they were
				require.Len(t, api.traceSlots, tc.busySlots)
				for i := 0; i < tc.busySlots; i++ {
					<-api.traceSlots
				}
			})
		}
	}

	t.Run("an invalid request is answered without a slot", func(t *testing.T) {
		api := &API{logger: log.NewNopLogger(), backend: &testBackend{}, handler: new(HandlerT), traceSlots: make(chan struct{}, 1)}
		api.traceSlots <- struct{}{}
		_, err := api.TraceBlockByNumber(0, nil)
		require.ErrorContains(t, err, "genesis is not traceable")
	})
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
