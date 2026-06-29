package types

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalBlockNumber(t *testing.T) {
	testCases := []struct {
		msg         string
		input       []byte
		expValue    BlockNumber
		expErr      bool
		expErrIs    error
		expErrValue string
	}{
		{
			"hex block number",
			[]byte("\"0x35\""),
			BlockNumber(0x35),
			false,
			nil,
			"",
		},
		{
			"latest block tag",
			[]byte("\"latest\""),
			EthLatestBlockNumber,
			false,
			nil,
			"",
		},
		{
			"safe block tag",
			[]byte("\"safe\""),
			EthLatestBlockNumber,
			false,
			nil,
			"",
		},
		{
			"pending block tag",
			[]byte("\"pending\""),
			EthPendingBlockNumber,
			false,
			nil,
			"",
		},
		{
			"quoted decimal block number",
			[]byte("\"35\""),
			BlockNumber(0),
			true,
			hexutil.ErrMissingPrefix,
			"hex string without 0x prefix",
		},
		{
			"unquoted decimal block number",
			[]byte("35"),
			BlockNumber(0),
			true,
			hexutil.ErrMissingPrefix,
			"hex string without 0x prefix",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.msg, func(t *testing.T) {
			var bn BlockNumber
			err := bn.UnmarshalJSON(tc.input)
			if tc.expErr {
				require.Error(t, err)
				if tc.expErrIs != nil {
					require.ErrorIs(t, err, tc.expErrIs)
				}
				if tc.expErrValue != "" {
					require.EqualError(t, err, tc.expErrValue)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expValue, bn)
		})
	}
}

func TestUnmarshalBlockNumberOrHash(t *testing.T) {
	bnh := new(BlockNumberOrHash)

	testCases := []struct {
		msg      string
		input    []byte
		malleate func()
		expPass  bool
	}{
		{
			"JSON input with block hash",
			[]byte("{\"blockHash\": \"0x579917054e325746fda5c3ee431d73d26255bc4e10b51163862368629ae19739\"}"),
			func() {
				require.Equal(t, *bnh.BlockHash, common.HexToHash("0x579917054e325746fda5c3ee431d73d26255bc4e10b51163862368629ae19739"))
				require.Nil(t, bnh.BlockNumber)
			},
			true,
		},
		{
			"JSON input with block number",
			[]byte("{\"blockNumber\": \"0x35\"}"),
			func() {
				require.Equal(t, *bnh.BlockNumber, BlockNumber(0x35))
				require.Nil(t, bnh.BlockHash)
			},
			true,
		},
		{
			"JSON input with block number latest",
			[]byte("{\"blockNumber\": \"latest\"}"),
			func() {
				require.Equal(t, *bnh.BlockNumber, EthLatestBlockNumber)
				require.Nil(t, bnh.BlockHash)
			},
			true,
		},
		{
			"JSON input with decimal block number",
			[]byte("{\"blockNumber\": \"35\"}"),
			func() {
			},
			false,
		},
		{
			"JSON input with both block hash and block number",
			[]byte("{\"blockHash\": \"0x579917054e325746fda5c3ee431d73d26255bc4e10b51163862368629ae19739\", \"blockNumber\": \"0x35\"}"),
			func() {
			},
			false,
		},
		{
			"String input with block hash",
			[]byte("\"0x579917054e325746fda5c3ee431d73d26255bc4e10b51163862368629ae19739\""),
			func() {
				require.Equal(t, *bnh.BlockHash, common.HexToHash("0x579917054e325746fda5c3ee431d73d26255bc4e10b51163862368629ae19739"))
				require.Nil(t, bnh.BlockNumber)
			},
			true,
		},
		{
			"String input with block number",
			[]byte("\"0x35\""),
			func() {
				require.Equal(t, *bnh.BlockNumber, BlockNumber(0x35))
				require.Nil(t, bnh.BlockHash)
			},
			true,
		},
		{
			"String input with block number latest",
			[]byte("\"latest\""),
			func() {
				require.Equal(t, *bnh.BlockNumber, EthLatestBlockNumber)
				require.Nil(t, bnh.BlockHash)
			},
			true,
		},
		{
			"String input with decimal block number",
			[]byte("\"35\""),
			func() {
			},
			false,
		},
		{
			"String input with block number safe",
			[]byte("\"safe\""),
			func() {
				require.Equal(t, *bnh.BlockNumber, EthLatestBlockNumber)
				require.Nil(t, bnh.BlockHash)
			},
			true,
		},
		{
			"String input with block number overflow",
			[]byte("\"0xffffffffffffffffffffffffffffffffffffff\""),
			func() {
			},
			false,
		},
	}

	for _, tc := range testCases {
		fmt.Printf("Case %s", tc.msg)
		// reset input
		bnh = new(BlockNumberOrHash)
		err := bnh.UnmarshalJSON(tc.input)
		tc.malleate()
		if tc.expPass {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
