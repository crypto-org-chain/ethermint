package types_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"

	"cosmossdk.io/x/tx/signing/aminojson"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	cryptocodec "github.com/evmos/ethermint/crypto/codec"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	ethermintcodec "github.com/evmos/ethermint/encoding/codec"
	"github.com/evmos/ethermint/types"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func init() {
	amino := codec.NewLegacyAmino()
	cryptocodec.RegisterCrypto(amino)
}

type AccountTestSuite struct {
	suite.Suite

	account *types.EthAccount
	cdc     codec.JSONCodec
}

func (suite *AccountTestSuite) SetupTest() {
	privKey, err := ethsecp256k1.GenerateKey()
	suite.Require().NoError(err)
	pubKey := privKey.PubKey()
	addr := sdk.AccAddress(pubKey.Address())
	baseAcc := authtypes.NewBaseAccount(addr, pubKey, 10, 50)
	suite.account = &types.EthAccount{
		BaseAccount: baseAcc,
		CodeHash:    common.Hash{}.String(),
	}

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	ethermintcodec.RegisterInterfaces(interfaceRegistry)
	suite.cdc = codec.NewProtoCodec(interfaceRegistry)
}

func TestAccountTestSuite(t *testing.T) {
	suite.Run(t, new(AccountTestSuite))
}

func (suite *AccountTestSuite) TestAccountType() {
	suite.account.CodeHash = common.BytesToHash(crypto.Keccak256(nil)).Hex()
	suite.Require().Equal(types.AccountTypeEOA, suite.account.Type())
	suite.account.CodeHash = common.BytesToHash(crypto.Keccak256([]byte{1, 2, 3})).Hex()
	suite.Require().Equal(types.AccountTypeContract, suite.account.Type())
}

func TestAminoMarshal(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	ethermint.RegisterInterfaces(registry)
	value := "\n\x86\x02\n+tcrc1l8h07yxmwvc963tg08at7k9882d5ha4t34guxe\x12\xd2\x01\n)/cosmos.crypto.multisig.LegacyAminoPubKey\x12\xa4\x01\x08\x02\x12O\n(/ethermint.crypto.v1.ethsecp256k1.PubKey\x12#\n!\x03һ\xfb\x81\x1a\xb40E\xc0\x16\xf4\x17\xfba\xe5V\x10\xfd]\x8d\x19m\x08\xe0\xc6i\x83\xf9S\x9e\x1b\xd8\x12O\n(/ethermint.crypto.v1.ethsecp256k1.PubKey\x12#\n!\x02\x19]\xed\xa5\x84:\xae\xa2\x0fȌ\xb6MKR\xfc\xc5\xc8SD1\x0cSn\x83|P\xad'\xf7\xa8\x8d\x18\x12 \x02\x12B0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
	msg := &anypb.Any{
		TypeUrl: "/ethermint.types.v1.EthAccount",
		Value:   []byte(value),
	}
	aj := aminojson.NewEncoder(aminojson.EncoderOptions{
		FileResolver: registry,
		Indent:       "	",
	})
	_, err := aj.Marshal(msg)
	require.NoError(t, err)
}
