package network

import (
	"context"
	"fmt"
	"time"

	"git.fuzzbuzz.io/fuzz"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

func FuzzNetworkRawRPC(f *fuzz.F) {
	msg := f.Bytes("msg").Get()
	ethjson := new(ethtypes.Transaction)
	binerr := ethjson.UnmarshalBinary(msg)
	if binerr == nil {
		testnetwork := New(nil, DefaultConfig())
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
		testnetwork.Cleanup()
	} else {
		f.Discard()
	}
}

// func FuzzNetworkBackend(f *fuzz.F) {
// 	args := types.SendTxArgs{}
// 	fromAddr := ethcommon.BytesToAddress(f.Bytes("To").Min(20).Max(20).Key())
// 	gasPriceI := f.Int64("GasPrice").Key()
// 	gasPrice := big.NewInt(gasPriceI)
// 	valueI := f.Int64("Value").Key()
// 	value := big.NewInt(valueI)
// 	chainI := f.Int64("ChainID").Key()
// 	chain := big.NewInt(chainI)
// 	f.Struct("ags").Register(types.SendTxArgs{
// 		From:       ethcommon.BytesToAddress(f.Bytes("From").Min(20).Max(20).Key()),
// 		To:         &fromAddr,
// 		Gas:        (*hexutil.Uint64)(f.Uint64Ptr("Gas").Key()),
// 		GasPrice:   (*hexutil.Big)(gasPrice),
// 		Value:      (*hexutil.Big)(value),
// 		Nonce:      (*hexutil.Uint64)(f.Uint64Ptr("Nonce").Key()),
// 		Data:       (*hexutil.Bytes)(f.BytesPtr("Data").Key()),
// 		Input:      (*hexutil.Bytes)(f.BytesPtr("Input").Key()),
// 		AccessList: &ethtypes.AccessList{},
// 		ChainID:    (*hexutil.Big)(chain),
// 	}).Populate(args)

// 	testnetwork := New(nil, DefaultConfig())
// 	// nolint
// 	testnetwork.Validators[0].ETHbackend.SendTransaction(args)
// 	h, err := testnetwork.WaitForHeightWithTimeout(10, time.Minute)
// 	if err != nil {
// 		f.Fail(fmt.Sprintf("expected to reach 10 blocks; got %d", h))
// 	}
// 	latestHeight, err := testnetwork.LatestHeight()
// 	if err != nil {
// 		f.Fail("latest height failed")
// 	}
// 	if latestHeight < h {
// 		f.Fail("latestHeight should be greater or equal to")
// 	}
// 	testnetwork.Cleanup()
// }
