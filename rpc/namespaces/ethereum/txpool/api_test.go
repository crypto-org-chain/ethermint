package txpool

import (
	"math/big"
	"testing"

	"cosmossdk.io/log/v2"
	protov2 "google.golang.org/protobuf/proto"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/appmempool"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

var (
	testChainID = big.NewInt(9000)
	testTo      = common.HexToAddress("0x1234567890123456789012345678901234567890")
)

// clientFunc adapts a plain pending-txs function to appmempool.MempoolClient
// for tests; InsertTx is unused here.
type clientFunc func() []*evmtypes.MsgEthereumTx

// singleMsgTx wraps a sdk.Msg as a minimal sdk.Tx for test purposes.
type singleMsgTx struct{ msg sdk.Msg }

func (t singleMsgTx) GetMsgs() []sdk.Msg                          { return []sdk.Msg{t.msg} }
func (singleMsgTx) GetMsgsV2() ([]protov2.Message, error)         { return nil, nil }
func (singleMsgTx) ValidateBasic() error                           { return nil }

func (f clientFunc) PendingTxs() []sdk.Tx {
	ethMsgs := f()
	out := make([]sdk.Tx, len(ethMsgs))
	for i, m := range ethMsgs {
		out[i] = singleMsgTx{m}
	}
	return out
}
func (clientFunc) InsertTx([]byte) (*sdk.TxResponse, error) { return nil, nil }

// newAPI builds a PublicAPI over a pending-txs func; a nil func maps to a nil
// interface so the empty-pool path is exercised faithfully.
func newAPI(reader func() []*evmtypes.MsgEthereumTx) *PublicAPI {
	clientCtx := client.Context{}.WithChainID("ethermint_9000-1")
	var c appmempool.MempoolClient
	if reader != nil {
		c = clientFunc(reader)
	}
	return NewPublicAPI(log.NewNopLogger(), clientCtx, c)
}

// newSender returns a fresh signer and its address.
func newSender(t *testing.T) (keyring.Signer, common.Address) {
	t.Helper()
	privKey, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	return tests.NewSigner(privKey), common.BytesToAddress(privKey.PubKey().Address().Bytes())
}

func signedTx(t *testing.T, signer keyring.Signer, from common.Address, nonce uint64) *evmtypes.MsgEthereumTx {
	t.Helper()
	tx := evmtypes.NewTx(testChainID, nonce, &testTo, big.NewInt(1000), 21000, big.NewInt(1000000000), nil, nil, nil, nil)
	tx.From = from.Bytes()
	require.NoError(t, tx.Sign(ethtypes.LatestSignerForChainID(testChainID), signer))
	return tx
}

func signedEIP1559Tx(t *testing.T, signer keyring.Signer, from common.Address, nonce uint64) *evmtypes.MsgEthereumTx {
	t.Helper()
	gasTipCap := big.NewInt(1_000_000_000)
	gasFeeCap := big.NewInt(2_000_000_000)
	tx := evmtypes.NewTx(testChainID, nonce, &testTo, big.NewInt(1000), 21000, nil, gasFeeCap, gasTipCap, nil, nil)
	tx.From = from.Bytes()
	require.NoError(t, tx.Sign(ethtypes.LatestSignerForChainID(testChainID), signer))
	return tx
}

func TestContentFrom(t *testing.T) {
	signerA, addrA := newSender(t)
	signerB, addrB := newSender(t)
	reader := func() []*evmtypes.MsgEthereumTx {
		return []*evmtypes.MsgEthereumTx{
			signedTx(t, signerA, addrA, 0),
			signedTx(t, signerA, addrA, 1),
			signedTx(t, signerB, addrB, 0),
		}
	}
	api := newAPI(reader)

	res, err := api.ContentFrom(addrA)
	require.NoError(t, err)
	require.Len(t, res["pending"], 2)
	require.Empty(t, res["queued"])
	require.Equal(t, addrA, res["pending"]["0"].From)
	require.Equal(t, hexutil.Uint64(1), res["pending"]["1"].Nonce)

	res, err = api.ContentFrom(addrB)
	require.NoError(t, err)
	require.Len(t, res["pending"], 1)
	require.Equal(t, addrB, res["pending"]["0"].From)

	// Unknown sender yields empty (non-nil) pools.
	res, err = api.ContentFrom(common.HexToAddress("0xdead"))
	require.NoError(t, err)
	require.Empty(t, res["pending"])
	require.NotNil(t, res["pending"])
}

func TestContent(t *testing.T) {
	signerA, addrA := newSender(t)
	signerB, addrB := newSender(t)
	api := newAPI(func() []*evmtypes.MsgEthereumTx {
		return []*evmtypes.MsgEthereumTx{
			signedTx(t, signerA, addrA, 0),
			signedTx(t, signerA, addrA, 1),
			signedTx(t, signerB, addrB, 7),
		}
	})

	content, err := api.Content()
	require.NoError(t, err)
	require.Len(t, content["pending"], 2) // two senders
	require.Empty(t, content["queued"])
	require.Len(t, content["pending"][addrA.Hex()], 2)
	require.Contains(t, content["pending"][addrB.Hex()], "7")
}

func TestStatus(t *testing.T) {
	signer, addr := newSender(t)
	api := newAPI(func() []*evmtypes.MsgEthereumTx {
		return []*evmtypes.MsgEthereumTx{
			signedTx(t, signer, addr, 0),
			signedTx(t, signer, addr, 1),
		}
	})

	status := api.Status()
	require.Equal(t, hexutil.Uint(2), status["pending"])
	require.Equal(t, hexutil.Uint(0), status["queued"])
}

// Status counts unique (sender, nonce) slots, matching Content which keys by
// nonce. Two txs sharing a nonce (a replacement) count once.
func TestStatusDedupesNonce(t *testing.T) {
	signer, addr := newSender(t)
	api := newAPI(func() []*evmtypes.MsgEthereumTx {
		return []*evmtypes.MsgEthereumTx{
			signedTx(t, signer, addr, 0),
			signedTx(t, signer, addr, 0), // same nonce, replacement
		}
	})

	require.Equal(t, hexutil.Uint(1), api.Status()["pending"])

	content, err := api.Content()
	require.NoError(t, err)
	require.Len(t, content["pending"][addr.Hex()], 1) // Status matches Content
}

func TestInspect(t *testing.T) {
	signer, addr := newSender(t)
	api := newAPI(func() []*evmtypes.MsgEthereumTx {
		return []*evmtypes.MsgEthereumTx{signedTx(t, signer, addr, 0)}
	})

	inspect, err := api.Inspect()
	require.NoError(t, err)
	require.Empty(t, inspect["queued"])
	require.Contains(t, inspect["pending"][addr.Hex()]["0"], testTo.Hex())
}

// A nil reader (app without mempool access) reports empty pools, not errors.
func TestNilReader(t *testing.T) {
	api := newAPI(nil)

	content, err := api.Content()
	require.NoError(t, err)
	require.Empty(t, content["pending"])

	from, err := api.ContentFrom(testTo)
	require.NoError(t, err)
	require.Empty(t, from["pending"])

	require.Equal(t, hexutil.Uint(0), api.Status()["pending"])
}

// TestInspectEIP1559 confirms Inspect formats EIP-1559 txs correctly.
// For unmined txs (no base fee), GasPrice is set to GasFeeCap by NewRPCTransaction.
func TestInspectEIP1559(t *testing.T) {
	signer, addr := newSender(t)
	api := newAPI(func() []*evmtypes.MsgEthereumTx {
		return []*evmtypes.MsgEthereumTx{signedEIP1559Tx(t, signer, addr, 0)}
	})

	inspect, err := api.Inspect()
	require.NoError(t, err)
	s := inspect["pending"][addr.Hex()]["0"]
	require.Contains(t, s, testTo.Hex())
	require.Contains(t, s, "2000000000 wei")
}

// TestSummaryNilGasPrice confirms summary falls back to GasFeeCap when GasPrice
// is nil, preventing "<nil>" in txpool_inspect output.
func TestSummaryNilGasPrice(t *testing.T) {
	gasFeeCap := big.NewInt(2_000_000_000)
	tx := &rpctypes.RPCTransaction{
		To:        &testTo,
		Value:     (*hexutil.Big)(big.NewInt(1000)),
		Gas:       21000,
		GasFeeCap: (*hexutil.Big)(gasFeeCap),
		GasPrice:  nil,
	}
	s := InspectFormat(tx)
	require.Contains(t, s, testTo.Hex())
	require.Contains(t, s, gasFeeCap.String()+" wei")
	require.NotContains(t, s, "<nil>")
}
