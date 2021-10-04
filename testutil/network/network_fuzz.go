package network_fuzz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"git.fuzzbuzz.io/fuzz"
	"github.com/tharsis/ethermint/testutil/network"
)

func FuzzNetworkRawRPC(f *fuzz.F) {
	msg := f.Bytes("msg").Get()
	var ethjson *ethtypes.Transaction = new(ethtypes.Transaction)
	jsonerr := json.Unmarshal(msg, ethjson)
	if jsonerr == nil {
		testnetwork := network.New(nil, network.DefaultConfig())
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
