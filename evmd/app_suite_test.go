package evmd_test

import (
	"time"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/suite"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/evmos/ethermint/evmd"
	"github.com/evmos/ethermint/testutil"
)

const _tmClientID = "07-tendermint-0"

type appTestSuite struct {
	suite.Suite

	app *evmd.EthermintApp
	ctx sdk.Context
}

func (s *appTestSuite) SetupTest() {
	s.app = testutil.Setup(false, nil)
	s.ctx = s.app.NewUncachedContext(false, tmproto.Header{Height: 5, Time: time.Now().UTC()}) //nolint:staticcheck
}
