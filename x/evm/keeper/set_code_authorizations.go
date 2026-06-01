package keeper

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

// validateAuthorization validates an EIP-7702 authorization against the state.
func (k *Keeper) validateAuthorization(auth *types.SetCodeAuthorization, stateDB vm.StateDB) (authority common.Address, err error) {
	// Verify chain ID is null or equal to current chain ID.
	if !auth.ChainID.IsZero() && auth.ChainID.CmpBig(k.eip155ChainID) != 0 {
		return authority, core.ErrAuthorizationWrongChainID
	}
	// Limit nonce to 2^64-1 per EIP-2681.
	if auth.Nonce+1 < auth.Nonce {
		return authority, core.ErrAuthorizationNonceOverflow
	}
	// Validate signature values and recover authority.
	authority, err = auth.Authority()
	if err != nil {
		return authority, fmt.Errorf("%w: %v", core.ErrAuthorizationInvalidSignature, err)
	}
	// Check the authority account
	//  1) doesn't have code or has exisiting delegation
	//  2) matches the auth's nonce
	//
	// Note it is added to the access list even if the authorization is invalid.
	stateDB.AddAddressToAccessList(authority)
	code := stateDB.GetCode(authority)
	if _, ok := types.ParseDelegation(code); len(code) != 0 && !ok {
		return authority, core.ErrAuthorizationDestinationHasCode
	}
	if have := stateDB.GetNonce(authority); have != auth.Nonce {
		return authority, core.ErrAuthorizationNonceMismatch
	}
	return authority, nil
}

// applyAuthorization validates and applies an EIP-7702 code delegation to the
// state, returning the recovered authority so callers can reuse it without a
// second (expensive) signature recovery.
func (k *Keeper) applyAuthorization(auth *types.SetCodeAuthorization, stateDB vm.StateDB) (common.Address, error) {
	authority, err := k.validateAuthorization(auth, stateDB)
	if err != nil {
		return authority, err
	}

	// If the account already exists in state, refund the new account cost
	// charged in the intrinsic calculation.
	if stateDB.Exist(authority) {
		stateDB.AddRefund(params.CallNewAccountGas - params.TxAuthTupleGas)
	}

	k.setAuthorizationDelegation(auth, authority, stateDB)
	return authority, nil
}

// applyDurableAuthorization replays the delegation effects of an
// already-validated authorization onto the durable stateDB. The authority was
// recovered by applyAuthorization, so we skip re-validation (no ecrecover) and
// the refund (which only matters for the EVM gas accounting on the main path).
func (k *Keeper) applyDurableAuthorization(auth *types.SetCodeAuthorization, authority common.Address, stateDB vm.StateDB) {
	k.setAuthorizationDelegation(auth, authority, stateDB)
}

// setAuthorizationDelegation writes the nonce bump and delegation code for a
// validated authorization.
func (k *Keeper) setAuthorizationDelegation(auth *types.SetCodeAuthorization, authority common.Address, stateDB vm.StateDB) {
	// Update nonce and account code.
	stateDB.SetNonce(authority, auth.Nonce+1, tracing.NonceChangeAuthorization)
	if auth.Address == (common.Address{}) {
		// Delegation to zero address means clear.
		stateDB.SetCode(authority, nil, tracing.CodeChangeAuthorizationClear)
		return
	}

	// Otherwise install delegation to auth.Address.
	stateDB.SetCode(authority, types.AddressToDelegation(auth.Address), tracing.CodeChangeAuthorization)
}
