package legacyparams_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/types/legacyparams"
	"github.com/stretchr/testify/suite"
)

type ProposalTestSuite struct {
	suite.Suite
}

func TestProposalTestSuite(t *testing.T) {
	suite.Run(t, new(ProposalTestSuite))
}

func (s *ProposalTestSuite) TestDecodeStoredProposal() {
	content := &legacyparams.ParameterChangeProposal{
		Title:       "raise min gas price",
		Description: "feemarket change",
		Changes:     []legacyparams.ParamChange{{Subspace: "feemarket", Key: "MinGasPrice", Value: `"5000"`}},
	}
	msg, err := govv1.NewLegacyContent(content, authtypes.NewModuleAddress(govtypes.ModuleName).String())
	s.Require().NoError(err)
	anyMsg, err := codectypes.NewAnyWithValue(msg)
	s.Require().NoError(err)
	stored, err := (&govv1.Proposal{Id: 1, Messages: []*codectypes.Any{anyMsg}}).Marshal()
	s.Require().NoError(err)

	testCases := []struct {
		name      string
		registry  func() codectypes.InterfaceRegistry
		expectErr bool
	}{
		{
			"ethermint encoding config decodes it",
			func() codectypes.InterfaceRegistry { return encoding.MakeConfig().InterfaceRegistry },
			false,
		},
		{
			"gov registrations alone cannot",
			codectypes.NewInterfaceRegistry,
			true,
		},
	}
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			registry := tc.registry()
			govv1.RegisterInterfaces(registry)
			govv1beta1.RegisterInterfaces(registry)
			var proposal govv1.Proposal
			err := codec.NewProtoCodec(registry).Unmarshal(stored, &proposal)
			if tc.expectErr {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)
			legacy, err := govv1.LegacyContentFromMessage(proposal.Messages[0].GetCachedValue().(*govv1.MsgExecLegacyContent))
			s.Require().NoError(err)
			s.Require().Equal(content, legacy)
			s.Require().Equal("params", legacy.ProposalRoute())
			s.Require().ErrorIs(legacy.ValidateBasic(), legacyparams.ErrRemoved)
		})
	}
}
