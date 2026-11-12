package types

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/suite"
)

type ParamsTestSuite struct {
	suite.Suite
}

func TestParamsTestSuite(t *testing.T) {
	suite.Run(t, new(ParamsTestSuite))
}

func (suite *ParamsTestSuite) TestParamsValidate() {
	withDefaults := func(malleate func(*Params)) Params {
		p := DefaultParams()
		malleate(&p)
		return p
	}

	testCases := []struct {
		name     string
		params   Params
		expError bool
	}{
		{"default", DefaultParams(), false},
		{
			"valid",
			NewParams(true, 7, 3, 2000000000, int64(544435345345435345), sdkmath.LegacyNewDecWithPrec(20, 4), DefaultMinGasMultiplier),
			false,
		},
		{
			"empty",
			Params{},
			true,
		},
		{
			"base fee change denominator is 0 ",
			NewParams(true, 0, 3, 2000000000, int64(544435345345435345), sdkmath.LegacyNewDecWithPrec(20, 4), DefaultMinGasMultiplier),
			true,
		},
		{
			"invalid: min gas price negative",
			NewParams(true, 7, 3, 2000000000, int64(544435345345435345), sdkmath.LegacyNewDecFromInt(sdkmath.NewInt(-1)), DefaultMinGasMultiplier),
			true,
		},
		{
			"valid: min gas multiplier zero",
			NewParams(true, 7, 3, 2000000000, int64(544435345345435345), DefaultMinGasPrice, sdkmath.LegacyZeroDec()),
			false,
		},
		{
			"invalid: min gas multiplier is negative",
			NewParams(true, 7, 3, 2000000000, int64(544435345345435345), DefaultMinGasPrice, sdkmath.LegacyNewDecWithPrec(-5, 1)),
			true,
		},
		{
			"invalid: min gas multiplier bigger than 1",
			NewParams(true, 7, 3, 2000000000, int64(544435345345435345), sdkmath.LegacyNewDecWithPrec(20, 4), sdkmath.LegacyNewDec(2)),
			true,
		},
		{
			// EIP-1559 base fee math divides by it.
			"invalid: elasticity multiplier is 0",
			withDefaults(func(p *Params) { p.ElasticityMultiplier = 0 }),
			true,
		},
		{
			// Missing from genesis JSON or MsgUpdateParams.
			"invalid: base fee is nil",
			withDefaults(func(p *Params) { p.BaseFee = sdkmath.Int{} }),
			true,
		},
		{
			"invalid: base fee is negative",
			withDefaults(func(p *Params) { p.BaseFee = sdkmath.NewInt(-1) }),
			true,
		},
		{
			"invalid: enable height is negative",
			withDefaults(func(p *Params) { p.EnableHeight = -1 }),
			true,
		},
	}

	for _, tc := range testCases {
		err := tc.params.Validate()

		if tc.expError {
			suite.Require().Error(err, tc.name)
		} else {
			suite.Require().NoError(err, tc.name)
		}
	}
}

func (suite *ParamsTestSuite) TestParamsValidatePriv() {
	suite.Require().Error(validateMinGasPrice(sdkmath.LegacyDec{}))
	suite.Require().Error(validateMinGasMultiplier(sdkmath.LegacyNewDec(-5)))
	suite.Require().Error(validateMinGasMultiplier(sdkmath.LegacyDec{}))
}

func (suite *ParamsTestSuite) TestParamsValidateMinGasPrice() {
	testCases := []struct {
		name     string
		value    sdkmath.LegacyDec
		expError bool
	}{
		{"default", DefaultParams().MinGasPrice, false},
		{"valid", sdkmath.LegacyNewDecFromInt(sdkmath.NewInt(1)), false},
		{"invalid - is nil", sdkmath.LegacyDec{}, true},
		{"invalid - is negative", sdkmath.LegacyNewDecFromInt(sdkmath.NewInt(-1)), true},
	}

	for _, tc := range testCases {
		err := validateMinGasPrice(tc.value)

		if tc.expError {
			suite.Require().Error(err, tc.name)
		} else {
			suite.Require().NoError(err, tc.name)
		}
	}
}
