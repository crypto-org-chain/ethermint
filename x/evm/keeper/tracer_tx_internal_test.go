package keeper

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestTxForTracer(t *testing.T) {
	to := common.Address{0xaa}
	auths := []ethtypes.SetCodeAuthorization{{Address: common.Address{0xbb}, Nonce: 1}}
	accessList := ethtypes.AccessList{{Address: to}}

	t.Run("set code tx carries authorizations and fees", func(t *testing.T) {
		tx := txForTracer(&core.Message{
			To:                    &to,
			Nonce:                 7,
			Value:                 uint256.NewInt(3),
			GasLimit:              100_000,
			GasPrice:              uint256.NewInt(10),
			GasFeeCap:             uint256.NewInt(10),
			GasTipCap:             uint256.NewInt(5),
			Data:                  []byte{0x01},
			AccessList:            accessList,
			SetCodeAuthorizations: auths,
		})
		require.Equal(t, uint8(ethtypes.SetCodeTxType), tx.Type())
		require.Equal(t, auths, tx.SetCodeAuthorizations())
		require.Equal(t, accessList, tx.AccessList())
		require.Equal(t, uint64(7), tx.Nonce())
		require.Equal(t, big.NewInt(3), tx.Value())
		tip, err := tx.EffectiveGasTip(big.NewInt(2))
		require.NoError(t, err)
		require.Equal(t, big.NewInt(5), tip)
	})

	t.Run("contract creation falls back to dynamic fee tx", func(t *testing.T) {
		tx := txForTracer(&core.Message{
			GasLimit:  100_000,
			GasFeeCap: uint256.NewInt(10),
			GasTipCap: uint256.NewInt(5),
		})
		require.Equal(t, uint8(ethtypes.DynamicFeeTxType), tx.Type())
		require.Nil(t, tx.To())
		require.Equal(t, big.NewInt(0), tx.Value())
		tip, err := tx.EffectiveGasTip(big.NewInt(2))
		require.NoError(t, err)
		require.Equal(t, big.NewInt(5), tip)
	})

	t.Run("nil fee caps fall back to gas price", func(t *testing.T) {
		tx := txForTracer(&core.Message{To: &to, GasLimit: 21_000, GasPrice: uint256.NewInt(4)})
		require.Equal(t, big.NewInt(4), tx.GasFeeCap())
		require.Equal(t, big.NewInt(4), tx.GasTipCap())
	})
}
