package eth

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"github.com/evmos/ethermint/rpc/backend"
)

type testBackend struct {
	backend.EVMBackend
	baseFee *big.Int
	err     error
}

func (b testBackend) BaseFeeForNextBlock() (*big.Int, error) {
	return b.baseFee, b.err
}

func TestPublicAPIBaseFee(t *testing.T) {
	baseFee := big.NewInt(123)
	baseFeeErr := errors.New("base fee error")

	testCases := []struct {
		name    string
		backend testBackend
		expBase *big.Int
		expErr  error
	}{
		{
			name: "returns backend error",
			backend: testBackend{
				err: baseFeeErr,
			},
			expErr: baseFeeErr,
		},
		{
			name: "returns zero when next base fee is nil",
			backend: testBackend{
				baseFee: nil,
			},
			expBase: new(big.Int),
		},
		{
			name: "returns next block base fee",
			backend: testBackend{
				baseFee: baseFee,
			},
			expBase: baseFee,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api := NewPublicAPI(log.NewNopLogger(), tc.backend)

			res, err := api.BaseFee()
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
				require.Nil(t, res)
				return
			}

			require.NoError(t, err)
			require.Equal(t, (*hexutil.Big)(tc.expBase), res)
		})
	}
}
