package legacyparams

import (
	"errors"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
)

const (
	proposalRoute      = "params"
	proposalTypeChange = "ParameterChange"
)

var ErrRemoved = errors.New("x/params has been removed; ParameterChangeProposal is decode-only")

var _ govv1beta1.Content = &ParameterChangeProposal{}

// RegisterInterfaces lets gov proposal queries and export decode ParameterChangeProposals stored before
// SDK v0.55. New ones are still rejected: no gov handler is registered for the "params" route.
func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	registry.RegisterImplementations((*govv1beta1.Content)(nil), &ParameterChangeProposal{})
}

func (pcp *ParameterChangeProposal) GetTitle() string { return pcp.Title }

func (pcp *ParameterChangeProposal) GetDescription() string { return pcp.Description }

func (pcp *ParameterChangeProposal) ProposalRoute() string { return proposalRoute }

func (pcp *ParameterChangeProposal) ProposalType() string { return proposalTypeChange }

func (pcp *ParameterChangeProposal) ValidateBasic() error { return ErrRemoved }
