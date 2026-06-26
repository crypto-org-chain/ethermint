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
package rpc

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"

	"github.com/ethereum/go-ethereum/rpc"

	"github.com/evmos/ethermint/appmempool"
	"github.com/evmos/ethermint/rpc/backend"
	"github.com/evmos/ethermint/rpc/namespaces/ethereum/debug"
	"github.com/evmos/ethermint/rpc/namespaces/ethereum/eth"
	"github.com/evmos/ethermint/rpc/namespaces/ethereum/eth/filters"
	"github.com/evmos/ethermint/rpc/namespaces/ethereum/net"
	"github.com/evmos/ethermint/rpc/namespaces/ethereum/personal"
	"github.com/evmos/ethermint/rpc/namespaces/ethereum/txpool"
	"github.com/evmos/ethermint/rpc/namespaces/ethereum/web3"
	"github.com/evmos/ethermint/rpc/stream"
	ethermint "github.com/evmos/ethermint/types"
)

// RPC namespaces and API version
const (
	// Cosmos namespaces

	CosmosNamespace = "cosmos"

	// Ethereum namespaces

	Web3Namespace     = "web3"
	EthNamespace      = "eth"
	PersonalNamespace = "personal"
	NetNamespace      = "net"
	TxPoolNamespace   = "txpool"
	DebugNamespace    = "debug"

	apiVersion = "1.0"
)

// APICreator creates the JSON-RPC API implementations. It is the public
// extension point used by RegisterAPINamespace; its signature is kept stable
// for downstream apps.
type APICreator = func(
	ctx *server.Context,
	clientCtx client.Context,
	stream *stream.RPCStream,
	allowUnprotectedTxs bool,
	indexer ethermint.EVMTxIndexer,
) []rpc.API

// apiCreator is the internal creator that also receives APIOptions, so built-in
// namespaces can wire app-provided backends without package-global state.
type apiCreator = func(
	ctx *server.Context,
	clientCtx client.Context,
	stream *stream.RPCStream,
	allowUnprotectedTxs bool,
	indexer ethermint.EVMTxIndexer,
	opts APIOptions,
) []rpc.API

// apiCreators defines the JSON-RPC API namespaces.
var apiCreators map[string]apiCreator

// APIOptions carries the app-provided mempool capabilities into the backends.
// The zero value keeps the default behavior: CometBFT BroadcastTx submission.
type APIOptions struct {
	// Inserter, when set, submits EVM txs straight to the app mempool.
	Inserter appmempool.Inserter
}

// backendOptions translates the app capabilities into backend constructor options.
func (o APIOptions) backendOptions() []backend.Option {
	if o.Inserter == nil {
		return nil
	}
	return []backend.Option{backend.WithTxInserter(o.Inserter.InsertMempoolTx)}
}

func init() {
	apiCreators = map[string]apiCreator{
		EthNamespace: func(ctx *server.Context,
			clientCtx client.Context,
			stream *stream.RPCStream,
			allowUnprotectedTxs bool,
			indexer ethermint.EVMTxIndexer,
			opts APIOptions,
		) []rpc.API {
			evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, allowUnprotectedTxs, indexer, opts.backendOptions()...)
			return []rpc.API{
				{
					Namespace: EthNamespace,
					Version:   apiVersion,
					Service:   eth.NewPublicAPI(ctx.Logger, evmBackend),
					Public:    true,
				},
				{
					Namespace: EthNamespace,
					Version:   apiVersion,
					Service:   filters.NewPublicAPI(ctx.Logger, clientCtx, stream, evmBackend),
					Public:    true,
				},
			}
		},
		Web3Namespace: func(*server.Context, client.Context, *stream.RPCStream, bool, ethermint.EVMTxIndexer, APIOptions) []rpc.API {
			return []rpc.API{
				{
					Namespace: Web3Namespace,
					Version:   apiVersion,
					Service:   web3.NewPublicAPI(),
					Public:    true,
				},
			}
		},
		NetNamespace: func(_ *server.Context, clientCtx client.Context, _ *stream.RPCStream, _ bool, _ ethermint.EVMTxIndexer, _ APIOptions) []rpc.API {
			return []rpc.API{
				{
					Namespace: NetNamespace,
					Version:   apiVersion,
					Service:   net.NewPublicAPI(clientCtx),
					Public:    true,
				},
			}
		},
		PersonalNamespace: func(ctx *server.Context,
			clientCtx client.Context,
			_ *stream.RPCStream,
			allowUnprotectedTxs bool,
			indexer ethermint.EVMTxIndexer,
			opts APIOptions,
		) []rpc.API {
			evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, allowUnprotectedTxs, indexer, opts.backendOptions()...)
			return []rpc.API{
				{
					Namespace: PersonalNamespace,
					Version:   apiVersion,
					Service:   personal.NewAPI(ctx.Logger, evmBackend),
					Public:    false,
				},
			}
		},
		TxPoolNamespace: func(ctx *server.Context, _ client.Context, _ *stream.RPCStream, _ bool, _ ethermint.EVMTxIndexer, _ APIOptions) []rpc.API {
			return []rpc.API{
				{
					Namespace: TxPoolNamespace,
					Version:   apiVersion,
					Service:   txpool.NewPublicAPI(ctx.Logger),
					Public:    true,
				},
			}
		},
		DebugNamespace: func(ctx *server.Context,
			clientCtx client.Context,
			_ *stream.RPCStream,
			allowUnprotectedTxs bool,
			indexer ethermint.EVMTxIndexer,
			opts APIOptions,
		) []rpc.API {
			evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, allowUnprotectedTxs, indexer, opts.backendOptions()...)
			return []rpc.API{
				{
					Namespace: DebugNamespace,
					Version:   apiVersion,
					Service:   debug.NewAPI(ctx, evmBackend),
					Public:    true,
				},
			}
		},
	}
}

// GetRPCAPIs returns the selected APIs with default backend options.
func GetRPCAPIs(ctx *server.Context,
	clientCtx client.Context,
	stream *stream.RPCStream,
	allowUnprotectedTxs bool,
	indexer ethermint.EVMTxIndexer,
	selectedAPIs []string,
) []rpc.API {
	return GetRPCAPIsWithOptions(ctx, clientCtx, stream, allowUnprotectedTxs, indexer, selectedAPIs, APIOptions{})
}

// GetRPCAPIsWithOptions returns the selected APIs, wiring the app-provided
// backends from opts into the built-in namespaces.
func GetRPCAPIsWithOptions(ctx *server.Context,
	clientCtx client.Context,
	stream *stream.RPCStream,
	allowUnprotectedTxs bool,
	indexer ethermint.EVMTxIndexer,
	selectedAPIs []string,
	opts APIOptions,
) []rpc.API {
	var apis []rpc.API

	for _, ns := range selectedAPIs {
		if creator, ok := apiCreators[ns]; ok {
			apis = append(apis, creator(ctx, clientCtx, stream, allowUnprotectedTxs, indexer, opts)...)
		} else {
			ctx.Logger.Error("invalid namespace value", "namespace", ns)
		}
	}

	return apis
}

// RegisterAPINamespace registers a new API namespace with the API creator.
// This function fails if the namespace is already registered.
func RegisterAPINamespace(ns string, creator APICreator) error {
	if _, ok := apiCreators[ns]; ok {
		return fmt.Errorf("duplicated api namespace %s", ns)
	}
	// Custom namespaces don't take APIOptions; ignore them.
	apiCreators[ns] = func(ctx *server.Context,
		clientCtx client.Context,
		stream *stream.RPCStream,
		allowUnprotectedTxs bool,
		indexer ethermint.EVMTxIndexer,
		_ APIOptions,
	) []rpc.API {
		return creator(ctx, clientCtx, stream, allowUnprotectedTxs, indexer)
	}
	return nil
}
