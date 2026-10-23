package ante_test

import (
	"context"
	"math/big"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
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

// noopAccountKeeper only implements SetAccount; the account getter holds the state.
type noopAccountKeeper struct{ evmtypes.AccountKeeper }

func (noopAccountKeeper) SetAccount(context.Context, sdk.AccountI) {}

func ethMsg(from []byte, nonce uint64) *evmtypes.MsgEthereumTx {
	msg := evmtypes.NewTx(nil, nonce, nil, nil, 21000, nil, nil, nil, nil, nil)
	msg.From = from
	return msg
}

// pooledMsg is an eth msg at the same sender and nonce as ethMsg, but a different tx.
func pooledMsg(from []byte, nonce uint64) *evmtypes.MsgEthereumTx {
	msg := evmtypes.NewTx(nil, nonce, nil, nil, 22000, nil, nil, nil, nil, nil)
	msg.From = from
	return msg
}

func TestCheckBatchTransaction(t *testing.T) {
	a, b := tests.GenerateAddress().Bytes(), tests.GenerateAddress().Bytes()
	for _, tc := range []struct {
		name   string
		tx     msgsTx
		expErr error
	}{
		{name: "single msg", tx: msgsTx{ethMsg(a, 5)}},
		{name: "same sender", tx: msgsTx{ethMsg(a, 5), ethMsg(a, 6)}},
		{name: "second sender", tx: msgsTx{ethMsg(a, 5), ethMsg(b, 0)}, expErr: errortypes.ErrInvalidRequest},
		{name: "second sender after a non-eth msg", tx: msgsTx{ethMsg(a, 5), &banktypes.MsgSend{}, ethMsg(b, 0)}, expErr: errortypes.ErrInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ante.CheckBatchTransaction(tc.tx)
			if tc.expErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.expErr)
		})
	}
}

type poolKey struct {
	sender string
	nonce  uint64
}

type fakePool map[poolKey]sdk.Tx

func (p fakePool) GetTx(sender string, nonce uint64) (sdk.Tx, bool) {
	tx, ok := p[poolKey{sender, nonce}]
	return tx, ok
}

func TestCheckAndSetEthSenderNonceTxPool(t *testing.T) {
	addr, other := sdk.AccAddress(tests.GenerateAddress().Bytes()), sdk.AccAddress(tests.GenerateAddress().Bytes())
	pooled := func(nonce uint64, tx msgsTx) fakePool {
		return fakePool{{addr.String(), nonce}: tx}
	}
	for _, tc := range []struct {
		name   string
		mode   sdk.ExecMode
		seq    uint64 // addr's sequence; other's is 0
		pool   ante.TxPool
		tx     msgsTx
		expErr error
		expSeq uint64
	}{
		{
			name:   "CheckTx: pooled nonce below the sequence is replaced",
			seq:    1,
			pool:   pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expSeq: 1,
		},
		{
			name:   "CheckTx: pooled nonce above a reset sequence is replaced",
			pool:   pooled(1, msgsTx{pooledMsg(addr, 1)}),
			tx:     msgsTx{ethMsg(addr, 1)},
		},
		{
			name: "CheckTx: pooled nonce equal to a reset sequence is replaced without incrementing it",
			pool: pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:   msgsTx{ethMsg(addr, 0)},
		},
		{
			name:   "CheckTx: resending the pooled tx itself reports it as already pooled",
			seq:    1,
			pool:   pooled(0, msgsTx{ethMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrTxInMempoolCache,
		},
		{
			name:   "Simulate: the pooled tx itself still simulates",
			mode:   sdk.ExecModeSimulate,
			seq:    1,
			pool:   pooled(0, msgsTx{ethMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expSeq: 1,
		},
		{
			name:   "CheckTx: matching unpooled nonce increments the sequence",
			pool:   fakePool{},
			tx:     msgsTx{ethMsg(addr, 0)},
			expSeq: 1,
		},
		{
			name:   "CheckTx: unpooled nonce is rejected",
			seq:    1,
			pool:   fakePool{},
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "CheckTx: nil pool disables replacement",
			seq:    1,
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "CheckTx: multi-msg tx doesn't replace",
			seq:    1,
			pool:   pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0), ethMsg(addr, 1)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "CheckTx: batch from another sender can't reuse a pooled nonce",
			seq:    1,
			pool:   pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(other, 0), ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "CheckTx: pooled multi-msg tx isn't replaced",
			seq:    2,
			pool:   pooled(0, msgsTx{ethMsg(addr, 0), ethMsg(addr, 1)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "CheckTx: pooled cosmos tx isn't replaced",
			seq:    1,
			pool:   pooled(0, msgsTx{&banktypes.MsgSend{}}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "ReCheckTx: pooled nonce is rejected",
			mode:   sdk.ExecModeReCheck,
			seq:    1,
			pool:   pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "Simulate: pooled nonce is replaced",
			mode:   sdk.ExecModeSimulate,
			seq:    1,
			pool:   pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expSeq: 1,
		},
		{
			name:   "PrepareProposal: pooled nonce is rejected",
			mode:   sdk.ExecModePrepareProposal,
			seq:    1,
			pool:   pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "ProcessProposal: pooled nonce is rejected",
			mode:   sdk.ExecModeProcessProposal,
			seq:    1,
			pool:   pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
		{
			name:   "FinalizeBlock: pooled nonce is rejected",
			mode:   sdk.ExecModeFinalize,
			seq:    1,
			pool:   pooled(0, msgsTx{pooledMsg(addr, 0)}),
			tx:     msgsTx{ethMsg(addr, 0)},
			expErr: errortypes.ErrInvalidSequence,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			acc := authtypes.NewBaseAccountWithAddress(addr)
			require.NoError(t, acc.SetSequence(tc.seq))
			otherAcc := authtypes.NewBaseAccountWithAddress(other)
			getter := func(a sdk.AccAddress) sdk.AccountI {
				if a.Equals(other) {
					return otherAcc
				}
				return acc
			}

			var ctx sdk.Context
			switch tc.mode {
			case sdk.ExecModeCheck:
				ctx = ctx.WithIsCheckTx(true)
			case sdk.ExecModeReCheck:
				ctx = ctx.WithIsReCheckTx(true)
			case sdk.ExecModeSimulate:
				ctx = ctx.WithIsCheckTx(true).WithExecMode(tc.mode)
			default:
				ctx = ctx.WithExecMode(tc.mode)
			}
			err := ante.CheckAndSetEthSenderNonce(ctx, tc.tx, noopAccountKeeper{}, false, getter, tc.pool)
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expSeq, acc.GetSequence())
		})
	}
}

func TestEthTxReplacement(t *testing.T) {
	dynamic := func(feeCap, tip int64) msgsTx {
		return msgsTx{evmtypes.NewTx(big.NewInt(1), 0, nil, nil, 21000, nil, big.NewInt(feeCap), big.NewInt(tip), nil, nil)}
	}
	legacy := func(gasPrice int64) msgsTx {
		return msgsTx{evmtypes.NewTx(big.NewInt(1), 0, nil, nil, 21000, big.NewInt(gasPrice), nil, nil, nil, nil)}
	}
	for _, tc := range []struct {
		name     string
		feeBump  int64
		oTx, nTx sdk.Tx
		exp      bool
	}{
		{"both caps bumped", 10, dynamic(1000, 100), dynamic(1100, 110), true},
		{"fee cap under the bump", 10, dynamic(1000, 100), dynamic(1099, 110), false},
		{"tip cap under the bump", 10, dynamic(1000, 100), dynamic(1100, 109), false},
		{"small tips are compared in wei", 10, dynamic(5e6, 5e6), dynamic(5.5e6, 5.5e6), true},
		{"fee cap must rise even when rounding meets the bump", 10, dynamic(5, 0), dynamic(5, 0), false},
		{"zero fee bump still needs a higher fee cap", 0, dynamic(1000, 100), dynamic(1000, 100), false},
		{"zero tip may stay zero", 10, dynamic(1000, 0), dynamic(1100, 0), true},
		{"tip whose bump rounds to zero may stay equal", 10, dynamic(5, 5), dynamic(6, 5), true},
		{"tip whose bump doesn't round to zero must rise", 10, dynamic(1000, 10), dynamic(1100, 10), false},
		{"legacy gas price bumped", 10, legacy(1000), legacy(1100), true},
		{"legacy gas price under the bump", 10, legacy(1000), legacy(1099), false},
		{"batch is never replaced", 10, append(dynamic(1000, 100), ethMsg(nil, 1)), dynamic(9000, 900), false},
		{"batch never replaces", 10, dynamic(1000, 100), append(dynamic(9000, 900), ethMsg(nil, 1)), false},
		{"cosmos tx is never replaced", 10, msgsTx{&banktypes.MsgSend{}}, dynamic(9000, 900), false},
		{"cosmos tx never replaces", 10, dynamic(1000, 100), msgsTx{&banktypes.MsgSend{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The pool priorities are ignored.
			require.Equal(t, tc.exp, ante.EthTxReplacement(tc.feeBump)(1, 0, tc.oTx, tc.nTx))
		})
	}
}
