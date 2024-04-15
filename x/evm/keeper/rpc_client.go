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

type sgxRPCClient struct {
	logger log.Logger
	cl     *rpc.Client
}

// newSgxRPCClient creates a new RPC client to communicate with the SGX binary.
func newSgxRPCClient(logger log.Logger) (*sgxRPCClient, error) {
	// TODO Make ports configurable
	cl, err := rpc.DialHTTP("tcp", "localhost"+":9092")
	if err != nil {
		return nil, err
	}

	return &sgxRPCClient{
		logger: logger,
		cl:     cl,
	}, nil
}

func (c *sgxRPCClient) doCall(method string, args, reply any) error {
	c.logger.Debug(fmt.Sprintf("RPC call %s", method), "args", args)
	err := c.cl.Call(method, args, reply)
	c.logger.Debug(fmt.Sprintf("RPC call %s", method), "reply", reply)
	return err
}

func (c *sgxRPCClient) PrepareTx(args PrepareTxArgs, reply *PrepareTxReply) error {
	return c.doCall("SgxRpcServer.PrepareTx", args, reply)
}

func (c *sgxRPCClient) Call(args CallArgs, reply *CallReply) error {
	return c.doCall("SgxRpcServer.Call", args, reply)
}

func (c *sgxRPCClient) Create(args CreateArgs, reply *CreateReply) error {
	return c.doCall("SgxRpcServer.Create", args, reply)
}

func (c *sgxRPCClient) Commit(args CommitArgs, reply *CommitReply) error {
	return c.doCall("SgxRpcServer.Commit", args, reply)
}

func (c *sgxRPCClient) StateDBAddBalance(args StateDBAddBalanceArgs, reply *StateDBAddBalanceReply) error {
	return c.doCall("SgxRpcServer.StateDBAddBalance", args, reply)
}

func (c *sgxRPCClient) StateDBSubBalance(args StateDBSubBalanceArgs, reply *StateDBSubBalanceReply) error {
	return c.doCall("SgxRpcServer.StateDBSubBalance", args, reply)
}

func (c *sgxRPCClient) StateDBSetNonce(args StateDBSetNonceArgs, reply *StateDBSetNonceReply) error {
	return c.doCall("SgxRpcServer.StateDBSetNonce", args, reply)
}

func (c *sgxRPCClient) StateDBIncreaseNonce(args StateDBIncreaseNonceArgs, reply *StateDBIncreaseNonceReply) error {
	return c.doCall("SgxRpcServer.StateDBIncreaseNonce", args, reply)
}

func (c *sgxRPCClient) StateDBPrepare(args StateDBPrepareArgs, reply *StateDBPrepareReply) error {
	return c.doCall("SgxRpcServer.StateDBPrepare", args, reply)
}

func (c *sgxRPCClient) StateDBGetRefund(args StateDBGetRefundArgs, reply *StateDBGetRefundReply) error {
	return c.doCall("SgxRpcServer.StateDBGetRefund", args, reply)
}

func (c *sgxRPCClient) StateDBGetLogs(args StateDBGetLogsArgs, reply *StateDBGetLogsReply) error {
	return c.doCall("SgxRpcServer.StateDBGetLogs", args, reply)
}

// PrepareTxEVMConfig only contains the fields from EVMConfig that are needed
// to create a new EVM instance. This is used to pass the EVM configuration
// over RPC to the SGX binary.
type PrepareTxEVMConfig struct {
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

// PrepareTxArgs is the argument struct for the SgxRpcServer.PrepareTx RPC method.
type PrepareTxArgs struct {
	TxHash []byte
	// Header is the Tendermint header of the block in which the transaction
	// will be executed.
	Header cmtproto.Header
	// Msg is the EVM transaction message to run on the EVM.
	Msg core.Message
	// EvmConfig is the EVM configuration to set.
	EvmConfig PrepareTxEVMConfig
}

// PrepareTxArgs is the reply struct for the SgxRpcServer.PrepareTx RPC method.
type PrepareTxReply struct {
}

// CallArgs is the argument struct for the SgxRpcServer.Call RPC method.
type CallArgs struct {
	Caller vm.AccountRef
	Addr   common.Address
	Input  []byte
	Gas    uint64
	Value  *big.Int
}

// CallReply is the reply struct for the SgxRpcServer.Call RPC method.
type CallReply struct {
	Ret         []byte
	LeftOverGas uint64
}

// CreateArgs is the argument struct for the SgxRpcServer.Create RPC method.
type CreateArgs struct {
	Caller vm.AccountRef
	Code   []byte
	Gas    uint64
	Value  *big.Int
}

// CreateReply is the reply struct for the SgxRpcServer.Create RPC method.
type CreateReply struct {
	Ret          []byte
	ContractAddr common.Address
	LeftOverGas  uint64
}

// CommitArgs is the argument struct for the SgxRpcServer.Commit RPC method.
type CommitArgs struct {
	Commit bool
}

// CommitReply is the reply struct for the SgxRpcServer.Commit RPC method.
type CommitReply struct {
}

// CommitArgs is the argument struct for the SgxRpcServer.StateDBSubBalance RPC method.
type StateDBSubBalanceArgs struct {
	Caller vm.AccountRef
	Msg    core.Message
}

// CommitReply is the reply struct for the SgxRpcServer.StateDBSubBalance RPC method.
type StateDBSubBalanceReply struct {
}

// CommitArgs is the argument struct for the SgxRpcServer.StateDSetNonce RPC method.
type StateDBSetNonceArgs struct {
	Caller vm.AccountRef
	Nonce  uint64
}

// CommitReply is the reply struct for the SgxRpcServer.StateDSetNonce RPC method.
type StateDBSetNonceReply struct {
}

// StateDBAddBalanceArgs is the argument struct for the SgxRpcServer.StateDBAddBalance RPC method.
type StateDBAddBalanceArgs struct {
	Caller      vm.AccountRef
	Msg         core.Message
	LeftoverGas uint64
}

// StateDBAddBalanceReply is the reply struct for the SgxRpcServer.StateDBAddBalance RPC method.
type StateDBAddBalanceReply struct {
}

type StateDBPrepareArgs struct {
	Msg   core.Message
	Rules params.Rules
}

type StateDBPrepareReply struct {
}

// StateDBIncreaseNonceArgs is the argument struct for the SgxRpcServer.StateDBIncreaseNonce RPC method.
type StateDBIncreaseNonceArgs struct {
	Caller vm.AccountRef
	Msg    core.Message
}

// StateDBIncreaseNonceReply is the reply struct for the SgxRpcServer.StateDBIncreaseNonce RPC method.
type StateDBIncreaseNonceReply struct {
}

type StateDBGetRefundArgs struct {
}

type StateDBGetRefundReply struct {
	Refund uint64
}

type StateDBGetLogsArgs struct {
}

type StateDBGetLogsReply struct {
	Logs []*ethtypes.Log
}
