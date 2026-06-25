package types

import (
	sdkmath "cosmossdk.io/math"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

type AuthList []SetCodeAuthorization

func NewAuthList(ethAuthList *[]ethtypes.SetCodeAuthorization) AuthList {
	if ethAuthList == nil {
		return nil
	}

	al := make([]SetCodeAuthorization, len(*ethAuthList))
	for i, auth := range *ethAuthList {
		chainID := sdkmath.NewIntFromBigInt(auth.ChainID.ToBig())

		al[i] = SetCodeAuthorization{
			ChainID: &chainID,
			Address: auth.Address.String(),
			Nonce:   auth.Nonce,
			V:       []byte{auth.V},
			R:       auth.R.Bytes(),
			S:       auth.S.Bytes(),
		}
	}
	return al
}

func (al AuthList) ToEthAuthList() *[]ethtypes.SetCodeAuthorization {
	ethAuthList := make([]ethtypes.SetCodeAuthorization, len(al))

	for i, auth := range al {
		chainID := new(uint256.Int)
		if auth.ChainID != nil {
			chainID.SetFromBig(auth.ChainID.BigInt())
		}

		r := uint256.NewInt(0).SetBytes(auth.R)
		s := uint256.NewInt(0).SetBytes(auth.S)

		// tolerate empty V for callers that bypass Validate
		var v byte
		if len(auth.V) > 0 {
			v = auth.V[0]
		}

		ethAuthList[i] = ethtypes.SetCodeAuthorization{
			ChainID: *chainID,
			Address: common.HexToAddress(auth.Address),
			Nonce:   auth.Nonce,
			V:       v,
			R:       *r,
			S:       *s,
		}
	}

	return &ethAuthList
}
