package app

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	"cosmossdk.io/store/cachemulti"
	storetypes "cosmossdk.io/store/types"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"

	blockstm "github.com/crypto-org-chain/go-block-stm"
)

const MinimalParallelPreEstimate = 32

func DefaultTxExecutor(_ context.Context,
	txs [][]byte,
	ms storetypes.MultiStore,
	deliverTxWithMultiStore func(int, sdk.Tx, storetypes.MultiStore, map[string]any) *abci.ExecTxResult,
) ([]*abci.ExecTxResult, error) {
	blockSize := len(txs)
	results := make([]*abci.ExecTxResult, blockSize)
	for i := 0; i < blockSize; i++ {
		results[i] = deliverTxWithMultiStore(i, nil, ms, nil)
	}
	return evmtypes.PatchTxResponses(results), nil
}

type evmKeeper interface {
	GetParams(ctx sdk.Context) evmtypes.Params
}

func STMTxExecutor(
	stores []storetypes.StoreKey,
	workers int,
	estimate bool,
	evmKeeper evmKeeper,
	txDecoder sdk.TxDecoder,
) baseapp.TxExecutor {
	var authStore, bankStore int
	index := make(map[storetypes.StoreKey]int, len(stores))
	for i, k := range stores {
		switch k.Name() {
		case authtypes.StoreKey:
			authStore = i
		case banktypes.StoreKey:
			bankStore = i
		}
		index[k] = i
	}
	return func(
		ctx context.Context,
		txs [][]byte,
		ms storetypes.MultiStore,
		deliverTxWithMultiStore func(int, sdk.Tx, storetypes.MultiStore, map[string]any) *abci.ExecTxResult,
	) ([]*abci.ExecTxResult, error) {
		blockSize := len(txs)
		if blockSize == 0 {
			return nil, nil
		}
		results := make([]*abci.ExecTxResult, blockSize)
		incarnationCache := make([]atomic.Pointer[map[string]any], blockSize)
		for i := 0; i < blockSize; i++ {
			m := make(map[string]any)
			incarnationCache[i].Store(&m)
		}

		var (
			estimates map[int]blockstm.MultiLocations
			memTxs    []sdk.Tx
		)
		if estimate {
			// pre-estimation
			evmDenom := evmKeeper.GetParams(sdk.NewContext(ms, cmtproto.Header{}, false, log.NewNopLogger())).EvmDenom
			memTxs, estimates = preEstimates(txs, workers, authStore, bankStore, evmDenom)
		}

		if err := blockstm.ExecuteBlockWithEstimates(
			ctx,
			blockSize,
			index,
			stmMultiStoreWrapper{ms},
			workers,
			estimates,
			func(txn blockstm.TxnIndex, ms blockstm.MultiStore) {
				var cache map[string]any

				// only one of the concurrent incarnations gets the cache if there are any, otherwise execute without
				// cache, concurrent incarnations should be rare.
				v := incarnationCache[txn].Swap(nil)
				if v != nil {
					cache = *v
				}

				results[txn] = deliverTxWithMultiStore(int(txn), memTxs[txn], msWrapper{ms}, cache)

				if v != nil {
					incarnationCache[txn].Store(v)
				}
			},
		); err != nil {
			return nil, err
		}

		return evmtypes.PatchTxResponses(results), nil
	}
}

type msWrapper struct {
	blockstm.MultiStore
}

var _ storetypes.MultiStore = msWrapper{}

func (ms msWrapper) getCacheWrapper(key storetypes.StoreKey) storetypes.CacheWrapper {
	return ms.GetStore(key)
}

func (ms msWrapper) GetStore(key storetypes.StoreKey) storetypes.Store {
	return ms.MultiStore.GetStore(key)
}

func (ms msWrapper) GetKVStore(key storetypes.StoreKey) storetypes.KVStore {
	return ms.MultiStore.GetKVStore(key)
}

func (ms msWrapper) GetObjKVStore(key storetypes.StoreKey) storetypes.ObjKVStore {
	return ms.MultiStore.GetObjKVStore(key)
}

func (ms msWrapper) CacheMultiStore() storetypes.CacheMultiStore {
	return cachemulti.NewFromParent(ms.getCacheWrapper, nil, nil)
}

// Implements CacheWrapper.
func (ms msWrapper) CacheWrap() storetypes.CacheWrap {
	return ms.CacheMultiStore().(storetypes.CacheWrap)
}

// GetStoreType returns the type of the store.
func (ms msWrapper) GetStoreType() storetypes.StoreType {
	return storetypes.StoreTypeMulti
}

// Implements interface MultiStore
func (ms msWrapper) SetTracer(io.Writer) storetypes.MultiStore {
	return nil
}

// Implements interface MultiStore
func (ms msWrapper) SetTracingContext(storetypes.TraceContext) storetypes.MultiStore {
	return nil
}

// Implements interface MultiStore
func (ms msWrapper) TracingEnabled() bool {
	return false
}

type stmMultiStoreWrapper struct {
	storetypes.MultiStore
}

var _ blockstm.MultiStore = stmMultiStoreWrapper{}

func (ms stmMultiStoreWrapper) GetStore(key storetypes.StoreKey) storetypes.Store {
	return ms.MultiStore.GetStore(key)
}

func (ms stmMultiStoreWrapper) GetKVStore(key storetypes.StoreKey) storetypes.KVStore {
	return ms.MultiStore.GetKVStore(key)
}

func (ms stmMultiStoreWrapper) GetObjKVStore(key storetypes.StoreKey) storetypes.ObjKVStore {
	return ms.MultiStore.GetObjKVStore(key)
}

// preEstimates returns a static estimation of the written keys for each transaction.
// NOTE: make sure it sync with the latest sdk logic when sdk upgrade.
func preEstimates(txs [][]byte, workers, authStore, bankStore int, evmDenom string, txDecoder sdk.TxDecoder) ([]sdk.Tx, map[int]blockstm.MultiLocations) {
	memTxs := make([]sdk.Tx, len(txs))
	estimates := make(map[int]blockstm.MultiLocations, len(txs))

	job := func(start, end int) {
		for i := start; i < end; i++ {
			rawTx := txs[i]
			tx, err := txDecoder(rawTx)
			if err != nil {
				continue
			}
			memTxs[i] = tx

			feeTx, ok := tx.(sdk.FeeTx)
			if !ok {
				continue
			}
			feePayer := sdk.AccAddress(feeTx.FeePayer())

			// account key
			accKey, err := collections.EncodeKeyWithPrefix(
				authtypes.AddressStoreKeyPrefix,
				sdk.AccAddressKey,
				feePayer,
			)
			if err != nil {
				continue
			}

			// balance key
			balanceKey, err := collections.EncodeKeyWithPrefix(
				banktypes.BalancesPrefix,
				collections.PairKeyCodec(sdk.AccAddressKey, collections.StringKey),
				collections.Join(feePayer, evmDenom),
			)
			if err != nil {
				continue
			}

			estimates[i] = blockstm.MultiLocations{
				authStore: {accKey},
				bankStore: {balanceKey},
			}
		}
	}

	blockSize := len(txs)
	chunk := blockSize / workers
	if blockSize < MinimalParallelPreEstimate || chunk == 0 {
		job(0, blockSize)
	} else {
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			start := i * chunk
			end := (i + 1) * chunk
			if i == workers-1 {
				end = blockSize
			}
			go func() {
				defer wg.Done()
				job(start, end)
			}()
		}
		wg.Wait()
	}
	return memTxs, estimates
}
