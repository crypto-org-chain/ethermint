package evmd

import (
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmempool "github.com/cosmos/cosmos-sdk/types/mempool"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	protov2 "google.golang.org/protobuf/proto"
)

type emptyEthExtTx struct {
	msgs []sdk.Msg
	opts []*codectypes.Any
}

func (t emptyEthExtTx) GetMsgs() []sdk.Msg                          { return t.msgs }
func (t emptyEthExtTx) GetMsgsV2() ([]protov2.Message, error)       { return nil, nil }
func (t emptyEthExtTx) GetExtensionOptions() []*codectypes.Any      { return t.opts }
func (t emptyEthExtTx) GetNonCriticalExtensionOptions() []*codectypes.Any {
	return nil
}

func TestEthSignerExtractionAdapter_NilAsTransaction(t *testing.T) {
	option, err := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
	if err != nil {
		t.Fatalf("NewAnyWithValue: %v", err)
	}
	tx := emptyEthExtTx{
		msgs: []sdk.Msg{&evmtypes.MsgEthereumTx{From: []byte{0x01}}},
		opts: []*codectypes.Any{option},
	}
	adapter := NewEthSignerExtractionAdapter(sdkmempool.NewDefaultSignerExtractionAdapter())

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GetSigners panicked on nil AsTransaction: %v", r)
		}
	}()
	sigs, err := adapter.GetSigners(tx)
	if err == nil {
		t.Fatal("expected error for nil ethereum transaction")
	}
	if sigs != nil {
		t.Fatalf("expected nil signers, got %v", sigs)
	}
}
