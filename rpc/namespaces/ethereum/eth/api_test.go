package eth

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"github.com/evmos/ethermint/rpc/backend"
	rpctypes "github.com/evmos/ethermint/rpc/types"
)

type testBackend struct {
	backend.EVMBackend
	baseFee *big.Int
	err     error
}

func (b testBackend) NextBaseFee() (*big.Int, error) {
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

func TestBlockNrOrHashOrLatest(t *testing.T) {
	t.Run("nil defaults to latest", func(t *testing.T) {
		result := blockNrOrHashOrLatest(nil)
		require.NotNil(t, result.BlockNumber)
		require.Equal(t, rpctypes.EthLatestBlockNumber, *result.BlockNumber)
		require.Nil(t, result.BlockHash)
	})

	t.Run("non-nil passes through unchanged", func(t *testing.T) {
		bn := rpctypes.EthEarliestBlockNumber
		input := &rpctypes.BlockNumberOrHash{BlockNumber: &bn}
		result := blockNrOrHashOrLatest(input)
		require.NotNil(t, result.BlockNumber)
		require.Equal(t, rpctypes.EthEarliestBlockNumber, *result.BlockNumber)
	})

	t.Run("explicit latest passes through", func(t *testing.T) {
		bn := rpctypes.EthLatestBlockNumber
		input := &rpctypes.BlockNumberOrHash{BlockNumber: &bn}
		result := blockNrOrHashOrLatest(input)
		require.NotNil(t, result.BlockNumber)
		require.Equal(t, rpctypes.EthLatestBlockNumber, *result.BlockNumber)
	})

	t.Run("block hash passes through", func(t *testing.T) {
		hash := common.HexToHash("0x579917054e325746fda5c3ee431d73d26255bc4e10b51163862368629ae19739")
		input := &rpctypes.BlockNumberOrHash{BlockHash: &hash}
		result := blockNrOrHashOrLatest(input)
		require.NotNil(t, result.BlockHash)
		require.Equal(t, hash, *result.BlockHash)
		require.Nil(t, result.BlockNumber)
	})
}
