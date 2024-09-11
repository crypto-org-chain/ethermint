package app

import (
	"context"
	"io"
	"sync/atomic"

	"cosmossdk.io/collections"
	"cosmossdk.io/store/cachemulti"
	storetypes "cosmossdk.io/store/types"
	abci "github.com/cometbft/cometbft/abci/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"

	blockstm "github.com/crypto-org-chain/go-block-stm"
)

var EVMDenomForEstimation = evmtypes.DefaultEVMDenom

func DefaultTxExecutor(_ context.Context,
	txs []sdk.Tx,
	ms storetypes.MultiStore,
	deliverTxWithMultiStore func(int, storetypes.MultiStore, map[string]any) *abci.ExecTxResult,
) ([]*abci.ExecTxResult, error) {
	blockSize := len(txs)
	results := make([]*abci.ExecTxResult, blockSize)
	for i := 0; i < blockSize; i++ {
		results[i] = deliverTxWithMultiStore(i, ms, nil)
	}
	return evmtypes.PatchTxResponses(results), nil
}

type evmKeeper interface {
	GetParams(ctx sdk.Context) evmtypes.Params
}

func STMTxExecutor(stores []storetypes.StoreKey, workers int, evmKeeper evmKeeper) baseapp.TxExecutor {
	var authStore, bankStore int
	index := make(map[storetypes.StoreKey]int, len(stores))
	for i, k := range stores {
		if k.Name() == authtypes.StoreKey {
			authStore = i
		} else if k.Name() == banktypes.StoreKey {
			bankStore = i
		}
		index[k] = i
	}
	return func(
		ctx context.Context,
		txs []sdk.Tx,
		ms storetypes.MultiStore,
		deliverTxWithMultiStore func(int, storetypes.MultiStore, map[string]any) *abci.ExecTxResult,
	) ([]*abci.ExecTxResult, error) {
		blockSize := len(txs)
		if len(txs) == 0 {
			return nil, nil
		}
		results := make([]*abci.ExecTxResult, blockSize)
		incarnationCache := make([]atomic.Pointer[map[string]any], blockSize)
		for i := 0; i < blockSize; i++ {
			m := make(map[string]any)
			incarnationCache[i].Store(&m)
		}

		// pre-estimation
		evmDenom := evmKeeper.GetParams(sdk.UnwrapSDKContext(ctx)).EvmDenom
		estimates := preEstimates(txs, authStore, bankStore, evmDenom)

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

				result := deliverTxWithMultiStore(int(txn), msWrapper{ms}, cache)
				results[txn] = result

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
func preEstimates(txs []sdk.Tx, authStore, bankStore int, evmDenom string) map[int]map[int][]blockstm.Key {
	estimates := make(map[int]map[int][]blockstm.Key, len(txs))
	for i, tx := range txs {
		feeTx, ok := tx.(sdk.FeeTx)
		if !ok {
			continue
		}
		feePayer := sdk.AccAddress(feeTx.FeePayer())

		// account key
		accKey, err := collections.EncodeKeyWithPrefix(authtypes.AddressStoreKeyPrefix, sdk.AccAddressKey, feePayer)
		if err != nil {
			continue
		}

		// balance key
		balanceKey, err := collections.EncodeKeyWithPrefix(banktypes.BalancesPrefix, collections.PairKeyCodec(sdk.AccAddressKey, collections.StringKey), collections.Join(feePayer, evmDenom))
		if err != nil {
			continue
		}

		estimates[i] = map[int][]blockstm.Key{
			authStore: {accKey},
			bankStore: {balanceKey},
		}
	}

	return estimates
}
