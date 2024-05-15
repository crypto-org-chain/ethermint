package keeper

import (
	"fmt"
	"math/big"
	"net/rpc"

	"cosmossdk.io/log"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/x/evm/statedb"
)

type teeRPCClient struct {
	logger log.Logger
	cl     *rpc.Client
}

// newTEERPCClient creates a new RPC client to communicate with the TEE binary.
func newTEERPCClient(logger log.Logger) (*teeRPCClient, error) {
	// TODO Make ports configurable
	cl, err := rpc.DialHTTP("tcp", "localhost"+":9092")
	if err != nil {
		return nil, err
	}

	return &teeRPCClient{
		logger: logger,
		cl:     cl,
	}, nil
}

func (c *teeRPCClient) doCall(method string, args, reply any) error {
	c.logger.Debug(fmt.Sprintf("RPC call %s", method), "args", args)
	err := c.cl.Call(method, args, reply)
	c.logger.Debug(fmt.Sprintf("RPC call %s", method), "reply", reply)
	return err
}

func (c *teeRPCClient) StartEVM(args StartEVMArgs, reply *StartEVMReply) error {
	return c.doCall("TEERpcServer.StartEVM", args, reply)
}

func (c *teeRPCClient) InitFhevm(args InitFhevmArgs, reply *InitFhevmReply) error {
	return c.doCall("TEERpcServer.InitFhevm", args, reply)
}

func (c *teeRPCClient) Call(args CallArgs, reply *CallReply) error {
	return c.doCall("TEERpcServer.Call", args, reply)
}

func (c *teeRPCClient) Create(args CreateArgs, reply *CreateReply) error {
	return c.doCall("TEERpcServer.Create", args, reply)
}

func (c *teeRPCClient) Commit(args CommitArgs, reply *CommitReply) error {
	return c.doCall("TEERpcServer.Commit", args, reply)
}

func (c *teeRPCClient) StateDBAddBalance(args StateDBAddBalanceArgs, reply *StateDBAddBalanceReply) error {
	return c.doCall("TEERpcServer.StateDBAddBalance", args, reply)
}

func (c *teeRPCClient) StateDBSubBalance(args StateDBSubBalanceArgs, reply *StateDBSubBalanceReply) error {
	return c.doCall("TEERpcServer.StateDBSubBalance", args, reply)
}

func (c *teeRPCClient) StateDBSetNonce(args StateDBSetNonceArgs, reply *StateDBSetNonceReply) error {
	return c.doCall("TEERpcServer.StateDBSetNonce", args, reply)
}

func (c *teeRPCClient) StateDBIncreaseNonce(args StateDBIncreaseNonceArgs, reply *StateDBIncreaseNonceReply) error {
	return c.doCall("TEERpcServer.StateDBIncreaseNonce", args, reply)
}

func (c *teeRPCClient) StateDBPrepare(args StateDBPrepareArgs, reply *StateDBPrepareReply) error {
	return c.doCall("TEERpcServer.StateDBPrepare", args, reply)
}

func (c *teeRPCClient) StateDBGetRefund(args StateDBGetRefundArgs, reply *StateDBGetRefundReply) error {
	return c.doCall("TEERpcServer.StateDBGetRefund", args, reply)
}

func (c *teeRPCClient) StateDBGetLogs(args StateDBGetLogsArgs, reply *StateDBGetLogsReply) error {
	return c.doCall("TEERpcServer.StateDBGetLogs", args, reply)
}

func (c *teeRPCClient) StopEVM(args StopEVMArgs, reply *StopEVMReply) error {
	return c.doCall("TEERpcServer.StopEVM", args, reply)
}

// StartEVMTxEVMConfig only contains the fields from EVMConfig that are needed
// to create a new EVM instance. This is used to pass the EVM configuration
// over RPC to the TEE binary.
type StartEVMTxEVMConfig struct {
	// ChainConfig is the EVM chain configuration in JSON format. Since the
	// underlying params.ChainConfig struct contains pointer fields, they are
	// not serializable over RPC with gob. Instead, the JSON representation is
	// used.
	ChainConfigJson []byte

	// Fields from EVMConfig
	CoinBase   common.Address
	BaseFee    *big.Int
	TxConfig   statedb.TxConfig
	DebugTrace bool

	// Fields from EVMConfig.FeeMarketParams struct
	NoBaseFee bool

	// Fields from EVMConfig.Params struct
	EvmDenom  string
	ExtraEips []int
	// *rpctypes.StateOverride : original type
	Overrides string
}

// StartEVMArgs is the argument struct for the TEERpcServer.StartEVM RPC method.
type StartEVMArgs struct {
	TxHash []byte
	// Header is the Tendermint header of the block in which the transaction
	// will be executed.
	Header cmtproto.Header
	// Msg is the EVM transaction message to run on the EVM.
	Msg core.Message
	// EvmConfig is the EVM configuration to set.
	EvmConfig StartEVMTxEVMConfig
}

// StartEVMReply is the reply struct for the TEERpcServer.StartEVM RPC method.
type StartEVMReply struct {
	EvmId uint64
}

// InitFhevmArgs is the arg struct for the TEERpcServer.InitFhevm RPC method.
type InitFhevmArgs struct {
	EvmId uint64
}

// InitFhevmReply is the reply struct for the TEERpcServer.InitFhevm RPC method.
type InitFhevmReply struct {
}

// CallArgs is the argument struct for the TEERpcServer.Call RPC method.
type CallArgs struct {
	EvmId  uint64
	Caller vm.AccountRef
	Addr   common.Address
	Input  []byte
	Gas    uint64
	Value  *big.Int
}

// CallReply is the reply struct for the TEERpcServer.Call RPC method.
type CallReply struct {
	Ret         []byte
	LeftOverGas uint64
}

// CreateArgs is the argument struct for the TEERpcServer.Create RPC method.
type CreateArgs struct {
	EvmId  uint64
	Caller vm.AccountRef
	Code   []byte
	Gas    uint64
	Value  *big.Int
}

// CreateReply is the reply struct for the TEERpcServer.Create RPC method.
type CreateReply struct {
	Ret          []byte
	ContractAddr common.Address
	LeftOverGas  uint64
}

// CommitArgs is the argument struct for the TEERpcServer.Commit RPC method.
type CommitArgs struct {
	EvmId uint64
}

// CommitReply is the reply struct for the TEERpcServer.Commit RPC method.
type CommitReply struct {
}

// CommitArgs is the argument struct for the TEERpcServer.StateDBSubBalance RPC method.
type StateDBSubBalanceArgs struct {
	EvmId  uint64
	Caller vm.AccountRef
	Msg    core.Message
}

// CommitReply is the reply struct for the TEERpcServer.StateDBSubBalance RPC method.
type StateDBSubBalanceReply struct {
}

// CommitArgs is the argument struct for the TEERpcServer.StateDSetNonce RPC method.
type StateDBSetNonceArgs struct {
	EvmId  uint64
	Caller vm.AccountRef
	Nonce  uint64
}

// CommitReply is the reply struct for the TEERpcServer.StateDSetNonce RPC method.
type StateDBSetNonceReply struct {
}

// StateDBAddBalanceArgs is the argument struct for the TEERpcServer.StateDBAddBalance RPC method.
type StateDBAddBalanceArgs struct {
	EvmId       uint64
	Caller      vm.AccountRef
	Msg         core.Message
	LeftoverGas uint64
}

// StateDBAddBalanceReply is the reply struct for the TEERpcServer.StateDBAddBalance RPC method.
type StateDBAddBalanceReply struct {
}

type StateDBPrepareArgs struct {
	EvmId    uint64
	Msg      core.Message
	Rules    params.Rules
	CoinBase common.Address
}

type StateDBPrepareReply struct {
}

// StateDBIncreaseNonceArgs is the argument struct for the TEERpcServer.StateDBIncreaseNonce RPC method.
type StateDBIncreaseNonceArgs struct {
	EvmId  uint64
	Caller vm.AccountRef
	Msg    core.Message
}

// StateDBIncreaseNonceReply is the reply struct for the TEERpcServer.StateDBIncreaseNonce RPC method.
type StateDBIncreaseNonceReply struct {
}

type StateDBGetRefundArgs struct {
	EvmId uint64
}

type StateDBGetRefundReply struct {
	Refund uint64
}

type StateDBGetLogsArgs struct {
	EvmId uint64
}

type StateDBGetLogsReply struct {
	Logs []*ethtypes.Log
}

type StopEVMArgs struct {
	EvmId uint64
}

type StopEVMReply struct {
}
