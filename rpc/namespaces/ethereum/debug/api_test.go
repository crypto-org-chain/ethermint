package debug

import (
	"errors"
	"testing"

	"cosmossdk.io/log/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/rpc/backend"
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
