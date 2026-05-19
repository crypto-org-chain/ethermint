package keeper_test

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

var _ = Describe("EIP-1559 tx cost balance check", func() {
	const (
		baseFee   = int64(1)
		gasLimit  = uint64(21_000)
		gasFeeCap = int64(100)
		value     = int64(10_000)
	)

	var recipient = common.HexToAddress("0x2000000000000000000000000000000000000002")

	setupAndBuildTx := func(senderBalance *big.Int) []byte {
		s.SetupTest(s.T())
		s.App.FeeMarketKeeper.SetBaseFee(s.Ctx, big.NewInt(baseFee))

		denom := s.App.EvmKeeper.GetParams(s.Ctx).EvmDenom
		Expect(s.App.EvmKeeper.SetBalance(
			s.Ctx, s.Address, *uint256.MustFromBig(senderBalance), denom,
		)).To(Succeed())

		chainID := s.App.EvmKeeper.ChainID()
		nonce := s.App.EvmKeeper.GetNonce(s.Ctx, s.Address)
		msg := evmtypes.NewTx(
			chainID, nonce, &recipient,
			big.NewInt(value),
			gasLimit,
			nil,
			big.NewInt(gasFeeCap),
			big.NewInt(0), // gasTipCap
			nil,           // data
			&ethtypes.AccessList{},
		)
		msg.From = s.Address.Bytes()
		return s.PrepareEthTx(msg, s.PrivKey)
	}

	recipientBalance := func() *big.Int {
		denom := s.App.EvmKeeper.GetParams(s.Ctx).EvmDenom
		bal := s.App.EvmKeeper.GetBalance(
			s.Ctx, sdk.AccAddress(recipient.Bytes()), denom,
		)
		return big.NewInt(bal.ToBig().Int64())
	}

	It("baseline: balance >= max_cost passes CheckTx and DeliverTx", func() {
		txBz := setupAndBuildTx(big.NewInt(2_120_000))
		Expect(s.CheckTx(txBz).IsOK()).To(BeTrue(),
			"CheckTx must accept fully-funded dynamic-fee tx")
		Expect(s.DeliverTx(txBz).IsOK()).To(BeTrue(),
			"DeliverTx must accept fully-funded dynamic-fee tx")
		Expect(recipientBalance().Cmp(big.NewInt(value))).To(Equal(0),
			"recipient must receive value")
	})

	It("negative control: balance < value fails DeliverTx without crediting recipient", func() {
		txBz := setupAndBuildTx(big.NewInt(9_000))
		Expect(s.DeliverTx(txBz).IsOK()).To(BeFalse(),
			"DeliverTx must reject when balance < value")
		Expect(recipientBalance().Sign()).To(BeZero(),
			"recipient must not be credited")
	})

	It("gap case: balance in [effective_cost, max_cost) fails both CheckTx and DeliverTx", func() {
		txBz := setupAndBuildTx(big.NewInt(40_000))
		Expect(s.CheckTx(txBz).IsOK()).To(BeFalse(),
			"CheckTx must reject when balance < value + gas_limit * max_fee_per_gas")
		Expect(s.DeliverTx(txBz).IsOK()).To(BeFalse(),
			"DeliverTx must also reject")
		Expect(recipientBalance().Sign()).To(BeZero(),
			"recipient must not be credited")
	})
})
