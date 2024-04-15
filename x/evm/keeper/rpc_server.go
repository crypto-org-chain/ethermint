package keeper

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/evmos/ethermint/x/evm/statedb"
)

// EthmRpcServer is a RPC server wrapper around the keeper. It is updated on
// each new sdk.Message with the latest context and Ethereum core.Message.
type EthmRpcServer struct {
	ctx    sdk.Context
	msg    core.Message
	evmCfg *EVMConfig
	k      *Keeper
}

func (s *EthmRpcServer) GetHash(height *uint64, hash *common.Hash) error {
	*hash = s.k.GetHashFn(s.ctx)(*height)
	return nil
}

func (s *EthmRpcServer) AddBalance(args *AddBalanceArgs, reply *AddBalanceReply) error {
	return s.k.AddBalance(s.ctx, args.Addr, args.Amount)
}

func (s *EthmRpcServer) SubBalance(args *SubBalanceArgs, reply *SubBalanceReply) error {
	return s.k.SubBalance(s.ctx, args.Addr, args.Amount)
}

func (s *EthmRpcServer) GetBalance(args *GetBalanceArgs, reply *GetBalanceReply) error {
	reply.Balance = s.k.GetBalance(s.ctx, args.Addr, args.Denom)
	return nil
}

func (s *EthmRpcServer) GetAccount(args *GetAccountArgs, reply *GetAccountReply) error {
	reply.Account = s.k.GetAccount(s.ctx, args.Addr)
	return nil
}

func (s *EthmRpcServer) GetState(args *GetStateArgs, reply *GetStateReply) error {
	reply.Hash = s.k.GetState(s.ctx, args.Addr, args.Key)
	return nil
}

func (s *EthmRpcServer) GetCode(args *GetCodeArgs, reply *GetCodeReply) error {
	reply.Code = s.k.GetCode(s.ctx, args.CodeHash)
	return nil
}

func (s *EthmRpcServer) SetAccount(args *SetAccountArgs, reply *SetAccountReply) error {
	return s.k.SetAccount(s.ctx, args.Addr, args.Account)
}

func (s *EthmRpcServer) SetState(args *SetStateArgs, reply *SetStateReply) error {
	s.k.SetState(s.ctx, args.Addr, args.Key, args.Value)
	return nil
}

func (s *EthmRpcServer) SetCode(args *SetCodeArgs, reply *SetCodeReply) error {
	s.k.SetCode(s.ctx, args.CodeHash, args.Code)
	return nil
}

func (s *EthmRpcServer) DeleteAccount(args *DeleteAccountArgs, reply *DeleteAccountReply) error {
	return s.k.DeleteAccount(s.ctx, args.Addr)
}

// AddBalanceArgs is the argument struct for the statedb.Keeper#AddBalance method.
type AddBalanceArgs struct {
	Addr   sdk.AccAddress
	Amount sdk.Coins
}

// AddBalanceReply is the reply struct for the statedb.Keeper#AddBalance method.
type AddBalanceReply struct {
}

// SubBalanceArgs is the argument struct for the statedb.Keeper#SubBalance method.
type SubBalanceArgs struct {
	Addr   sdk.AccAddress
	Amount sdk.Coins
}

// SubBalanceReply is the reply struct for the statedb.Keeper#SubBalance method.
type SubBalanceReply struct {
}

// GetBalanceArgs is the argument struct for the statedb.Keeper#GetBalance method.
type GetBalanceArgs struct {
	Addr  sdk.AccAddress
	Denom string
}

// GetBalanceReply is the reply struct for the statedb.Keeper#GetBalance method.
type GetBalanceReply struct {
	Balance *big.Int
}

// GetAccountArgs is the argument struct for the statedb.Keeper#GetAccount method.
type GetAccountArgs struct {
	Addr common.Address
}

// GetAccountReply is the reply struct for the statedb.Keeper#GetAccount method.
type GetAccountReply struct {
	Account *statedb.Account
}

// GetStateArgs is the argument struct for the statedb.Keeper#GetState method.
type GetStateArgs struct {
	Addr common.Address
	Key  common.Hash
}

// GetStateReply is the reply struct for the statedb.Keeper#GetState method.
type GetStateReply struct {
	Hash common.Hash
}

// GetCodeArgs is the argument struct for the statedb.Keeper#GetCode method.
type GetCodeArgs struct {
	CodeHash common.Hash
}

// GetCodeReply is the reply struct for the statedb.Keeper#GetCode method.
type GetCodeReply struct {
	Code []byte
}

// SetAccountArgs is the argument struct for the statedb.Keeper#SetAccount method.
type SetAccountArgs struct {
	Addr    common.Address
	Account statedb.Account
}

// SetAccountReply is the reply struct for the statedb.Keeper#SetAccount method.
type SetAccountReply struct {
}

// SetStateArgs is the argument struct for the statedb.Keeper#SetState method.
type SetStateArgs struct {
	Addr  common.Address
	Key   common.Hash
	Value []byte
}

// SetStateReply is the reply struct for the statedb.Keeper#SetState method.
type SetStateReply struct {
}

// SetCodeArgs is the argument struct for the statedb.Keeper#SetCode method.
type SetCodeArgs struct {
	CodeHash []byte
	Code     []byte
}

// SetCodeReply is the reply struct for the statedb.Keeper#SetCode method.
type SetCodeReply struct {
}

// DeleteAccountArgs is the argument struct for the statedb.Keeper#DeleteAccount method.
type DeleteAccountArgs struct {
	Addr common.Address
}

// DeleteAccountReply is the reply struct for the statedb.Keeper#DeleteAccount method.
type DeleteAccountReply struct {
}
