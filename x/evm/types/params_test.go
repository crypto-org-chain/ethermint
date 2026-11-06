package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateEIPsRejectsAmsterdamEIPs(t *testing.T) {
	for _, eip := range []int64{7843, 8024, 8037, 8038} {
		require.ErrorContains(t, validateEIPs([]int64{eip}), "Amsterdam is not supported", "EIP %d", eip)
	}
	require.NoError(t, validateEIPs([]int64{3855, 1153}))
	require.Error(t, validateEIPs([]int64{1}))
}
