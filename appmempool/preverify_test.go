package appmempool_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/suite"

	"github.com/evmos/ethermint/ante/cache"
	"github.com/evmos/ethermint/appmempool"
	"github.com/evmos/ethermint/tests"
	testutilconfig "github.com/evmos/ethermint/testutil/config"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

const testChainID = "ethermint_9000-1"

type PreVerifierTestSuite struct {
	suite.Suite

	txConfig client.TxConfig
	chainID  *big.Int
	signer   ethtypes.Signer
	addr     common.Address
	priv     cryptotypes.PrivKey
}

func TestPreVerifierTestSuite(t *testing.T) {
	suite.Run(t, new(PreVerifierTestSuite))
}

func (s *PreVerifierTestSuite) SetupTest() {
	s.txConfig = testutilconfig.MakeConfigForTest(nil).TxConfig

	chainID, err := ethermint.ParseChainID(testChainID)
	s.Require().NoError(err)
	s.chainID = chainID
	s.signer = ethtypes.LatestSignerForChainID(chainID)

	s.addr, s.priv = tests.NewAddrKey()
}

func (s *PreVerifierTestSuite) signedEthMsg() *evmtypes.MsgEthereumTx {
	to := tests.GenerateAddress()
	msg := evmtypes.NewTx(s.chainID, 0, &to, big.NewInt(10), 100000, big.NewInt(1), nil, nil, nil, nil)
	msg.From = s.addr.Bytes()
	s.Require().NoError(msg.Sign(s.signer, tests.NewSigner(s.priv)))
	return msg
}

func (s *PreVerifierTestSuite) bankSendMsg() *banktypes.MsgSend {
	return banktypes.NewMsgSend(s.addr.Bytes(), tests.GenerateAddress().Bytes(),
		sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1))))
}

func (s *PreVerifierTestSuite) dataFormatMsg(msg *evmtypes.MsgEthereumTx) *evmtypes.MsgEthereumTx {
	txData, err := evmtypes.NewTxDataFromTx(msg.AsTransaction())
	s.Require().NoError(err)
	data, err := evmtypes.PackTxData(txData)
	s.Require().NoError(err)
	return &evmtypes.MsgEthereumTx{From: msg.From, Data: data}
}

// wireDecoded round-trips through the codec so the verifier sees a wire-decoded tx, as in admission.
func (s *PreVerifierTestSuite) wireDecoded(tx sdk.Tx) sdk.Tx {
	bz, err := s.txConfig.TxEncoder()(tx)
	s.Require().NoError(err)
	decoded, err := s.txConfig.TxDecoder()(bz)
	s.Require().NoError(err)
	return decoded
}

func (s *PreVerifierTestSuite) buildEthTx(msg *evmtypes.MsgEthereumTx) sdk.Tx {
	tx, err := msg.BuildTx(s.txConfig.NewTxBuilder(), evmtypes.DefaultEVMDenom)
	s.Require().NoError(err)
	return s.wireDecoded(tx)
}

func (s *PreVerifierTestSuite) buildTx(msgs ...sdk.Msg) sdk.Tx {
	builder := s.txConfig.NewTxBuilder()
	s.Require().NoError(builder.SetMsgs(msgs...))
	return s.wireDecoded(builder.GetTx())
}

func (s *PreVerifierTestSuite) TestNewEVMSigPreVerifier() {
	testCases := []struct {
		name    string
		chainID string
		expNil  bool
	}{
		// A nil verifier keeps admission fully on the locked path.
		{"unparsable chain ID", "garbage", true},
		{"valid chain ID", testChainID, false},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			verifier := appmempool.NewEVMSigPreVerifier(tc.chainID, nil)
			if tc.expNil {
				s.Require().Nil(verifier)
				return
			}
			s.Require().NotNil(verifier)
		})
	}
}

func (s *PreVerifierTestSuite) TestVerify() {
	testCases := []struct {
		name     string
		malleate func() sdk.Tx
		expErr   error
	}{
		{
			"valid signature on a pure EVM tx",
			func() sdk.Tx { return s.buildEthTx(s.signedEthMsg()) },
			nil,
		},
		{
			"tampered sender on a pure EVM tx",
			func() sdk.Tx {
				msg := s.signedEthMsg()
				msg.From = tests.GenerateAddress().Bytes()
				return s.buildEthTx(msg)
			},
			errortypes.ErrorInvalidSigner,
		},
		{
			"valid signature on a legacy Data-format msg",
			func() sdk.Tx { return s.buildTx(s.dataFormatMsg(s.signedEthMsg())) },
			nil,
		},
		{
			"tampered sender on a legacy Data-format msg",
			func() sdk.Tx {
				msg := s.dataFormatMsg(s.signedEthMsg())
				msg.From = tests.GenerateAddress().Bytes()
				return s.buildTx(msg)
			},
			errortypes.ErrorInvalidSigner,
		},
		{
			"tx without msgs defers",
			func() sdk.Tx { return s.buildTx() },
			nil,
		},
		{
			"cosmos tx defers",
			func() sdk.Tx { return s.buildTx(s.bankSendMsg()) },
			nil,
		},
		{
			// The bad EVM signature is left for the locked path once any msg is non-EVM.
			"mixed EVM and cosmos tx defers",
			func() sdk.Tx {
				msg := s.signedEthMsg()
				msg.From = tests.GenerateAddress().Bytes()
				return s.buildTx(msg, s.bankSendMsg())
			},
			nil,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			verifier := appmempool.NewEVMSigPreVerifier(testChainID, nil)
			err := verifier(tc.malleate())
			if tc.expErr != nil {
				s.Require().ErrorIs(err, tc.expErr)
				return
			}
			s.Require().NoError(err)
		})
	}
}

func (s *PreVerifierTestSuite) TestSenderCache() {
	senderCache := cache.NewSenderCache(64)
	verifier := appmempool.NewEVMSigPreVerifier(testChainID, senderCache)
	msg := s.signedEthMsg()
	tx := s.buildEthTx(msg)

	s.Require().NoError(verifier(tx))
	s.Require().NoError(verifier(tx))

	hits, misses := senderCache.Stats()
	s.Require().Equal(uint64(1), misses, "only the first call should miss, populating the cache")
	s.Require().Equal(uint64(1), hits, "second call should hit senderCache instead of re-running ecrecover")

	cached, ok := senderCache.Get(msg.AsTransaction(), s.signer)
	s.Require().True(ok)
	s.Require().Equal(s.addr, cached)
}
