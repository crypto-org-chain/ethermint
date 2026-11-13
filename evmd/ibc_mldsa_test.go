package evmd_test

import (
	"testing"
	"time"

	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/crypto/mldsa65"
	"github.com/cometbft/cometbft/crypto/tmhash"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmtversion "github.com/cometbft/cometbft/proto/tendermint/version"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/runtime"
	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	commitmenttypes "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types"
	ibcexported "github.com/cosmos/ibc-go/v11/modules/core/exported"
	ibctm "github.com/cosmos/ibc-go/v11/modules/light-clients/07-tendermint"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
)

const _counterpartyChainID = "counterparty-1"

type IBCMlDsaTestSuite struct {
	appTestSuite
}

func TestIBCMlDsaTestSuite(t *testing.T) {
	suite.Run(t, new(IBCMlDsaTestSuite))
}

func (s *IBCMlDsaTestSuite) mlDsaKey() crypto.PrivKey {
	key, err := mldsa65.GenPrivKey()
	s.Require().NoError(err)
	return key
}

func (s *IBCMlDsaTestSuite) validatorSet(keys ...crypto.PrivKey) (*cmttypes.ValidatorSet, map[string]cmttypes.PrivValidator) {
	vals := make([]*cmttypes.Validator, len(keys))
	signers := make(map[string]cmttypes.PrivValidator, len(keys))
	for i, key := range keys {
		vals[i] = cmttypes.NewValidator(key.PubKey(), 10)
		signers[key.PubKey().Address().String()] = cmttypes.NewMockPVWithParams(key, false, false)
	}
	return cmttypes.NewValidatorSet(vals), signers
}

func (s *IBCMlDsaTestSuite) signedHeader(
	valSet *cmttypes.ValidatorSet, signers map[string]cmttypes.PrivValidator, height int64, blockTime time.Time,
) *tmproto.SignedHeader {
	unusedHash := tmhash.Sum([]byte{0x00})
	header := cmttypes.Header{
		Version:            cmtversion.Consensus{Block: version.BlockProtocol, App: 2},
		ChainID:            _counterpartyChainID,
		Height:             height,
		Time:               blockTime,
		LastBlockID:        cmttypes.BlockID{Hash: make([]byte, tmhash.Size), PartSetHeader: cmttypes.PartSetHeader{Total: 1, Hash: make([]byte, tmhash.Size)}},
		LastCommitHash:     unusedHash,
		DataHash:           unusedHash,
		ValidatorsHash:     valSet.Hash(),
		NextValidatorsHash: valSet.Hash(),
		ConsensusHash:      unusedHash,
		AppHash:            unusedHash,
		LastResultsHash:    unusedHash,
		EvidenceHash:       unusedHash,
		ProposerAddress:    valSet.Proposer.Address,
	}
	signedHeader, err := ibctesting.CommitHeader(header, valSet, signers)
	s.Require().NoError(err)
	return signedHeader
}

// Verifies a counterparty header through the same ibc-go 07-tendermint module that MsgUpdateClient runs.
func (s *IBCMlDsaTestSuite) TestVerifyMlDsaSignedHeader() {
	testCases := []struct {
		name     string
		keys     func() []crypto.PrivKey
		malleate func(*ibctm.Header)
		expPass  bool
	}{
		{
			name:     "ML-DSA-65 validators",
			keys:     func() []crypto.PrivKey { return []crypto.PrivKey{s.mlDsaKey(), s.mlDsaKey()} },
			malleate: func(*ibctm.Header) {},
			expPass:  true,
		},
		{
			name:     "ed25519 and ML-DSA-65 validators",
			keys:     func() []crypto.PrivKey { return []crypto.PrivKey{ed25519.GenPrivKey(), s.mlDsaKey()} },
			malleate: func(*ibctm.Header) {},
			expPass:  true,
		},
		{
			name: "tampered ML-DSA-65 signature",
			keys: func() []crypto.PrivKey { return []crypto.PrivKey{s.mlDsaKey()} },
			malleate: func(h *ibctm.Header) {
				h.SignedHeader.Commit.Signatures[0].Signature[0] ^= 0xff
			},
			expPass: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			valSet, signers := s.validatorSet(tc.keys()...)
			cdc := s.app.AppCodec()
			lightClient := ibctm.NewLightClientModule(
				cdc, clienttypes.NewStoreProvider(runtime.NewKVStoreService(s.app.GetKey(ibcexported.StoreKey))),
			)

			trustedTime := s.ctx.BlockTime().Add(-time.Minute)
			trustedHeight := clienttypes.NewHeight(1, 1)
			clientState := ibctm.NewClientState(
				_counterpartyChainID, ibctm.DefaultTrustLevel, time.Hour, 2*time.Hour, 10*time.Second,
				trustedHeight, commitmenttypes.GetSDKSpecs(), []string{"upgrade", "upgradedIBCState"},
			)
			consState := ibctm.NewConsensusState(trustedTime, commitmenttypes.NewMerkleRoot(tmhash.Sum([]byte("app"))), valSet.Hash())
			s.Require().NoError(lightClient.Initialize(
				s.ctx, _tmClientID, cdc.MustMarshal(clientState), cdc.MustMarshal(consState),
			))

			valSetProto, err := valSet.ToProto()
			s.Require().NoError(err)
			header := &ibctm.Header{
				SignedHeader:      s.signedHeader(valSet, signers, 2, trustedTime.Add(time.Second)),
				ValidatorSet:      valSetProto,
				TrustedHeight:     trustedHeight,
				TrustedValidators: valSetProto,
			}
			tc.malleate(header)

			// Decode the header from its MsgUpdateClient encoding, as a relayed tx would.
			headerBz, err := cdc.MarshalInterface(header)
			s.Require().NoError(err)
			var clientMsg ibcexported.ClientMessage
			s.Require().NoError(cdc.UnmarshalInterface(headerBz, &clientMsg))
			s.Require().NoError(clientMsg.ValidateBasic())

			err = lightClient.VerifyClientMessage(s.ctx, _tmClientID, clientMsg)
			if !tc.expPass {
				s.Require().ErrorContains(err, "wrong signature")
				return
			}
			s.Require().NoError(err)
			s.Require().Equal(
				[]ibcexported.Height{clienttypes.NewHeight(1, 2)},
				lightClient.UpdateState(s.ctx, _tmClientID, clientMsg),
			)
		})
	}
}
