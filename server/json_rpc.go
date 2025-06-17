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
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"golang.org/x/sync/errgroup"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	ethlog "github.com/ethereum/go-ethereum/log"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/evmos/ethermint/app/ante"
	"github.com/evmos/ethermint/rpc"
	"github.com/evmos/ethermint/rpc/stream"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/server/config"
	ethermint "github.com/evmos/ethermint/types"
)

const ServerStartTime = 5 * time.Second

type AppWithPendingTxStream interface {
	RegisterPendingTxListener(listener ante.PendingTxListener)
}

// StartJSONRPC starts the JSON-RPC server
func StartJSONRPC(
	ctx context.Context,
	srvCtx *server.Context,
	clientCtx client.Context,
	g *errgroup.Group,
	config *config.Config,
	indexer ethermint.EVMTxIndexer,
	app AppWithPendingTxStream,
) (*http.Server, error) {
	logger := srvCtx.Logger.With("module", "geth")

	evtClient, ok := clientCtx.Client.(rpcclient.EventsClient)
	if !ok {
		return nil, fmt.Errorf("client %T does not implement EventsClient", clientCtx.Client)
	}

	queryClient := rpctypes.NewQueryClient(clientCtx)
	rpcStream := stream.NewRPCStreams(evtClient, logger, clientCtx.TxConfig.TxDecoder(), queryClient.ValidatorAccount)

	app.RegisterPendingTxListener(rpcStream.ListenPendingTx)

	ethlog.Root().SetHandler(ethlog.FuncHandler(func(r *ethlog.Record) error {
		switch r.Lvl {
		case ethlog.LvlTrace, ethlog.LvlDebug:
			logger.Debug(r.Msg, r.Ctx...)
		case ethlog.LvlInfo, ethlog.LvlWarn:
			logger.Info(r.Msg, r.Ctx...)
		case ethlog.LvlError, ethlog.LvlCrit:
			logger.Error(r.Msg, r.Ctx...)
		}
		return nil
	}))

	rpcServer := ethrpc.NewServer()

	allowUnprotectedTxs := config.JSONRPC.AllowUnprotectedTxs
	rpcAPIArr := config.JSONRPC.API

	apis := rpc.GetRPCAPIs(srvCtx, clientCtx, rpcStream, allowUnprotectedTxs, indexer, rpcAPIArr)

	for _, api := range apis {
		if err := rpcServer.RegisterName(api.Namespace, api.Service); err != nil {
			srvCtx.Logger.Error(
				"failed to register service in JSON RPC namespace",
				"namespace", api.Namespace,
				"service", api.Service,
			)
			return nil, err
		}
	}

	r := mux.NewRouter()
	r.HandleFunc("/", rpcServer.ServeHTTP).Methods("POST")

	handlerWithCors := cors.Default()
	if config.API.EnableUnsafeCORS {
		handlerWithCors = cors.AllowAll()
	}

	httpSrv := &http.Server{
		Addr:              config.JSONRPC.Address,
		Handler:           handlerWithCors.Handler(r),
		ReadHeaderTimeout: config.JSONRPC.HTTPTimeout,
		ReadTimeout:       config.JSONRPC.HTTPTimeout,
		WriteTimeout:      config.JSONRPC.HTTPTimeout,
		IdleTimeout:       config.JSONRPC.HTTPIdleTimeout,
	}
	httpSrvDone := make(chan struct{}, 1)

	ln, err := Listen(httpSrv.Addr, config)
	if err != nil {
		return nil, err
	}

	g.Go(func() error {
		srvCtx.Logger.Info("Starting JSON-RPC server", "address", config.JSONRPC.Address)
		errCh := make(chan error)
		go func() {
			errCh <- httpSrv.Serve(ln)
		}()

		// Start a blocking select to wait for an indication to stop the server or that
		// the server failed to start properly.
		select {
		case <-ctx.Done():
			// The calling process canceled or closed the provided context, so we must
			// gracefully stop the JSON-RPC server.
			logger.Info("stopping JSON-RPC server...", "address", config.JSONRPC.Address)
			if err := httpSrv.Shutdown(context.Background()); err != nil {
				logger.Error("failed to shutdown JSON-RPC server", "error", err.Error())
			}
			return nil

		case err := <-errCh:
			if err == http.ErrServerClosed {
				close(httpSrvDone)
			}

			srvCtx.Logger.Error("failed to start JSON-RPC server", "error", err.Error())
			return err
		}
	})

	srvCtx.Logger.Info("Starting JSON WebSocket server", "address", config.JSONRPC.WsAddress)

	wsSrv := rpc.NewWebsocketsServer(ctx, clientCtx, srvCtx.Logger, rpcStream, config)
	wsSrv.Start()
	return httpSrv, nil
}
