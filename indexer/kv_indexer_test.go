package indexer_test

import (
	"math/big"
	"testing"

	tmlog "cosmossdk.io/log"
	abci "github.com/cometbft/cometbft/abci/types"
	tmtypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/indexer"
	"github.com/evmos/ethermint/tests"
	"github.com/evmos/ethermint/testutil/config"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/require"
)

func TestKVIndexer(t *testing.T) {
	priv, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	from := common.BytesToAddress(priv.PubKey().Address().Bytes())
	signer := tests.NewSigner(priv)
	ethSigner := ethtypes.LatestSignerForChainID(nil)

	to := common.BigToAddress(big.NewInt(1))
	tx := types.NewTx(
		nil, 0, &to, big.NewInt(1000), 21000, nil, nil, nil, nil, nil,
	)
	tx.From = from.Bytes()
	require.NoError(t, tx.Sign(ethSigner, signer))
	txHash := tx.AsTransaction().Hash()

	encodingConfig := config.MakeConfigForTest(nil)
	clientCtx := client.Context{}.WithTxConfig(encodingConfig.TxConfig).WithCodec(encodingConfig.Codec)

	// build cosmos-sdk wrapper tx
	tmTx, err := tx.BuildTx(clientCtx.TxConfig.NewTxBuilder(), "aphoton")
	require.NoError(t, err)
	txBz, err := clientCtx.TxConfig.TxEncoder()(tmTx)
	require.NoError(t, err)

	// build an invalid wrapper tx
	builder := clientCtx.TxConfig.NewTxBuilder()
	require.NoError(t, builder.SetMsgs(tx))
	tmTx2 := builder.GetTx()
	txBz2, err := clientCtx.TxConfig.TxEncoder()(tmTx2)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		block       *tmtypes.Block
		blockResult []*abci.ExecTxResult
		expSuccess  bool
	}{
		{
			"success, format 1",
			&tmtypes.Block{Header: tmtypes.Header{Height: 1}, Data: tmtypes.Data{Txs: []tmtypes.Tx{txBz}}},
			[]*abci.ExecTxResult{
				{
					Code: 0,
					Events: []abci.Event{
						{Type: types.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash.Hex()},
							{Key: "txIndex", Value: "0"},
							{Key: "amount", Value: "1000"},
							{Key: "txGasUsed", Value: "21000"},
							{Key: "txHash", Value: ""},
							{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
						}},
					},
				},
			},
			true,
		},
		{
			"success, format 2",
			&tmtypes.Block{Header: tmtypes.Header{Height: 1}, Data: tmtypes.Data{Txs: []tmtypes.Tx{txBz}}},
			[]*abci.ExecTxResult{
				{
					Code: 0,
					Events: []abci.Event{
						{Type: types.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash.Hex()},
							{Key: "txIndex", Value: "0"},
						}},
						{Type: types.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash.Hex()},
							{Key: "amount", Value: "1000"},
							{Key: "txGasUsed", Value: "21000"},
							{Key: "txHash", Value: "14A84ED06282645EFBF080E0B7ED80D8D8D6A36337668A12B5F229F81CDD3F57"},
							{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
						}},
					},
				},
			},
			true,
		},
		{
			"success, exceed block gas limit",
			&tmtypes.Block{Header: tmtypes.Header{Height: 1}, Data: tmtypes.Data{Txs: []tmtypes.Tx{txBz}}},
			[]*abci.ExecTxResult{
				{
					Code:   11,
					Log:    "out of gas in location: block gas meter; gasWanted: 21000",
					Events: []abci.Event{},
				},
			},
			true,
		},
		{
			"fail, failed eth tx",
			&tmtypes.Block{Header: tmtypes.Header{Height: 1}, Data: tmtypes.Data{Txs: []tmtypes.Tx{txBz}}},
			[]*abci.ExecTxResult{
				{
					Code:   15,
					Log:    "nonce mismatch",
					Events: []abci.Event{},
				},
			},
			false,
		},
		{
			"fail, invalid events",
			&tmtypes.Block{Header: tmtypes.Header{Height: 1}, Data: tmtypes.Data{Txs: []tmtypes.Tx{txBz}}},
			[]*abci.ExecTxResult{
				{
					Code:   0,
					Events: []abci.Event{},
				},
			},
			false,
		},
		{
			"fail, not eth tx",
			&tmtypes.Block{Header: tmtypes.Header{Height: 1}, Data: tmtypes.Data{Txs: []tmtypes.Tx{txBz2}}},
			[]*abci.ExecTxResult{
				{
					Code:   0,
					Events: []abci.Event{},
				},
			},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := dbm.NewMemDB()
			idxer := indexer.NewKVIndexer(db, tmlog.NewNopLogger(), clientCtx)

			err = idxer.IndexBlock(tc.block, tc.blockResult)
			require.NoError(t, err)
			if !tc.expSuccess {
				first, err := idxer.FirstIndexedBlock()
				require.NoError(t, err)
				require.Equal(t, int64(-1), first)

				last, err := idxer.LastIndexedBlock()
				require.NoError(t, err)
				require.Equal(t, int64(-1), last)
			} else {
				first, err := idxer.FirstIndexedBlock()
				require.NoError(t, err)
				require.Equal(t, tc.block.Header.Height, first)

				last, err := idxer.LastIndexedBlock()
				require.NoError(t, err)
				require.Equal(t, tc.block.Header.Height, last)

				res1, err := idxer.GetByTxHash(txHash)
				require.NoError(t, err)
				require.NotNil(t, res1)
				res2, err := idxer.GetByBlockAndIndex(1, 0)
				require.NoError(t, err)
				require.Equal(t, res1, res2)
			}
		})
	}
}

// TestKVIndexer_BlockWideCumulativeGas verifies that CumulativeGasUsed is
// accumulated across all eth txs in the block, matching the eth semantics
// used by eth_getBlockReceipts.
func TestKVIndexer_BlockWideCumulativeGas(t *testing.T) {
	priv, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	from := common.BytesToAddress(priv.PubKey().Address().Bytes())
	signer := tests.NewSigner(priv)
	ethSigner := ethtypes.LatestSignerForChainID(nil)
	to := common.BigToAddress(big.NewInt(1))

	encodingConfig := config.MakeConfigForTest(nil)
	clientCtx := client.Context{}.WithTxConfig(encodingConfig.TxConfig).WithCodec(encodingConfig.Codec)

	buildTx := func(nonce uint64) (common.Hash, tmtypes.Tx) {
		tx := types.NewTx(nil, nonce, &to, big.NewInt(1000), 21000, nil, nil, nil, nil, nil)
		tx.From = from.Bytes()
		require.NoError(t, tx.Sign(ethSigner, signer))
		txHash := tx.AsTransaction().Hash()
		tmTx, err := tx.BuildTx(clientCtx.TxConfig.NewTxBuilder(), "aphoton")
		require.NoError(t, err)
		bz, err := clientCtx.TxConfig.TxEncoder()(tmTx)
		require.NoError(t, err)
		return txHash, bz
	}

	hash0, tx0 := buildTx(0)
	hash1, tx1 := buildTx(1)

	ethEvent := func(h common.Hash, idx string) abci.Event {
		return abci.Event{Type: types.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
			{Key: "ethereumTxHash", Value: h.Hex()},
			{Key: "txIndex", Value: idx},
			{Key: "amount", Value: "1000"},
			{Key: "txGasUsed", Value: "21000"},
			{Key: "txHash", Value: ""},
			{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
		}}
	}

	block := &tmtypes.Block{
		Header: tmtypes.Header{Height: 1},
		Data:   tmtypes.Data{Txs: []tmtypes.Tx{tx0, tx1}},
	}
	results := []*abci.ExecTxResult{
		{Code: 0, GasUsed: 21000, Events: []abci.Event{ethEvent(hash0, "0")}},
		{Code: 0, GasUsed: 21000, Events: []abci.Event{ethEvent(hash1, "1")}},
	}

	db := dbm.NewMemDB()
	idxer := indexer.NewKVIndexer(db, tmlog.NewNopLogger(), clientCtx)
	require.NoError(t, idxer.IndexBlock(block, results))

	r0, err := idxer.GetByTxHash(hash0)
	require.NoError(t, err)
	require.Equal(t, uint64(21000), r0.CumulativeGasUsed)

	r1, err := idxer.GetByTxHash(hash1)
	require.NoError(t, err)
	require.Equal(t, uint64(42000), r1.CumulativeGasUsed, "second eth tx must accumulate block-wide cumulative")
}
