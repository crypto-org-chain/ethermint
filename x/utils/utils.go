// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/evmos/ethermint/blob/main/LICENSE
package utils

import (
	"context"
	"strconv"

	"google.golang.org/grpc/metadata"

	errorsmod "cosmossdk.io/errors"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
)

func GetHeightFromMetadata(c context.Context) (int64, error) {
	if md, ok := metadata.FromIncomingContext(c); ok {
		heightHeaders := md.Get(grpctypes.GRPCBlockHeightHeader)
		// Get height header from the request context, if present.
		if len(heightHeaders) == 1 {
			height, err := strconv.ParseInt(heightHeaders[0], 10, 64)
			if err != nil {
				return height, errorsmod.Wrapf(
					sdkerrors.ErrInvalidRequest,
					"invalid height header %q: %v", grpctypes.GRPCBlockHeightHeader, err)
			}
			if height < 0 {
				return height, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "cannot query with height < 0; please provide a valid height")
			}
		}
	}
	return 0, nil
}
