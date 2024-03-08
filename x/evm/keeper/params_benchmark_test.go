package keeper_test

import (
	"testing"

	"github.com/evmos/ethermint/testutil"
	"github.com/evmos/ethermint/x/evm/types"
)

type ParamsBenchmarkTestSuite struct {
	testutil.BaseTestSuite
}

func BenchmarkSetParams(b *testing.B) {
	suite := ParamsBenchmarkTestSuite{}
	suite.SetupTest()
	params := types.DefaultParams()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = suite.App.EvmKeeper.SetParams(suite.Ctx, params)
	}
}

func BenchmarkGetParams(b *testing.B) {
	suite := ParamsBenchmarkTestSuite{}
	suite.SetupTest()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = suite.App.EvmKeeper.GetParams(suite.Ctx)
	}
}
