package network

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"git.fuzzbuzz.io/fuzz"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/tharsis/ethermint/ethereum/rpc/types"
)

func FuzzNetworkRawRPC(f *fuzz.F) {
	msg := f.Bytes("msg").Get()
	ethjson := new(ethtypes.Transaction)
	binerr := ethjson.UnmarshalBinary(msg)
	if binerr == nil {
		testnetwork := New(nil, DefaultConfig())
		defer testnetwork.Cleanup()
		// nolint
		testnetwork.Validators[0].JSONRPCClient.SendTransaction(context.Background(), ethjson)
		h, err := testnetwork.WaitForHeightWithTimeout(10, time.Minute)
		if err != nil {
			f.Fail(fmt.Sprintf("expected to reach 10 blocks; got %d", h))
		}
		latestHeight, err := testnetwork.LatestHeight()
		if err != nil {
			f.Fail("latest height failed")
		}
		if latestHeight < h {
			f.Fail("latestHeight should be greater or equal to")
		}
	}
}

func FuzzNetworkBackend(f *fuzz.F) {
	al := &ethtypes.AccessList{}
	f.Array("AccessList", ethtypes.AccessList{}).Populate(al)
	to := ethcommon.BytesToAddress(f.Bytes("To").Min(20).Max(20).Get())
	args := types.SendTxArgs{
		From:       ethcommon.BytesToAddress(f.Bytes("From").Min(20).Max(20).Get()),
		To:         &to,
		Gas:        (*hexutil.Uint64)(f.Uint64Ptr("Gas").Get()),
		GasPrice:   (*hexutil.Big)(big.NewInt(f.Int64("GasPrice").Get())),
		Value:      (*hexutil.Big)(big.NewInt(f.Int64("Value").Get())),
		Nonce:      (*hexutil.Uint64)(f.Uint64Ptr("Nonce").Get()),
		Data:       (*hexutil.Bytes)(f.BytesPtr("Data").Get()),
		Input:      (*hexutil.Bytes)(f.BytesPtr("Input").Get()),
		ChainID:    (*hexutil.Big)(big.NewInt(f.Int64("ChainID").Get())),
		AccessList: al,
	}
	testnetwork := New(nil, DefaultConfig())
	defer testnetwork.Cleanup()
	// nolint
	testnetwork.Validators[0].ETHbackend.SendTransaction(args)
	h, err := testnetwork.WaitForHeightWithTimeout(10, time.Minute)
	if err != nil {
		f.Fail(fmt.Sprintf("expected to reach 10 blocks; got %d", h))
	}
	latestHeight, err := testnetwork.LatestHeight()
	if err != nil {
		f.Fail("latest height failed")
	}
	if latestHeight < h {
		f.Fail("latestHeight should be greater or equal to")
	}
}
