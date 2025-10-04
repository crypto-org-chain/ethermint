// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/evmos/ethermint/blob/main/LICENSE
package keeper

import (
	"encoding/binary"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	ethparams "github.com/ethereum/go-ethereum/params"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/holiman/uint256"
)

// CustomContractFn defines a custom precompiled contract generator with ctx, rules and returns a precompiled contract.
type CustomContractFn func(sdk.Context, ethparams.Rules) vm.PrecompiledContract

// GasNoLimit is the value for keeper.queryMaxGasLimit in case there is no limit
const GasNoLimit = 0

// Keeper grants access to the EVM module state and implements the go-ethereum StateDB interface.
type Keeper struct {
	// Protobuf codec
	cdc codec.Codec
	// Store key required for the EVM Prefix KVStore. It is required by:
	// - storing account's Storage State
	// - storing account's Code
	// - storing module parameters
	storeKey storetypes.StoreKey

	// key to access the object store, which is reset on every block during Commit
	objectKey storetypes.StoreKey

	// the address capable of executing a MsgUpdateParams message. Typically, this should be the x/gov module account.
	authority sdk.AccAddress
	// access to account state
	accountKeeper types.AccountKeeper
	// update balance and accounting operations with coins
	bankKeeper types.BankKeeper
	// access historical headers for EVM state transition execution
	stakingKeeper types.StakingKeeper
	// fetch EIP1559 base fee and parameters
	feeMarketKeeper types.FeeMarketKeeper

	// chain ID number obtained from the context's chain id
	eip155ChainID *big.Int

	// Tracer used to collect execution traces from the EVM transaction execution
	tracer string

	// EVM Hooks for tx post-processing
	hooks types.EvmHooks

	// Legacy subspace
	ss                paramstypes.Subspace
	customContractFns []CustomContractFn

	// queryMaxGasLimit max amount of gas allowed during a single tx execution, 0 means no limit
	queryMaxGasLimit uint64
}

// NewKeeper generates new evm module keeper
func NewKeeper(
	cdc codec.Codec,
	storeKey, objectKey storetypes.StoreKey,
	authority sdk.AccAddress,
	ak types.AccountKeeper,
	bankKeeper types.BankKeeper,
	sk types.StakingKeeper,
	fmk types.FeeMarketKeeper,
	tracer string,
	ss paramstypes.Subspace,
	customContractFns []CustomContractFn,
	queryMaxGasLimit uint64,
) *Keeper {
	// ensure evm module account is set
	if addr := ak.GetModuleAddress(types.ModuleName); addr == nil {
		panic("the EVM module account has not been set")
	}

	// ensure the authority account is correct
	if err := sdk.VerifyAddressFormat(authority); err != nil {
		panic(err)
	}

	// NOTE: we pass in the parameter space to the CommitStateDB in order to use custom denominations for the EVM operations
	return &Keeper{
		cdc:               cdc,
		authority:         authority,
		accountKeeper:     ak,
		bankKeeper:        bankKeeper,
		stakingKeeper:     sk,
		feeMarketKeeper:   fmk,
		storeKey:          storeKey,
		objectKey:         objectKey,
		tracer:            tracer,
		ss:                ss,
		customContractFns: customContractFns,
		queryMaxGasLimit:  queryMaxGasLimit,
	}
}

// Logger returns a module-specific logger.
func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return sdkCtx.Logger().With("module", "x/"+types.ModuleName)
}

// WithChainID sets the chain ID for the keeper by extracting it from the provided context
func (k *Keeper) WithChainID(ctx sdk.Context) {
	k.WithChainIDString(ctx.ChainID())
}

// WithChainIDString sets the chain ID for the keeper after parsing the provided string value
func (k *Keeper) WithChainIDString(value string) {
	chainID, err := ethermint.ParseChainID(value)
	if err != nil {
		panic(err)
	}

	if k.eip155ChainID != nil && k.eip155ChainID.Cmp(chainID) != 0 {
		panic("chain id already set")
	}

	k.eip155ChainID = chainID
}

// ChainID returns the EIP155 chain ID for the EVM context
func (k Keeper) ChainID() *big.Int {
	return k.eip155ChainID
}

// ----------------------------------------------------------------------------
// Block Bloom
// Required by Web3 API.
// ----------------------------------------------------------------------------

// EmitBlockBloomEvent emit block bloom events
func (k Keeper) EmitBlockBloomEvent(ctx sdk.Context, bloom []byte) {
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeBlockBloom,
			sdk.NewAttribute(types.AttributeKeyEthereumBloom, string(bloom)),
		),
	)
}

// GetAuthority returns the x/evm module authority address
func (k Keeper) GetAuthority() sdk.AccAddress {
	return k.authority
}

// ----------------------------------------------------------------------------
// Storage
// ----------------------------------------------------------------------------

// GetAccountStorage return state storage associated with an account
func (k Keeper) GetAccountStorage(ctx sdk.Context, address common.Address) types.Storage {
	storage := types.Storage{}

	k.ForEachStorage(ctx, address, func(key, value common.Hash) bool {
		storage = append(storage, types.NewState(key, value))
		return true
	})

	return storage
}

// ----------------------------------------------------------------------------
// Account
// ----------------------------------------------------------------------------

// SetHooks sets the hooks for the EVM module
// It should be called only once during initialization, it panic if called more than once.
func (k *Keeper) SetHooks(eh types.EvmHooks) *Keeper {
	if k.hooks != nil {
		panic("cannot set evm hooks twice")
	}

	k.hooks = eh
	return k
}

// PostTxProcessing delegate the call to the hooks. If no hook has been registered, this function returns with a `nil` error
func (k *Keeper) PostTxProcessing(ctx sdk.Context, msg *core.Message, receipt *ethtypes.Receipt) error {
	if k.hooks == nil {
		return nil
	}
	return k.hooks.PostTxProcessing(ctx, msg, receipt)
}

// Tracer return a default vm.Tracer based on current keeper state
func (k Keeper) Tracer(ctx sdk.Context, msg core.Message, ethCfg *ethparams.ChainConfig) *tracing.Hooks {
	return types.NewTracer(k.tracer, msg, ethCfg, ctx.BlockHeight(), uint64(ctx.BlockTime().Unix())) //#nosec G115 -- int overflow is not a concern here
}

// GetAccount load nonce and codehash without balance,
// more efficient in cases where balance is not needed.
func (k *Keeper) GetAccount(ctx sdk.Context, addr common.Address) *statedb.Account {
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	if acct == nil {
		return nil
	}
	return statedb.NewAccountFromSdkAccount(acct)
}

// GetAccountOrEmpty returns empty account if not exist, returns error if it's not `EthAccount`
func (k *Keeper) GetAccountOrEmpty(ctx sdk.Context, addr common.Address) statedb.Account {
	acct := k.GetAccount(ctx, addr)
	if acct != nil {
		return *acct
	}

	// empty account
	return statedb.Account{
		CodeHash: types.EmptyCodeHash,
	}
}

// GetNonce returns the sequence number of an account, returns 0 if not exists.
func (k *Keeper) GetNonce(ctx sdk.Context, addr common.Address) uint64 {
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	if acct == nil {
		return 0
	}

	return acct.GetSequence()
}

// GetEVMDenomBalance returns the balance of evm denom
func (k *Keeper) GetEVMDenomBalance(ctx sdk.Context, addr common.Address) *big.Int {
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	evmParams := k.GetParams(ctx)
	evmDenom := evmParams.GetEvmDenom()
	// if node is pruned, params is empty. Return invalid value
	if evmDenom == "" {
		return big.NewInt(-1)
	}
	balance := k.GetBalance(ctx, cosmosAddr, evmDenom)
	return balance.ToBig()
}

// GetBalance load account's balance of specified denom
func (k *Keeper) GetBalance(ctx sdk.Context, addr sdk.AccAddress, denom string) uint256.Int {
	balance := k.bankKeeper.GetBalance(ctx, addr, denom).Amount.BigInt()
	return *uint256.MustFromBig(balance)
}

// GetBaseFee returns current base fee, return values:
// - `nil`: london hardfork not enabled.
// - `0`: london hardfork enabled but feemarket is not enabled.
// - `n`: both london hardfork and feemarket are enabled.
func (k Keeper) GetBaseFee(ctx sdk.Context, ethCfg *ethparams.ChainConfig) *big.Int {
	return k.getBaseFee(ctx, types.IsLondon(ethCfg, ctx.BlockHeight()))
}

func (k Keeper) getBaseFee(ctx sdk.Context, london bool) *big.Int {
	if !london {
		return nil
	}
	baseFee := k.feeMarketKeeper.GetBaseFee(ctx)
	if baseFee == nil {
		// return 0 if feemarket not enabled.
		baseFee = big.NewInt(0)
	}
	return baseFee
}

// GetTransientGasUsed returns the gas used by current cosmos tx.
func (k Keeper) GetTransientGasUsed(ctx sdk.Context) uint64 {
	store := ctx.ObjectStore(k.objectKey)
	v := store.Get(types.ObjectGasUsedKey(ctx.TxIndex()))
	if v == nil {
		return 0
	}
	return v.(uint64)
}

// SetTransientGasUsed sets the gas used by current cosmos tx.
func (k Keeper) SetTransientGasUsed(ctx sdk.Context, gasUsed uint64) {
	store := ctx.ObjectStore(k.objectKey)
	store.Set(types.ObjectGasUsedKey(ctx.TxIndex()), gasUsed)
}

// AddTransientGasUsed accumulate gas used by each eth msgs included in current cosmos tx.
func (k Keeper) AddTransientGasUsed(ctx sdk.Context, gasUsed uint64) (uint64, error) {
	result := k.GetTransientGasUsed(ctx) + gasUsed
	if result < gasUsed {
		return 0, errorsmod.Wrap(types.ErrGasOverflow, "transient gas used")
	}
	k.SetTransientGasUsed(ctx, result)
	return result, nil
}

// SetHeaderHash stores the hash of the current block header in the store.
func (k Keeper) SetHeaderHash(ctx sdk.Context) {
	acct := k.GetAccount(ctx, ethparams.HistoryStorageAddress)
	if acct != nil && acct.IsContract() {
		window := types.DefaultHistoryServeWindow
		params := k.GetParams(ctx)
		if params.HistoryServeWindow > 0 {
			window = params.HistoryServeWindow
		}
		// set current block hash in the contract storage, compatible with EIP-2935
		ringIndex := uint64(ctx.BlockHeight()) % window //nolint:gosec // G115 // won't exceed uint64
		var key common.Hash
		binary.BigEndian.PutUint64(key[24:], ringIndex)
		k.SetState(ctx, ethparams.HistoryStorageAddress, key, ctx.HeaderHash())
	} else {
		// fallback old implementation
		store := ctx.KVStore(k.storeKey)
		height, err := ethermint.SafeUint64(ctx.BlockHeight())
		if err != nil {
			panic(err)
		}
		store.Set(types.GetHeaderHashKey(height), ctx.HeaderHash())
	}
}

// GetHeaderHash sets block hash into EIP-2935 compatible storage contract.
func (k Keeper) GetHeaderHash(ctx sdk.Context, height uint64) common.Hash {
	// check if history contract has been deployed
	acct := k.GetAccount(ctx, ethparams.HistoryStorageAddress)
	if acct != nil && acct.IsContract() {
		window := types.DefaultHistoryServeWindow
		params := k.GetParams(ctx)
		if params.HistoryServeWindow > 0 {
			window = params.HistoryServeWindow
		}

		ringIndex := height % window
		var key common.Hash
		binary.BigEndian.PutUint64(key[24:], ringIndex)
		hash := k.GetState(ctx, ethparams.HistoryStorageAddress, key)

		if hash.Cmp(common.Hash{}) != 0 {
			return hash
		}
	}
	// fall back to old behavior for retro compatibility
	// TODO can be removed along with DeleteHeaderHash once HistoryStorage has been filled up in next protocol upgrade
	store := ctx.KVStore(k.storeKey)
	hashByte := store.Get(types.GetHeaderHashKey(height))
	if len(hashByte) > 0 {
		return common.BytesToHash(hashByte)
	}
	return common.Hash{}
}

// DeleteHeaderHash removes the hash of a block header from the store by height
func (k Keeper) DeleteHeaderHash(ctx sdk.Context, height uint64) {
	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetHeaderHashKey(height))
}

func (k *Keeper) AddPreinstalls(ctx sdk.Context, preinstalls []types.Preinstall) error {
	for _, preinstall := range preinstalls {
		address := common.HexToAddress(preinstall.Address)
		accAddress := sdk.AccAddress(address.Bytes())

		if len(preinstall.Code) == 0 {
			return errorsmod.Wrapf(types.ErrInvalidPreinstall,
				"preinstall %s, address %s has no code", preinstall.Name, preinstall.Address)
		}

		// check that the address does not conflict with the precompiles
		cfg, err := k.EVMBlockConfig(ctx, k.ChainID())
		if err != nil {
			return err
		}
		for _, fn := range k.customContractFns {
			c := fn(ctx, cfg.Rules)
			if address == c.Address() {
				return errorsmod.Wrapf(types.ErrInvalidPreinstall,
					"preinstall %s, address %s already exists as a precompile", preinstall.Name, preinstall.Address)
			}
		}

		codeHash := crypto.Keccak256Hash(common.FromHex(preinstall.Code))
		codeHashBytes := codeHash.Bytes()
		if types.IsEmptyCodeHash(codeHashBytes) {
			k.Logger(ctx).Error("preinstall has empty code hash",
				"preinstall address", preinstall.Address)
			return errorsmod.Wrapf(types.ErrInvalidPreinstall,
				"preinstall %s, address %s has empty code hash", preinstall.Name, preinstall.Address)
		}

		acct := k.accountKeeper.GetAccount(ctx, accAddress)
		// check that the account is not already set
		if acct != nil {
			return errorsmod.Wrapf(types.ErrInvalidPreinstall,
				"preinstall %s, address %s already has an account in account keeper", preinstall.Name, preinstall.Address)
		}
		// create account with the account keeper and set code hash
		acct = k.accountKeeper.NewAccountWithAddress(ctx, accAddress)
		if ethAcct, ok := acct.(ethermint.EthAccountI); ok {
			if err := ethAcct.SetCodeHash(codeHash); err != nil {
				return err
			}
		}
		k.accountKeeper.SetAccount(ctx, acct)
		k.SetCode(ctx, codeHashBytes, common.FromHex(preinstall.Code))

		// We are not setting any storage for preinstalls, so we skip that step.
	}
	return nil
}

// GetCodeHash loads the code hash from the database for the given contract address.
func (k *Keeper) GetCodeHash(acct sdk.AccountI) common.Hash {
	if ethAcct, ok := acct.(ethermint.EthAccountI); ok {
		hash := ethAcct.GetCodeHash()
		if len(hash.Bytes()) == 0 {
			return common.BytesToHash(types.EmptyCodeHash)
		}
		return hash
	}
	return common.BytesToHash(types.EmptyCodeHash)
}
