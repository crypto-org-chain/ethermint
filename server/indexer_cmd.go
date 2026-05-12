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
package server

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	cmtnode "github.com/cometbft/cometbft/config"
	sm "github.com/cometbft/cometbft/state"
	cmtstore "github.com/cometbft/cometbft/store"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/evmos/ethermint/indexer"
)

const directionRange = "range"

func NewIndexTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index-eth-tx [backward|forward|range <start> <end>]",
		Short: "Index historical eth txs",
		Long: `Index historical eth txs, it only support two traverse direction to avoid creating gaps in the indexer db if using arbitrary block ranges:
		- backward: index the blocks from the first indexed block to the earliest block in the chain, if indexer db is empty, start from the latest block.
		- forward: index the blocks from the latest indexed block to latest block in the chain.
		- range <start> <end>: re-index a specific inclusive block range, overwriting any existing entries.

		When start the node, the indexer start from the latest indexed block to avoid creating gap.
        Backward mode should be used most of the time, so the latest indexed block is always up-to-date.
		`,
		Args: cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverCtx := server.GetServerContextFromCmd(cmd)
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			direction := args[0]
			if direction != "backward" && direction != "forward" && direction != directionRange {
				return fmt.Errorf("unknown index direction, expect: backward|forward|range, got: %s", direction)
			}
			if direction == directionRange && len(args) != 3 {
				return fmt.Errorf("range requires exactly two arguments: <start> <end>")
			}

			cfg := serverCtx.Config
			home := cfg.RootDir
			logger := serverCtx.Logger
			idxDB, err := OpenIndexerDB(home, server.GetAppDBBackend(serverCtx.Viper))
			if err != nil {
				logger.Error("failed to open evm indexer DB", "error", err.Error())
				return err
			}
			idxer := indexer.NewKVIndexer(idxDB, logger.With("module", "evmindex"), clientCtx)

			// open local tendermint db, because the local rpc won't be available.
			cmtdb, err := cmtnode.DefaultDBProvider(&cmtnode.DBContext{ID: "blockstore", Config: cfg})
			if err != nil {
				return err
			}
			blockStore := cmtstore.NewBlockStore(cmtdb)

			stateDB, err := cmtnode.DefaultDBProvider(&cmtnode.DBContext{ID: "state", Config: cfg})
			if err != nil {
				return err
			}
			stateStore := sm.NewStore(stateDB, sm.StoreOptions{
				DiscardABCIResponses: cfg.Storage.DiscardABCIResponses,
			})

			indexBlock := func(height int64) error {
				blk := blockStore.LoadBlock(height)
				if blk == nil {
					return fmt.Errorf("block not found %d", height)
				}
				resBlk, err := stateStore.LoadFinalizeBlockResponse(height)
				if err != nil {
					return err
				}
				if err := idxer.IndexBlock(blk, resBlk.TxResults); err != nil {
					return err
				}
				fmt.Println(height)
				return nil
			}

			switch args[0] {
			case "backward":
				first, err := idxer.FirstIndexedBlock()
				if err != nil {
					return err
				}
				if first == -1 {
					// start from the latest block if indexer db is empty
					first = blockStore.Height()
				}
				for i := first - 1; i > 0; i-- {
					if err := indexBlock(i); err != nil {
						return err
					}
				}
			case "forward":
				latest, err := idxer.LastIndexedBlock()
				if err != nil {
					return err
				}
				if latest == -1 {
					// start from genesis if empty
					latest = 0
				}
				for i := latest + 1; i <= blockStore.Height(); i++ {
					if err := indexBlock(i); err != nil {
						return err
					}
				}
			case directionRange:
				start, err := strconv.ParseInt(args[1], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid start block: %w", err)
				}
				end, err := strconv.ParseInt(args[2], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid end block: %w", err)
				}
				if start <= 0 || end < start {
					return fmt.Errorf("invalid range: start must be > 0 and end >= start, got %d-%d", start, end)
				}
				if end > blockStore.Height() {
					return fmt.Errorf("end block %d exceeds current chain height %d", end, blockStore.Height())
				}
				for i := start; i <= end; i++ {
					if err := indexBlock(i); err != nil {
						return err
					}
				}
			default:
				return fmt.Errorf("unknown direction %s", args[0])
			}

			return nil
		},
	}
	return cmd
}
