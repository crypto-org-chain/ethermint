package cosmos_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	secp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/evmos/ethermint/ante/cosmos"
)

func TestFindRejectedStakingMsg(t *testing.T) {
	from := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address()).String()
	to := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address()).String()
	granteeAddr := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	valSrc := sdk.ValAddress(secp256k1.GenPrivKey().PubKey().Address()).String()
	valDst := sdk.ValAddress(secp256k1.GenPrivKey().PubKey().Address()).String()

	send := &banktypes.MsgSend{
		FromAddress: from,
		ToAddress:   to,
		Amount:      sdk.NewCoins(sdk.NewCoin("aphoton", sdkmath.NewInt(1))),
	}
	undel := &stakingtypes.MsgUndelegate{
		DelegatorAddress: from,
		ValidatorAddress: valSrc,
		Amount:           sdk.NewCoin("aphoton", sdkmath.NewInt(1)),
	}
	redel := &stakingtypes.MsgBeginRedelegate{
		DelegatorAddress:    from,
		ValidatorSrcAddress: valSrc,
		ValidatorDstAddress: valDst,
		Amount:              sdk.NewCoin("aphoton", sdkmath.NewInt(1)),
	}
	exec := func(inner ...sdk.Msg) sdk.Msg {
		m := authz.NewMsgExec(granteeAddr, inner)
		return &m
	}

	cases := []struct {
		name    string
		msgs    []sdk.Msg
		wantURL string
	}{
		{
			name:    "empty",
			msgs:    nil,
			wantURL: "",
		},
		{
			name:    "non-staking only",
			msgs:    []sdk.Msg{send},
			wantURL: "",
		},
		{
			name:    "bare MsgUndelegate",
			msgs:    []sdk.Msg{undel},
			wantURL: sdk.MsgTypeURL(undel),
		},
		{
			name:    "bare MsgBeginRedelegate",
			msgs:    []sdk.Msg{redel},
			wantURL: sdk.MsgTypeURL(redel),
		},
		{
			// authz bypass: grantee submits MsgExec wrapping a staking msg.
			// The decorator must descend into MsgExec and still flag it.
			name:    "MsgExec wrapping MsgUndelegate",
			msgs:    []sdk.Msg{exec(undel)},
			wantURL: sdk.MsgTypeURL(undel),
		},
		{
			name:    "MsgExec wrapping MsgBeginRedelegate",
			msgs:    []sdk.Msg{exec(redel)},
			wantURL: sdk.MsgTypeURL(redel),
		},
		{
			// Single-level unwrap is bypassable by adding another wrap.
			name:    "nested MsgExec wrapping MsgUndelegate",
			msgs:    []sdk.Msg{exec(exec(undel))},
			wantURL: sdk.MsgTypeURL(undel),
		},
		{
			name:    "MsgExec wrapping non-staking msg",
			msgs:    []sdk.Msg{exec(send)},
			wantURL: "",
		},
		{
			// Mixed: a non-staking MsgExec followed by a bare staking msg
			// at the top level must still be flagged.
			name:    "MsgExec(non-staking) + bare MsgUndelegate",
			msgs:    []sdk.Msg{exec(send), undel},
			wantURL: sdk.MsgTypeURL(undel),
		},
		{
			// Deep nesting is allowed at this layer — the global cap is
			// enforced by AuthzLimiterDecorator earlier in the ante chain.
			// FindRejectedStakingMsg must still descend and flag the inner
			// staking msg.
			name:    "deeply nested MsgExec wrapping MsgUndelegate",
			msgs:    []sdk.Msg{exec(exec(exec(exec(exec(exec(exec(undel)))))))},
			wantURL: sdk.MsgTypeURL(undel),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, err := cosmos.FindRejectedStakingMsg(tc.msgs)
			require.NoError(t, err)
			require.Equal(t, tc.wantURL, url)
		})
	}
}
