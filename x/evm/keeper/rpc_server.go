package keeper

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/x/evm/statedb"
)

// EthmRpcServer is a RPC server wrapper around the keeper. It is updated on
// each new sdk.Message with the latest context and Ethereum core.Message.
type EthmRpcServer struct {
	Keeper *Keeper
}

func (s *EthmRpcServer) GetHash(args *GetHashArgs, reply *GetHashReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}

	reply.Hash = s.Keeper.GetHashFn(*ctx)(args.Height)
	return nil
}

func (s *EthmRpcServer) AddBalance(args *AddBalanceArgs, reply *AddBalanceReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}

	return s.Keeper.AddBalance(*ctx, args.Addr, args.Amount)
}

func (s *EthmRpcServer) SubBalance(args *SubBalanceArgs, reply *SubBalanceReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}

	return s.Keeper.SubBalance(*ctx, args.Addr, args.Amount)
}

func (s *EthmRpcServer) GetBalance(args *GetBalanceArgs, reply *GetBalanceReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}

	reply.Balance = s.Keeper.GetBalance(*ctx, args.Addr, args.Denom)
	return nil
}

func (s *EthmRpcServer) GetAccount(args *GetAccountArgs, reply *GetAccountReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}

	reply.Account = s.Keeper.GetAccount(*ctx, args.Addr)
	return nil
}

func (s *EthmRpcServer) GetState(args *GetStateArgs, reply *GetStateReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}

	reply.Hash = s.Keeper.GetState(*ctx, args.Addr, args.Key)
	return nil
}

func (s *EthmRpcServer) GetCode(args *GetCodeArgs, reply *GetCodeReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}

	reply.Code = s.Keeper.GetCode(*ctx, args.CodeHash)
	return nil
}

func (s *EthmRpcServer) SetAccount(args *SetAccountArgs, reply *SetAccountReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}

	return s.Keeper.SetAccount(*ctx, args.Addr, args.Account)
}

func (s *EthmRpcServer) SetState(args *SetStateArgs, reply *SetStateReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}
	s.Keeper.SetState(*ctx, args.Addr, args.Key, args.Value)
	return nil
}

func (s *EthmRpcServer) SetCode(args *SetCodeArgs, reply *SetCodeReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}
	s.Keeper.SetCode(*ctx, args.CodeHash, args.Code)
	return nil
}

func (s *EthmRpcServer) DeleteAccount(args *DeleteAccountArgs, reply *DeleteAccountReply) error {
	ctx := s.Keeper.getSdkCtx(args.EvmId)
	if ctx == nil {
		panic("context is invalid")
	}
	return s.Keeper.DeleteAccount(*ctx, args.Addr)
}

// AddBalanceArgs is the argument struct for the statedb.Keeper#AddBalance method.
type AddBalanceArgs struct {
	EvmId  uint64
	Addr   sdk.AccAddress
	Amount sdk.Coins
}

// AddBalanceReply is the reply struct for the statedb.Keeper#AddBalance method.
type AddBalanceReply struct {
}

// SubBalanceArgs is the argument struct for the statedb.Keeper#SubBalance method.
type SubBalanceArgs struct {
	EvmId  uint64
	Addr   sdk.AccAddress
	Amount sdk.Coins
}

// SubBalanceReply is the reply struct for the statedb.Keeper#SubBalance method.
type SubBalanceReply struct {
}

// GetBalanceArgs is the argument struct for the statedb.Keeper#GetBalance method.
type GetBalanceArgs struct {
	EvmId uint64
	Addr  sdk.AccAddress
	Denom string
}

// GetBalanceReply is the reply struct for the statedb.Keeper#GetBalance method.
type GetBalanceReply struct {
	Balance *big.Int
}

// GetAccountArgs is the argument struct for the statedb.Keeper#GetAccount method.
type GetAccountArgs struct {
	EvmId uint64
	Addr  common.Address
}

// GetAccountReply is the reply struct for the statedb.Keeper#GetAccount method.
type GetAccountReply struct {
	Account *statedb.Account
}

// GetStateArgs is the argument struct for the statedb.Keeper#GetState method.
type GetStateArgs struct {
	EvmId uint64
	Addr  common.Address
	Key   common.Hash
}

// GetStateReply is the reply struct for the statedb.Keeper#GetState method.
type GetStateReply struct {
	EvmId uint64
	Hash  common.Hash
}

// GetCodeArgs is the argument struct for the statedb.Keeper#GetCode method.
type GetCodeArgs struct {
	EvmId    uint64
	CodeHash common.Hash
}

// GetCodeReply is the reply struct for the statedb.Keeper#GetCode method.
type GetCodeReply struct {
	Code []byte
}

// SetAccountArgs is the argument struct for the statedb.Keeper#SetAccount method.
type SetAccountArgs struct {
	EvmId   uint64
	Addr    common.Address
	Account statedb.Account
}

// SetAccountReply is the reply struct for the statedb.Keeper#SetAccount method.
type SetAccountReply struct {
}

// SetStateArgs is the argument struct for the statedb.Keeper#SetState method.
type SetStateArgs struct {
	EvmId uint64
	Addr  common.Address
	Key   common.Hash
	Value []byte
}

// SetStateReply is the reply struct for the statedb.Keeper#SetState method.
type SetStateReply struct {
}

// SetCodeArgs is the argument struct for the statedb.Keeper#SetCode method.
type SetCodeArgs struct {
	EvmId    uint64
	CodeHash []byte
	Code     []byte
}

// SetCodeReply is the reply struct for the statedb.Keeper#SetCode method.
type SetCodeReply struct {
}

// DeleteAccountArgs is the argument struct for the statedb.Keeper#DeleteAccount method.
type DeleteAccountArgs struct {
	EvmId uint64
	Addr  common.Address
}

// DeleteAccountReply is the reply struct for the statedb.Keeper#DeleteAccount method.
type DeleteAccountReply struct {
}

// GetHashArgs is the argument struct for the statedb.Keeper#GetHash method.
type GetHashArgs struct {
	EvmId  uint64
	Height uint64
}

// GetHashReply is the reply struct for the statedb.Keeper#GetHash method.
type GetHashReply struct {
	Hash common.Hash
}
