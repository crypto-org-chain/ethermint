package keeper_test

import (
	"testing"

	"github.com/cometbft/cometbft/abci/types"
	"github.com/evmos/ethermint/testutil"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/suite"
)

type ABCITestSuite struct {
	testutil.BaseTestSuite
}

func TestABCITestSuite(t *testing.T) {
	suite.Run(t, new(ABCITestSuite))
}

func (suite *ABCITestSuite) TestInitGenesis() {
	suite.SetupTest()
	em := suite.Ctx.EventManager()
	suite.Require().Equal(0, len(em.Events()))

	res := suite.App.EvmKeeper.EndBlock(suite.Ctx, types.RequestEndBlock{})
	suite.Require().Equal([]types.ValidatorUpdate{}, res)

	// should emit 1 EventTypeBlockBloom event on EndBlock
	suite.Require().Equal(1, len(em.Events()))
	suite.Require().Equal(evmtypes.EventTypeBlockBloom, em.Events()[0].Type)
}
