package evmd

import (
	"context"
	"fmt"
	"math/big"
	"runtime"
	"testing"

	"cosmossdk.io/log"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/rootmulti"
	storetypes "cosmossdk.io/store/types"
	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"

	evmante "github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/evmd/ante"
	testutilconfig "github.com/evmos/ethermint/testutil/config"
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func BenchmarkPreEstimatesWithSigVerify(b *testing.B) {
	b.StopTimer()

	encCfg := testutilconfig.MakeConfigForTest(nil)
	txConfig := encCfg.TxConfig
	txDecoder := txConfig.TxDecoder()

	chainID := big.NewInt(1)
	signer := ethtypes.LatestSignerForChainID(chainID)

	privKey, err := ethsecp256k1.GenerateKey()
	if err != nil {
		b.Fatal(err)
	}
	from := common.BytesToAddress(privKey.PubKey().Address())
	keyringSigner := newBenchmarkSigner(privKey)

	const txCount = 256
	txs := make([][]byte, txCount)
	for i := 0; i < txCount; i++ {
		msg := evmtypes.NewTxContract(
			chainID,
			uint64(i),
			big.NewInt(0),
			params.TxGasContractCreation,
			big.NewInt(1),
			big.NewInt(1),
			big.NewInt(1),
			[]byte("contract_data"),
			nil,
		)
		msg.From = from.Bytes()
		err := msg.Sign(signer, keyringSigner)
		if err != nil {
			b.Fatal(err)
		}
		txBuilder := txConfig.NewTxBuilder()
		if err := txBuilder.SetMsgs(msg); err != nil {
			b.Fatal(err)
		}
		txBz, err := txConfig.TxEncoder()(txBuilder.GetTx())
		if err != nil {
			b.Fatal(err)
		}
		txs[i] = txBz
	}

	workers := runtime.GOMAXPROCS(0)
	b.ReportAllocs()
	b.StartTimer()

	b.Run("baseline_no_sigverify", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			preEstimatesWithSigVerify(txs, workers, 0, 1, evmtypes.DefaultEVMDenom, txDecoder, nil)
		}
	})

	b.Run("preverify_sig", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			preEstimatesWithSigVerify(txs, workers, 0, 1, evmtypes.DefaultEVMDenom, txDecoder, signer)
		}
	})
}

func BenchmarkSTMTxExecutorEndToEnd(b *testing.B) {
	benchmarkSTMTxExecutor(b, 256, true, true)
}

func BenchmarkSTMTxExecutor_SmallBlock(b *testing.B) {
	benchmarkSTMTxExecutor(b, 10, false, false)
}

func BenchmarkSTMTxExecutor_MediumBlock(b *testing.B) {
	benchmarkSTMTxExecutor(b, 100, false, false)
}

func BenchmarkSTMTxExecutor_LargeBlock(b *testing.B) {
	benchmarkSTMTxExecutor(b, 5000, false, false)
}

func BenchmarkSTMTxExecutor_WithEstimate(b *testing.B) {
	benchmarkSTMTxExecutor(b, 256, true, false)
}

func BenchmarkSTMTxExecutor_MemoryAlloc(b *testing.B) {
	benchmarkSTMTxExecutor(b, 256, true, true)
}

func benchmarkSTMTxExecutor(b *testing.B, txCount int, estimate bool, reportAllocs bool) {
	b.StopTimer()

	encCfg := testutilconfig.MakeConfigForTest(nil)
	txConfig := encCfg.TxConfig
	txDecoder := txConfig.TxDecoder()

	chainID := big.NewInt(1)
	signer := ethtypes.LatestSignerForChainID(chainID)

	privKey, err := ethsecp256k1.GenerateKey()
	if err != nil {
		b.Fatal(err)
	}
	from := common.BytesToAddress(privKey.PubKey().Address())
	keyringSigner := newBenchmarkSigner(privKey)

	txs := make([][]byte, txCount)
	for i := 0; i < txCount; i++ {
		msg := evmtypes.NewTxContract(
			chainID,
			uint64(i),
			big.NewInt(0),
			params.TxGasContractCreation,
			big.NewInt(1),
			big.NewInt(1),
			big.NewInt(1),
			[]byte("contract_data"),
			nil,
		)
		msg.From = from.Bytes()
		err := msg.Sign(signer, keyringSigner)
		if err != nil {
			b.Fatal(err)
		}
		txBuilder := txConfig.NewTxBuilder()
		if err := txBuilder.SetMsgs(msg); err != nil {
			b.Fatal(err)
		}
		txBz, err := txConfig.TxEncoder()(txBuilder.GetTx())
		if err != nil {
			b.Fatal(err)
		}
		txs[i] = txBz
	}

	authKey := storetypes.NewKVStoreKey("auth")
	bankKey := storetypes.NewKVStoreKey("bank")
	storeKeys := []storetypes.StoreKey{authKey, bankKey}

	db := dbm.NewMemDB()
	cms := rootmulti.NewStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	for _, key := range storeKeys {
		cms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, nil)
	}
	if err := cms.LoadLatestVersion(); err != nil {
		b.Fatal(err)
	}

	mockKeeper := benchmarkEvmKeeper{
		params:  evmtypes.DefaultParams(),
		chainID: chainID,
	}
	workers := runtime.GOMAXPROCS(0)

	executor := STMTxExecutor(storeKeys, workers, estimate, mockKeeper, txDecoder)
	deliver := func(_ int, tx sdk.Tx, _ storetypes.MultiStore, cache map[string]any) *abci.ExecTxResult {
		if tx == nil {
			return &abci.ExecTxResult{}
		}
		if cache != nil {
			if v, ok := cache[ante.EthSigVerificationResultCacheKey]; ok {
				if err, ok := v.(error); ok && err != nil {
					return &abci.ExecTxResult{Code: 1, Log: err.Error()}
				}
				return &abci.ExecTxResult{}
			}
		}
		if err := evmante.VerifyEthSig(tx, signer); err != nil {
			return &abci.ExecTxResult{Code: 1, Log: err.Error()}
		}
		return &abci.ExecTxResult{}
	}

	if reportAllocs {
		b.ReportAllocs()
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if _, err := executor(context.Background(), txs, cms, deliver); err != nil {
			b.Fatal(err)
		}
	}
}

type benchmarkSigner struct {
	privKey cryptotypes.PrivKey
}

var _ keyring.Signer = benchmarkSigner{}

func newBenchmarkSigner(sk cryptotypes.PrivKey) keyring.Signer {
	return benchmarkSigner{privKey: sk}
}

func (s benchmarkSigner) Sign(_ string, msg []byte, _ signing.SignMode) ([]byte, cryptotypes.PubKey, error) {
	if s.privKey.Type() != ethsecp256k1.KeyType {
		return nil, nil, fmt.Errorf(
			"invalid private key type for signing ethereum tx; expected %s, got %s",
			ethsecp256k1.KeyType,
			s.privKey.Type(),
		)
	}
	sig, err := s.privKey.Sign(msg)
	if err != nil {
		return nil, nil, err
	}
	return sig, s.privKey.PubKey(), nil
}

func (s benchmarkSigner) SignByAddress(address sdk.Address, msg []byte, signMode signing.SignMode) ([]byte, cryptotypes.PubKey, error) {
	signer := sdk.AccAddress(s.privKey.PubKey().Address())
	if !signer.Equals(address) {
		return nil, nil, fmt.Errorf("address mismatch: signer %s ≠ given address %s", signer, address)
	}
	return s.Sign("", msg, signMode)
}

type benchmarkEvmKeeper struct {
	params  evmtypes.Params
	chainID *big.Int
}

func (k benchmarkEvmKeeper) GetParams(_ sdk.Context) evmtypes.Params {
	return k.params
}

func (k benchmarkEvmKeeper) ChainID() *big.Int {
	return k.chainID
}

func (k benchmarkEvmKeeper) EVMBlockConfig(ctx sdk.Context, chainID *big.Int) (*evmkeeper.EVMBlockConfig, error) {
	ethCfg := k.params.ChainConfig.EthereumConfig(chainID)
	blockNumber := big.NewInt(ctx.BlockHeight())
	var blockTime uint64
	if !ctx.BlockHeader().Time.IsZero() {
		blockTime = uint64(ctx.BlockHeader().Time.Unix())
	}
	rules := ethCfg.Rules(blockNumber, ethCfg.MergeNetsplitBlock != nil, blockTime)
	return &evmkeeper.EVMBlockConfig{
		Params:      k.params,
		ChainConfig: ethCfg,
		BlockNumber: blockNumber,
		BlockTime:   blockTime,
		Rules:       rules,
	}, nil
}
