package network

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/tharsis/ethermint/ethereum/rpc/types"

	"git.fuzzbuzz.io/fuzz"
)

func FuzzNetworkRawRPC(f *fuzz.F) {
	msg := f.Bytes("msg").Get()
	ethjson := new(ethtypes.Transaction)
	jsonerr := json.Unmarshal(msg, ethjson)
	if jsonerr == nil {
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

func FuzzNetworkBackend(f *fuzz.F) {
	args := types.SendTxArgs{}
	f.Struct("args", types.SendTxArgs{}).Populate(args)
	testnetwork := New(nil, DefaultConfig())
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
	testnetwork.Cleanup()
}
