package evmd

import (
	"context"
	"io"
	"math/big"
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
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	evmante "github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/evmd/ante"
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"

	blockstm "github.com/crypto-org-chain/go-block-stm"
)

const MinimalParallelPreEstimate = 16

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
	ChainID() *big.Int
	EVMBlockConfig(sdk.Context, *big.Int) (*evmkeeper.EVMBlockConfig, error)
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
		incarnationCache := initIncarnationCache(blockSize)

		var estimates []blockstm.MultiLocations
		var memTxs []sdk.Tx
		if estimate {
			memTxs, estimates = preEstimateAndCacheSigResults(
				ms,
				txs,
				workers,
				authStore,
				bankStore,
				evmKeeper,
				txDecoder,
				incarnationCache,
			)
		}

		if err := blockstm.ExecuteBlockWithEstimates(
			ctx,
			blockSize,
			index,
			stmMultiStoreWrapper{ms},
			workers,
			estimates,
			func(txn blockstm.TxnIndex, ms blockstm.MultiStore) {
				cachePtr := incarnationCache[txn].Swap(nil)
				cache := map[string]any(nil)
				if cachePtr != nil {
					cache = *cachePtr
				}

				var memTx sdk.Tx
				if memTxs != nil {
					memTx = memTxs[txn]
				}
				results[txn] = deliverTxWithMultiStore(int(txn), memTx, msWrapper{ms}, cache)

				if cachePtr != nil {
					incarnationCache[txn].Store(cachePtr)
				}
			},
		); err != nil {
			return nil, err
		}

		return evmtypes.PatchTxResponses(results), nil
	}
}

func initIncarnationCache(blockSize int) []atomic.Pointer[map[string]any] {
	incarnationCache := make([]atomic.Pointer[map[string]any], blockSize)
	for i := 0; i < blockSize; i++ {
		m := make(map[string]any)
		incarnationCache[i].Store(&m)
	}
	return incarnationCache
}

func preEstimateAndCacheSigResults(
	ms storetypes.MultiStore,
	txs [][]byte,
	workers, authStore, bankStore int,
	evmKeeper evmKeeper,
	txDecoder sdk.TxDecoder,
	incarnationCache []atomic.Pointer[map[string]any],
) ([]sdk.Tx, []blockstm.MultiLocations) {
	sdkCtx := sdk.NewContext(ms, cmtproto.Header{}, false, log.NewNopLogger())
	evmParams := evmKeeper.GetParams(sdkCtx)
	evmDenom := evmParams.EvmDenom

	var ethSigner ethtypes.Signer
	if blockCfg, err := evmKeeper.EVMBlockConfig(sdkCtx, evmKeeper.ChainID()); err == nil {
		ethSigner = ethtypes.MakeSigner(blockCfg.ChainConfig, blockCfg.BlockNumber, blockCfg.BlockTime)
	}

	memTxs, estimates, sigVerResults := preEstimatesWithSigVerify(
		txs,
		workers,
		authStore,
		bankStore,
		evmDenom,
		txDecoder,
		ethSigner,
	)

	for i, result := range sigVerResults {
		if memTxs[i] == nil {
			continue
		}
		cache := incarnationCache[i].Load()
		if cache == nil {
			continue
		}
		(*cache)[ante.EthSigVerificationResultCacheKey] = result
	}

	return memTxs, estimates
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

func (ms msWrapper) CacheWrapWithTrace(_ io.Writer, _ storetypes.TraceContext) storetypes.CacheWrap {
	return ms.CacheWrap()
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

// preEstimatesWithSigVerify returns a static estimation of the written keys for each transaction,
// and optionally pre-verifies Ethereum signatures in parallel to avoid redundant crypto.Ecrecover calls.
// The signature verification results are returned to be stored in the incarnation cache.
func preEstimatesWithSigVerify(
	txs [][]byte,
	workers, authStore, bankStore int,
	evmDenom string,
	txDecoder sdk.TxDecoder,
	ethSigner ethtypes.Signer,
) ([]sdk.Tx, []blockstm.MultiLocations, []error) {
	memTxs := make([]sdk.Tx, len(txs))
	estimates := make([]blockstm.MultiLocations, len(txs))
	sigVerResults := make([]error, len(txs))

	job := func(start, end int) {
		for i := start; i < end; i++ {
			rawTx := txs[i]
			tx, err := txDecoder(rawTx)
			if err != nil {
				continue
			}
			memTxs[i] = tx

			// Pre-verify Ethereum signatures in parallel (expensive crypto.Ecrecover).
			// Only run for MsgEthereumTx to avoid extra work on cosmos txs.
			if ethSigner != nil {
				if _, ok := tx.(*evmtypes.MsgEthereumTx); ok {
					sigVerResults[i] = evmante.VerifyEthSig(tx, ethSigner)
				}
			}

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
	chunk := (blockSize + workers - 1) / workers
	var wg sync.WaitGroup
	for i := 0; i < blockSize; i += chunk {
		start := i
		end := min(i+chunk, blockSize)
		wg.Add(1)
		go func() {
			defer wg.Done()
			job(start, end)
		}()
	}
	wg.Wait()

	return memTxs, estimates, sigVerResults
}
