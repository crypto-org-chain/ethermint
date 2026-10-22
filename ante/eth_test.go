package ante_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

type msgsTx []sdk.Msg

func (t msgsTx) GetMsgs() []sdk.Msg                { return t }
func (msgsTx) GetMsgsV2() ([]proto.Message, error) { return nil, nil }

func TestCheckEthSingleSender(t *testing.T) {
	a, b := tests.GenerateAddress(), tests.GenerateAddress()
	msgFrom := func(from []byte) sdk.Msg { return &evmtypes.MsgEthereumTx{From: from} }

	for _, tc := range []struct {
		name   string
		tx     msgsTx
		expErr error
	}{
		{"single msg", msgsTx{msgFrom(a.Bytes())}, nil},
		{"same-sender batch", msgsTx{msgFrom(a.Bytes()), msgFrom(a.Bytes())}, nil},
		{"second sender", msgsTx{msgFrom(a.Bytes()), msgFrom(b.Bytes())}, errortypes.ErrInvalidRequest},
		{"second sender after a non-eth msg", msgsTx{msgFrom(a.Bytes()), &banktypes.MsgSend{}, msgFrom(b.Bytes())}, errortypes.ErrInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ante.CheckEthSingleSender(tc.tx)
			if tc.expErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.expErr)
		})
	}
}
