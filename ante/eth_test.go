package ante_test

import (
	"context"
	"math"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/ante/cache"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

type msgsTx []sdk.Msg

func (t msgsTx) GetMsgs() []sdk.Msg                { return t }
func (msgsTx) GetMsgsV2() ([]proto.Message, error) { return nil, nil }

// noopAccountKeeper only implements SetAccount; the account getter holds the state.
type noopAccountKeeper struct{ evmtypes.AccountKeeper }

func (noopAccountKeeper) SetAccount(context.Context, sdk.AccountI) {}

func ethMsg(from []byte, nonce uint64) *evmtypes.MsgEthereumTx {
	msg := evmtypes.NewTx(nil, nonce, nil, nil, 21000, nil, nil, nil, nil, nil)
	msg.From = from
	return msg
}

func TestCheckEthBatch(t *testing.T) {
	a, b := tests.GenerateAddress().Bytes(), tests.GenerateAddress().Bytes()
	for _, tc := range []struct {
		name   string
		tx     msgsTx
		expErr error
	}{
		{name: "single msg", tx: msgsTx{ethMsg(a, 5)}},
		{name: "same sender, consecutive nonces", tx: msgsTx{ethMsg(a, 5), ethMsg(a, 6), ethMsg(a, 7)}},
		{name: "non-eth msg", tx: msgsTx{ethMsg(a, 5), &banktypes.MsgSend{}}, expErr: errortypes.ErrUnknownRequest},
		{name: "second sender", tx: msgsTx{ethMsg(a, 5), ethMsg(b, 0)}, expErr: errortypes.ErrInvalidRequest},
		{name: "nonce gap", tx: msgsTx{ethMsg(a, 5), ethMsg(a, 7)}, expErr: errortypes.ErrInvalidSequence},
		{name: "repeated nonce", tx: msgsTx{ethMsg(a, 5), ethMsg(a, 5)}, expErr: errortypes.ErrInvalidSequence},
		{name: "nonces out of order", tx: msgsTx{ethMsg(a, 6), ethMsg(a, 5)}, expErr: errortypes.ErrInvalidSequence},
		{name: "nonce wraps past max", tx: msgsTx{ethMsg(a, math.MaxUint64), ethMsg(a, 0)}, expErr: errortypes.ErrInvalidSequence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ante.CheckEthBatch(tc.tx)
			if tc.expErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.expErr)
		})
	}
}

func TestCheckAndSetEthSenderNonceCache(t *testing.T) {
	for _, tc := range []struct {
		name       string
		checkTx    bool
		seq        uint64
		cached     []uint64
		nonces     []uint64
		expErr     error
		expPending int
		expCached  map[uint64]bool
	}{
		{
			name:       "CheckTx: single-msg tx is staged for the cache",
			checkTx:    true,
			nonces:     []uint64{0},
			expPending: 1,
		},
		{
			name:      "CheckTx: single-msg cache hit skips the nonce check",
			checkTx:   true,
			seq:       1,
			cached:    []uint64{0},
			nonces:    []uint64{0},
			expCached: map[uint64]bool{0: true},
		},
		{
			name:      "CheckTx: multi-msg tx ignores the cache",
			checkTx:   true,
			seq:       1,
			cached:    []uint64{0},
			nonces:    []uint64{0, 1},
			expErr:    errortypes.ErrInvalidSequence,
			expCached: map[uint64]bool{0: true},
		},
		{
			name:    "CheckTx: multi-msg tx isn't staged for the cache",
			checkTx: true,
			nonces:  []uint64{0, 1},
		},
		{
			name:      "execution clears every msg's entry",
			cached:    []uint64{0, 1},
			nonces:    []uint64{0, 1},
			expCached: map[uint64]bool{0: false, 1: false},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr := sdk.AccAddress(tests.GenerateAddress().Bytes())
			acc := authtypes.NewBaseAccountWithAddress(addr)
			require.NoError(t, acc.SetSequence(tc.seq))
			getter := func(sdk.AccAddress) sdk.AccountI { return acc }

			anteCache := cache.NewAnteCache(0)
			for _, n := range tc.cached {
				anteCache.Set(addr.String(), n)
			}
			tx := make(msgsTx, 0, len(tc.nonces))
			for _, n := range tc.nonces {
				tx = append(tx, ethMsg(addr, n))
			}

			ctx := sdk.Context{}.WithIsCheckTx(tc.checkTx)
			pending, err := ante.CheckAndSetEthSenderNonce(ctx, tx, noopAccountKeeper{}, false, getter, anteCache)
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, pending, tc.expPending)
			for n, cached := range tc.expCached {
				require.Equal(t, cached, anteCache.Exists(addr.String(), n), "nonce %d", n)
			}
		})
	}
}
