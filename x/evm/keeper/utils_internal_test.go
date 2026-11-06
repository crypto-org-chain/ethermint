package keeper

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestDebugTraceFeeAmountOverflow(t *testing.T) {
	maxU256 := new(uint256.Int).SetAllOne()
	msg := &core.Message{GasLimit: 2, GasPrice: maxU256}

	_, err := debugTraceFeeAmount(msg, nil)
	require.ErrorIs(t, err, core.ErrInsufficientFunds)

	msg.GasLimit = 1
	fee, err := debugTraceFeeAmount(msg, nil)
	require.NoError(t, err)
	require.Equal(t, maxU256, fee)

	msg = &core.Message{GasLimit: 3, GasPrice: uint256.NewInt(5)}
	fee, err = debugTraceFeeAmount(msg, big.NewInt(1))
	require.NoError(t, err)
	require.Equal(t, uint256.NewInt(15), fee)
}
