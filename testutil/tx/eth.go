package tx

import (
	"math/big"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/x/evm/types"
)

func CreateContractMsgTx(
	nonce uint64,
	signer ethtypes.Signer,
	gasPrice *big.Int,
	from common.Address,
	keyringSigner keyring.Signer,
) (*types.MsgEthereumTx, error) {
	contractCreateTx := &ethtypes.AccessListTx{
		GasPrice: gasPrice,
		Gas:      params.TxGasContractCreation,
		To:       nil,
		Data:     []byte("contract_data"),
		Nonce:    nonce,
	}
	ethTx := ethtypes.NewTx(contractCreateTx)
	ethMsg := &types.MsgEthereumTx{}
	ethMsg.FromEthereumTx(ethTx)
	ethMsg.From = from.Bytes()

	return ethMsg, ethMsg.Sign(signer, keyringSigner)
}
