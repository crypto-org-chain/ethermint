package backend

import (
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	goethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"google.golang.org/grpc/metadata"
)

func (suite *BackendTestSuite) TestSendTransaction() {
	gasPrice := new(hexutil.Big)
	gas := hexutil.Uint64(1)
	zeroGas := hexutil.Uint64(0)
	toAddr := tests.GenerateAddress()
	priv, _ := ethsecp256k1.GenerateKey()
	from := common.BytesToAddress(priv.PubKey().Address().Bytes())
	nonce := hexutil.Uint64(1)
	baseFee := sdkmath.NewInt(1)
	callArgsDefault := evmtypes.TransactionArgs{
		From:     &from,
		To:       &toAddr,
		GasPrice: gasPrice,
		Gas:      &gas,
		Nonce:    &nonce,
	}
	hash := common.Hash{}
	validator := sdk.AccAddress(tests.GenerateAddress().Bytes())
	height := int64(1)
	testCases := []struct {
		name         string
		registerMock func()
		args         evmtypes.TransactionArgs
		expHash      common.Hash
		expPass      bool
	}{
		{
			"fail - Can't find account in Keyring",
			func() {},
			evmtypes.TransactionArgs{},
			hash,
			false,
		},
		{
			"fail - Block error can't set Tx defaults",
			func() {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				armor := crypto.EncryptArmorPrivKey(priv, "", "eth_secp256k1")
				suite.backend.clientCtx.Keyring.ImportPrivKey("test_key", armor, "")
				RegisterParams(queryClient, &header, height)
				RegisterHeaderError(client, &height)
			},
			callArgsDefault,
			hash,
			false,
		},
		{
			"fail - Cannot validate transaction gas set to 0",
			func() {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				armor := crypto.EncryptArmorPrivKey(priv, "", "eth_secp256k1")
				suite.backend.clientCtx.Keyring.ImportPrivKey("test_key", armor, "")
				RegisterParams(queryClient, &header, 1)
				RegisterHeader(client, &height, nil)
				RegisterBlockResults(client, height)
				RegisterBaseFee(queryClient, baseFee)
				RegisterValidatorAccount(queryClient, validator)
			},
			evmtypes.TransactionArgs{
				From:     &from,
				To:       &toAddr,
				GasPrice: gasPrice,
				Gas:      &zeroGas,
				Nonce:    &nonce,
			},
			hash,
			false,
		},
		{
			"fail - Cannot broadcast transaction",
			func() {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				armor := crypto.EncryptArmorPrivKey(priv, "", "eth_secp256k1")
				suite.backend.clientCtx.Keyring.ImportPrivKey("test_key", armor, "")
				RegisterParams(queryClient, &header, 1)
				RegisterHeader(client, &height, nil)
				RegisterBlockResults(client, height)
				RegisterBaseFee(queryClient, baseFee)
				RegisterParamsWithoutHeader(queryClient, height)
				ethSigner := ethtypes.LatestSigner(suite.backend.ChainConfig())
				msg := callArgsDefault.ToTransaction()
				msg.Sign(ethSigner, suite.backend.clientCtx.Keyring)
				tx, _ := msg.BuildTx(suite.backend.clientCtx.TxConfig.NewTxBuilder(), "aphoton")
				txEncoder := suite.backend.clientCtx.TxConfig.TxEncoder()
				txBytes, _ := txEncoder(tx)
				RegisterBroadcastTxError(client, txBytes)
				RegisterValidatorAccount(queryClient, validator)
			},
			callArgsDefault,
			common.Hash{},
			false,
		},
		{
			"pass - Return the transaction hash",
			func() {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				armor := crypto.EncryptArmorPrivKey(priv, "", "eth_secp256k1")
				suite.backend.clientCtx.Keyring.ImportPrivKey("test_key", armor, "")
				RegisterParams(queryClient, &header, 1)
				RegisterHeader(client, &height, nil)
				RegisterBlockResults(client, height)
				RegisterBaseFee(queryClient, baseFee)
				RegisterParamsWithoutHeader(queryClient, height)
				ethSigner := ethtypes.LatestSigner(suite.backend.ChainConfig())
				msg := callArgsDefault.ToTransaction()
				msg.Sign(ethSigner, suite.backend.clientCtx.Keyring)
				tx, _ := msg.BuildTx(suite.backend.clientCtx.TxConfig.NewTxBuilder(), "aphoton")
				txEncoder := suite.backend.clientCtx.TxConfig.TxEncoder()
				txBytes, _ := txEncoder(tx)
				RegisterBroadcastTx(client, txBytes)
				RegisterValidatorAccount(queryClient, validator)
			},
			callArgsDefault,
			hash,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			if tc.expPass {
				// Sign the transaction and get the hash
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterParamsWithoutHeader(queryClient, 1)
				ethSigner := ethtypes.LatestSigner(suite.backend.ChainConfig())
				msg := callArgsDefault.ToTransaction()
				msg.Sign(ethSigner, suite.backend.clientCtx.Keyring)
				tc.expHash = msg.AsTransaction().Hash()
			}
			responseHash, err := suite.backend.SendTransaction(tc.args)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expHash, responseHash)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestSign() {
	from, priv := tests.NewAddrKey()
	testCases := []struct {
		name         string
		registerMock func()
		fromAddr     common.Address
		inputBz      hexutil.Bytes
		expPass      bool
	}{
		{
			"fail - can't find key in Keyring",
			func() {},
			from,
			nil,
			false,
		},
		{
			"pass - sign nil data",
			func() {
				armor := crypto.EncryptArmorPrivKey(priv, "", "eth_secp256k1")
				suite.backend.clientCtx.Keyring.ImportPrivKey("test_key", armor, "")
			},
			from,
			nil,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			responseBz, err := suite.backend.Sign(tc.fromAddr, tc.inputBz)
			if tc.expPass {
				signature, _, err := suite.backend.clientCtx.Keyring.SignByAddress((sdk.AccAddress)(from.Bytes()), accounts.TextHash(tc.inputBz), signing.SignMode_SIGN_MODE_TEXTUAL)
				signature[goethcrypto.RecoveryIDOffset] += 27
				suite.Require().NoError(err)
				suite.Require().Equal((hexutil.Bytes)(signature), responseBz)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

// TestSign_CannotForgeTransaction verifies that eth_sign applies the EIP-191
// prefix, making the returned signature unusable as a transaction signature.
func (suite *BackendTestSuite) TestSign_CannotForgeTransaction() {
	from, priv := tests.NewAddrKey()
	to := tests.GenerateAddress()

	armor := crypto.EncryptArmorPrivKey(priv, "", "eth_secp256k1")
	suite.Require().NoError(suite.backend.clientCtx.Keyring.ImportPrivKey("victim_key", armor, ""))

	unsignedTx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:   suite.backend.chainID,
		Nonce:     7,
		To:        &to,
		Gas:       21000,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Value:     big.NewInt(123456789),
	})
	signer := ethtypes.LatestSignerForChainID(suite.backend.chainID)
	txHash := signer.Hash(unsignedTx)

	signature, err := suite.backend.Sign(from, txHash.Bytes())
	suite.Require().NoError(err)
	suite.Require().Len(signature, goethcrypto.SignatureLength)

	// The signature should verify against the EIP-191 prefixed hash, not the raw tx hash.
	suite.Require().True(
		goethcrypto.VerifySignature(priv.PubKey().Bytes(), accounts.TextHash(txHash.Bytes()), signature[:goethcrypto.RecoveryIDOffset]),
		"signature should verify against TextHash(txHash)",
	)
	suite.Require().False(
		goethcrypto.VerifySignature(priv.PubKey().Bytes(), txHash.Bytes(), signature[:goethcrypto.RecoveryIDOffset]),
		"signature must NOT verify against raw txHash (would allow tx forgery)",
	)

	// Attaching the signature to the unsigned tx must NOT recover the victim address.
	signature[goethcrypto.RecoveryIDOffset] -= 27
	signedTx, err := unsignedTx.WithSignature(signer, signature)
	suite.Require().NoError(err)

	recoveredFrom, err := ethtypes.Sender(signer, signedTx)
	suite.Require().NoError(err)
	suite.Require().NotEqual(from, recoveredFrom, "forged tx must not recover victim address")
}

func (suite *BackendTestSuite) TestSignTypedData() {
	from, priv := tests.NewAddrKey()
	testCases := []struct {
		name           string
		registerMock   func()
		fromAddr       common.Address
		inputTypedData apitypes.TypedData
		expPass        bool
	}{
		{
			"fail - can't find key in Keyring",
			func() {},
			from,
			apitypes.TypedData{},
			false,
		},
		{
			"fail - empty TypeData",
			func() {
				armor := crypto.EncryptArmorPrivKey(priv, "", "eth_secp256k1")
				suite.backend.clientCtx.Keyring.ImportPrivKey("test_key", armor, "")
			},
			from,
			apitypes.TypedData{},
			false,
		},
		// TODO: Generate a TypedData msg
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			responseBz, err := suite.backend.SignTypedData(tc.fromAddr, tc.inputTypedData)

			if tc.expPass {
				sigHash, _, err := apitypes.TypedDataAndHash(tc.inputTypedData)
				suite.Require().NoError(err)
				signature, _, err := suite.backend.clientCtx.Keyring.SignByAddress((sdk.AccAddress)(from.Bytes()), sigHash, signing.SignMode_SIGN_MODE_TEXTUAL)
				signature[goethcrypto.RecoveryIDOffset] += 27
				suite.Require().NoError(err)
				suite.Require().Equal((hexutil.Bytes)(signature), responseBz)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}
