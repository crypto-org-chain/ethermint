package app

import (
	"context"
	"io"

	"cosmossdk.io/store/cachemulti"
	storetypes "cosmossdk.io/store/types"
	abci "github.com/cometbft/cometbft/abci/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/cosmos/cosmos-sdk/baseapp"

	block_stm "github.com/yihuang/go-block-stm"
)

func DefaultTxExecutor(_ context.Context,
	blockSize int,
	ms storetypes.MultiStore,
	deliverTxWithMultiStore func(int, storetypes.MultiStore) *abci.ExecTxResult,
) ([]*abci.ExecTxResult, error) {
	results := make([]*abci.ExecTxResult, blockSize)
	for i := 0; i < blockSize; i++ {
		results[i] = deliverTxWithMultiStore(i, ms)
	}
	return evmtypes.PatchTxResponses(results), nil
}

func STMTxExecutor(stores []storetypes.StoreKey, workers int) baseapp.TxExecutor {
	index := make(map[storetypes.StoreKey]int, len(stores))
	for i, k := range stores {
		index[k] = i
	}
	return func(
		ctx context.Context,
		blockSize int,
		ms storetypes.MultiStore,
		deliverTxWithMultiStore func(int, storetypes.MultiStore) *abci.ExecTxResult,
	) ([]*abci.ExecTxResult, error) {
		if blockSize == 0 {
			return nil, nil
		}
		results := make([]*abci.ExecTxResult, blockSize)
		if err := block_stm.ExecuteBlock(
			ctx,
			blockSize,
			index,
			stmMultiStoreWrapper{ms},
			workers,
			func(txn block_stm.TxnIndex, ms block_stm.MultiStore) {
				result := deliverTxWithMultiStore(int(txn), newMultiStoreWrapper(ms, stores))
				results[txn] = result
			},
		); err != nil {
			return nil, err
		}

		return evmtypes.PatchTxResponses(results), nil
	}
}

type msWrapper struct {
	block_stm.MultiStore
	stores     []storetypes.StoreKey
	keysByName map[string]storetypes.StoreKey
}

var _ storetypes.MultiStore = msWrapper{}

func newMultiStoreWrapper(ms block_stm.MultiStore, stores []storetypes.StoreKey) msWrapper {
	keysByName := make(map[string]storetypes.StoreKey, len(stores))
	for _, k := range stores {
		keysByName[k.Name()] = k
	}
	return msWrapper{ms, stores, keysByName}
}

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

func (ms msWrapper) CacheMultiStoreWithVersion(_ int64) (storetypes.CacheMultiStore, error) {
	panic("cannot branch cached multi-store with a version")
}

// Implements CacheWrapper.
func (ms msWrapper) CacheWrap() storetypes.CacheWrap {
	return ms.CacheMultiStore().(storetypes.CacheWrap)
}

// CacheWrapWithTrace implements the CacheWrapper interface.
func (ms msWrapper) CacheWrapWithTrace(_ io.Writer, _ storetypes.TraceContext) storetypes.CacheWrap {
	return ms.CacheWrap()
}

// GetStoreType returns the type of the store.
func (ms msWrapper) GetStoreType() storetypes.StoreType {
	return storetypes.StoreTypeMulti
}

// LatestVersion returns the branch version of the store
func (ms msWrapper) LatestVersion() int64 {
	panic("cannot get latest version from branch cached multi-store")
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
	inner storetypes.MultiStore
}

var _ block_stm.MultiStore = stmMultiStoreWrapper{}

func (ms stmMultiStoreWrapper) GetStore(key storetypes.StoreKey) storetypes.Store {
	return ms.inner.GetStore(key)
}

func (ms stmMultiStoreWrapper) GetKVStore(key storetypes.StoreKey) storetypes.KVStore {
	return ms.inner.GetKVStore(key)
}

func (ms stmMultiStoreWrapper) GetObjKVStore(key storetypes.StoreKey) storetypes.ObjKVStore {
	return ms.inner.GetObjKVStore(key)
}
