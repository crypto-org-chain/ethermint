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
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/log/v2"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/evmos/ethermint/rpc/stream"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Resend accepts an existing transaction and a new gas price and limit. It will remove
// the given transaction from the pool and reinsert it with the new gas price and limit.
func (b *Backend) Resend(args evmtypes.TransactionArgs, gasPrice *hexutil.Big, gasLimit *hexutil.Uint64) (common.Hash, error) {
	if args.Nonce == nil {
		return common.Hash{}, fmt.Errorf("missing transaction nonce in transaction spec")
	}

	args, err := b.SetTxDefaults(args)
	if err != nil {
		return common.Hash{}, err
	}

	// The signer used should always be the 'latest' known one because we expect
	// signers to be backwards-compatible with old transactions.
	eip155ChainID, err := ethermint.ParseChainID(b.clientCtx.ChainID)
	if err != nil {
		return common.Hash{}, err
	}

	cfg := b.ChainConfig()
	if cfg == nil {
		cfg = evmtypes.DefaultChainConfig().EthereumConfig(eip155ChainID)
	}

	signer := ethtypes.LatestSigner(cfg)

	matchTx := args.ToTransaction().AsTransaction()

	// Before replacing the old transaction, ensure the _new_ transaction fee is reasonable.
	price := matchTx.GasPrice()
	if gasPrice != nil {
		price = gasPrice.ToInt()
	}
	gas := matchTx.Gas()
	if gasLimit != nil {
		gas = uint64(*gasLimit)
	}
	if err := rpctypes.CheckTxFee(price, gas, b.RPCTxFeeCap()); err != nil {
		return common.Hash{}, err
	}

	pending, err := b.PendingTransactions()
	if err != nil {
		return common.Hash{}, err
	}

	for _, tx := range pending {
		// FIXME does Resend api possible at all?  https://github.com/evmos/ethermint/issues/905
		p, err := evmtypes.UnwrapEthereumMsg(tx, common.Hash{})
		if err != nil {
			// not valid ethereum tx
			continue
		}

		pTx := p.AsTransaction()

		wantSigHash := signer.Hash(matchTx)
		pFrom, err := ethtypes.Sender(signer, pTx)
		if err != nil {
			continue
		}

		if pFrom == *args.From && signer.Hash(pTx) == wantSigHash {
			// Match. Re-sign and send the transaction.
			if gasPrice != nil && (*big.Int)(gasPrice).Sign() != 0 {
				args.GasPrice = gasPrice
			}
			if gasLimit != nil && *gasLimit != 0 {
				args.Gas = gasLimit
			}

			return b.SendTransaction(args) // TODO: this calls SetTxDefaults again, refactor to avoid calling it twice
		}
	}

	return common.Hash{}, fmt.Errorf("transaction %#x not found", matchTx.Hash())
}

// broadcastTx routes txBytes to the app mempool client when one is set.
// InsertTx returning (nil, nil) declines — falls back to CometBFT BroadcastTx.
func (b *Backend) broadcastTx(txBytes []byte) (*sdk.TxResponse, error) {
	if b.mempoolClient != nil {
		if rsp, err := b.mempoolClient.InsertTx(txBytes); rsp != nil || err != nil {
			return rsp, err
		}
	}
	syncCtx := b.clientCtx.WithBroadcastMode(flags.BroadcastSync)
	return syncCtx.BroadcastTx(txBytes)
}

// SendRawTransaction send a raw Ethereum transaction.
//
// Note: unlike geth and cosmos-evm (which queue future nonces), CometBFT's mempool
// requires the exact next nonce — future nonces are rejected. Clients must submit
// transactions in strict nonce order.
func (b *Backend) SendRawTransaction(data hexutil.Bytes) (common.Hash, error) {
	// RLP decode raw transaction bytes
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(data); err != nil {
		b.logger.Error("transaction decoding failed", "error", err.Error())
		return common.Hash{}, err
	}
	return b.submitTransaction(&tx)
}

// submitTransaction validates and broadcasts a decoded tx. It returns a zero hash only
// when the tx is rejected before broadcast.
func (b *Backend) submitTransaction(tx *ethtypes.Transaction) (common.Hash, error) {
	// Ensure the transaction fee is reasonable — matches geth's SubmitTransaction.
	if err := rpctypes.CheckTxFee(tx.GasPrice(), tx.Gas(), b.RPCTxFeeCap()); err != nil {
		return common.Hash{}, err
	}

	// check the local node config in case unprotected txs are disabled
	if !b.UnprotectedAllowed() && !tx.Protected() {
		// Ensure only eip155 signed transactions are submitted if EIP155Required is set.
		return common.Hash{}, errors.New("only replay-protected (EIP-155) transactions allowed over RPC")
	}

	// Reject transactions targeting a different chain.
	if tx.Protected() && tx.ChainId().Cmp(b.chainID) != 0 {
		return common.Hash{}, fmt.Errorf("incorrect chain-id; expected %d, got %d", b.chainID, tx.ChainId())
	}

	var ethereumTx evmtypes.MsgEthereumTx
	if err := ethereumTx.FromSignedEthereumTx(tx, ethtypes.LatestSignerForChainID(b.chainID)); err != nil {
		b.logger.Error("transaction converting failed", "error", err.Error())
		return common.Hash{}, fmt.Errorf("failed to convert ethereum transaction: %w", err)
	}

	if err := ethereumTx.ValidateBasic(); err != nil {
		b.logger.Debug("tx failed basic validation", "error", err.Error())
		return common.Hash{}, fmt.Errorf("failed to validate transaction: %w", err)
	}

	// Query params to use the EVM denomination
	res, err := b.queryClient.Params(b.ctx, &evmtypes.QueryParamsRequest{})
	if err != nil {
		b.logger.Error("failed to query evm params", "error", err.Error())
		return common.Hash{}, fmt.Errorf("failed to query evm params: %w", err)
	}

	cosmosTx, err := ethereumTx.BuildTx(b.clientCtx.TxConfig.NewTxBuilder(), res.Params.EvmDenom)
	if err != nil {
		b.logger.Error("failed to build cosmos tx", "error", err.Error())
		return common.Hash{}, fmt.Errorf("failed to build cosmos tx: %w", err)
	}

	// Encode transaction by default Tx encoder
	txBytes, err := b.clientCtx.TxConfig.TxEncoder()(cosmosTx)
	if err != nil {
		b.logger.Error("failed to encode eth tx using default encoder", "error", err.Error())
		return common.Hash{}, fmt.Errorf("failed to encode cosmos tx: %w", err)
	}

	txHash := tx.Hash()

	rsp, err := b.broadcastTx(txBytes)
	if rsp != nil && rsp.Code != 0 {
		err = errorsmod.ABCIError(rsp.Codespace, rsp.Code, rsp.RawLog)
	}
	if err != nil {
		b.logger.Error("failed to broadcast tx", "error", err.Error())
		return txHash, err
	}

	// Log submitted transaction for manual investigation (mirrors geth's SubmitTransaction).
	from := common.BytesToAddress(ethereumTx.GetFrom())
	if tx.To() == nil {
		b.logger.Info("Submitted contract creation", "hash", txHash.Hex(), "from", from, "nonce", tx.Nonce())
	} else {
		b.logger.Info("Submitted transaction", "hash", txHash.Hex(), "from", from, "nonce", tx.Nonce(), "recipient", tx.To())
	}

	return txHash, nil
}

const (
	_receiptPollInterval = 200 * time.Millisecond
	// Indexers run behind the new-block event; the EVM indexer's retries stop once it catches up.
	_indexLagRetryDelay   = 100 * time.Millisecond
	_cometIndexLagRetries = 3
	_evmIndexLagRetries   = 20
	// Caps a wait for the next block, so a dropped header or a late indexer still gets polled.
	_headerStallPollInterval = time.Second
	// Without this margin the RPC server's request timeout replies first, without the tx hash.
	_receiptWaitResponseMargin = 250 * time.Millisecond
	// How long a lookup may run past the wait deadline; it must stay inside the margin.
	_receiptLookupGrace = 100 * time.Millisecond
	// The broadcast cannot be bounded, so with less request time left the tx is not sent:
	// a slow CheckTx could otherwise let the server's timeout reply first, without the hash.
	_minRequestBudget = time.Second
)

// MinSyncHTTPTimeout is the shortest json-rpc http-timeout under which eth_sendRawTransactionSync
// can run; geth's request timer fires 100ms before the HTTP write timeout.
const MinSyncHTTPTimeout = _minRequestBudget + _receiptWaitResponseMargin + 100*time.Millisecond

// SendRawTransactionSync submits a raw transaction and waits for its receipt (EIP-7966).
// A tx that passes CheckTx but fails ante in the block is never indexed, so it times out.
func (b *Backend) SendRawTransactionSync(
	ctx context.Context,
	data hexutil.Bytes,
	timeoutMs *uint64,
) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	hash := tx.Hash()

	// The RPC request timer is already running, so this budget must cover the broadcast too.
	budget, bounded := receiptRequestBudget(ctx)
	if bounded && budget < _minRequestBudget {
		return nil, errors.New("request timeout leaves no time to wait for the receipt; the transaction was not submitted")
	}
	replyBy := time.Now().Add(budget)

	poller, release, err := b.acquireReceiptWatch(hash)
	if err != nil {
		return nil, err
	}
	defer release()

	// Created before broadcast so the block that includes the tx cannot be missed.
	waiter := b.newReceiptWaiter()
	sentHash, err := b.submitTransaction(&tx)
	submittedAt := time.Now()
	known := errors.Is(err, errortypes.ErrTxInMempoolCache)

	// Like geth, the client's timeout starts once the tx is submitted.
	timeout := b.receiptWaitTimeout(timeoutMs)
	if bounded {
		timeout = max(min(timeout, time.Until(replyBy)), 0)
	}
	receiptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	lookupCtx, cancelLookup := context.WithTimeout(ctx, timeout+_receiptLookupGrace)
	defer cancelLookup()

	// A fresh broadcast cannot be in a block yet. A failed one may be an included tx
	// (evicted from the cache) and a cache hit a resend, so both look the receipt up first.
	if err != nil {
		// A zero hash means the tx was never broadcast.
		if sentHash == (common.Hash{}) {
			return nil, err
		}
		// Another waiter's lookup may predate the inclusion that made this broadcast fail.
		if receipt := poller.poll(lookupCtx, submittedAt); receipt != nil {
			return receipt, nil
		}
		if !known {
			if includedErr := poller.includedErr(); includedErr != nil {
				return nil, includedErr
			}
			return nil, err
		}
	}

	for waiter.wait(receiptCtx) {
		if receipt := poller.poll(lookupCtx, waiter.wokeAt); receipt != nil {
			return receipt, nil
		}
	}
	if !errors.Is(receiptCtx.Err(), context.DeadlineExceeded) {
		return nil, receiptCtx.Err()
	}
	if receipt := poller.poll(lookupCtx, waiter.wokeAt); receipt != nil {
		return receipt, nil
	}
	// The timeout error would wrongly say the tx is still pending if it is indexed.
	if includedErr := poller.includedErr(); includedErr != nil {
		return nil, includedErr
	}
	timeoutErr := rpctypes.NewReceiptWaitTimeoutError(hash, timeout)
	// With keep-invalid-txs-in-cache a cache hit may be a tx CheckTx rejected.
	timeoutErr.AlreadyKnown = known
	return nil, timeoutErr
}

// _maxWaitsPerTx bounds the waiter slots one tx can hold, so resending it from many
// connections cannot fill max-receipt-waiters.
const _maxWaitsPerTx = 4

// receiptWatch is the receipt lookup shared by all waits on one tx.
type receiptWatch struct {
	poller *receiptPoller
	refs   int
	// held is the slot of the last waiter to leave while a lookup still runs; the watch
	// stays registered so a resend reuses that lookup and takes over the slot.
	held *heldSlot
}

type heldSlot struct{ free func() }

type receiptWatches struct {
	mu     sync.Mutex
	byHash map[common.Hash]*receiptWatch
}

func (b *Backend) acquireReceiptWatch(hash common.Hash) (*receiptPoller, func(), error) {
	ws := b.receiptWatches
	ws.mu.Lock()
	defer ws.mu.Unlock()
	w, ok := ws.byHash[hash]
	// The per-tx cap only protects max-receipt-waiters, so it is off along with it.
	if ok && b.receiptWaiters != nil && w.refs >= _maxWaitsPerTx {
		return nil, nil, &rpctypes.ReceiptWaitersLimitError{Limit: _maxWaitsPerTx, PerTx: true}
	}
	var free func()
	if ok && w.held != nil {
		free, w.held = w.held.free, nil
	} else {
		var err error
		if free, err = b.acquireReceiptWaiter(); err != nil {
			return nil, nil, err
		}
	}
	if !ok {
		w = &receiptWatch{poller: &receiptPoller{lookup: b.getTransactionReceiptByIndexer, logger: b.logger, hash: hash}}
		ws.byHash[hash] = w
	}
	w.refs++
	return w.poller, func() { b.releaseReceiptWatch(hash, w, free) }, nil
}

func (b *Backend) releaseReceiptWatch(hash common.Hash, w *receiptWatch, free func()) {
	ws := b.receiptWatches
	ws.mu.Lock()
	w.refs--
	if w.refs > 0 {
		ws.mu.Unlock()
		free()
		return
	}
	// Lookups cannot be canceled, so the slot stays held until the running one ends. The hold
	// counts from the lookup's start so resends taking the slot over cannot extend it forever.
	running := w.poller.running()
	var hold time.Duration
	if running != nil {
		hold = 2*b.cfg.JSONRPC.ReceiptWaitMaxTimeout - time.Since(running.started)
	}
	if hold <= 0 {
		delete(ws.byHash, hash)
		ws.mu.Unlock()
		free()
		return
	}
	held := &heldSlot{free: free}
	w.held = held
	ws.mu.Unlock()

	go func() {
		timer := time.NewTimer(hold)
		defer timer.Stop()
		select {
		case <-running.done:
		case <-timer.C:
		}
		ws.mu.Lock()
		if w.held != held {
			// A resend took the slot over.
			ws.mu.Unlock()
			return
		}
		w.held = nil
		if w.refs == 0 && ws.byHash[hash] == w {
			delete(ws.byHash, hash)
		}
		ws.mu.Unlock()
		held.free()
	}()
}

func (b *Backend) acquireReceiptWaiter() (release func(), err error) {
	if b.receiptWaiters == nil {
		return func() {}, nil
	}
	select {
	case b.receiptWaiters <- struct{}{}:
		return func() { <-b.receiptWaiters }, nil
	default:
		return nil, &rpctypes.ReceiptWaitersLimitError{Limit: cap(b.receiptWaiters)}
	}
}

type receiptResult struct {
	receipt map[string]interface{}
	// err comes only from building the receipt of a tx found in the index.
	err error
}

type receiptLookup struct {
	started time.Time
	done    chan struct{}
	res     receiptResult
}

// receiptPoller looks up one tx's receipt for all its waiters. Lookups cannot be canceled,
// so one still running when its context ends is reused by the next poll and holds the
// waiter slot until it ends.
type receiptPoller struct {
	lookup func(common.Hash) (map[string]interface{}, error)
	logger log.Logger
	hash   common.Hash

	mu      sync.Mutex
	pending *receiptLookup
	// buildErr is the latest lookup's error; it may be transient, so lookups keep going.
	buildErr error
}

func (p *receiptPoller) includedErr() error {
	p.mu.Lock()
	buildErr := p.buildErr
	p.mu.Unlock()
	if buildErr == nil {
		return nil
	}
	return fmt.Errorf("transaction %s was included but its receipt could not be built: %w", p.hash.Hex(), buildErr)
}

// poll returns the receipt, or nil on a miss. A shared lookup that started before notBefore
// may have missed the tx, so its miss is followed by a fresh lookup.
func (p *receiptPoller) poll(ctx context.Context, notBefore time.Time) map[string]interface{} {
	for {
		p.mu.Lock()
		call := p.pending
		if call == nil {
			if ctx.Err() != nil {
				p.mu.Unlock()
				return nil
			}
			call = &receiptLookup{started: time.Now(), done: make(chan struct{})}
			p.pending = call
			go p.run(call)
		}
		p.mu.Unlock()

		select {
		case <-call.done:
		case <-ctx.Done():
			// select picks at random when both are ready; a finished lookup's receipt still counts.
			select {
			case <-call.done:
			default:
				return nil
			}
		}
		p.mu.Lock()
		if p.pending == call {
			p.pending = nil
			p.buildErr = call.res.err
		}
		p.mu.Unlock()
		if call.res.receipt != nil || !call.started.Before(notBefore) {
			return call.res.receipt
		}
	}
}

func (p *receiptPoller) run(call *receiptLookup) {
	// Off geth's handler goroutine, so its panic recovery no longer applies. A panic
	// says nothing about inclusion, so it counts as a miss rather than a build error.
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("eth_sendRawTransactionSync: receipt lookup panicked", "hash", p.hash.Hex(), "panic", r)
			call.res = receiptResult{}
		}
		close(call.done)
	}()
	call.res.receipt, call.res.err = p.lookup(p.hash)
}

// running returns the lookup in flight, or nil if none is.
func (p *receiptPoller) running() *receiptLookup {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		return nil
	}
	select {
	case <-p.pending.done:
		return nil
	default:
		return p.pending
	}
}

// receiptWaiter paces receipt lookups to new blocks instead of a fixed interval.
type receiptWaiter struct {
	indexer ethermint.EVMTxIndexer
	headers *stream.Stream[stream.RPCHeader]
	offset  int
	height  int64
	retries int
	// EVM indexer height just before the last lookup; reading it later could miss a
	// catch-up that happened after that lookup missed.
	indexed int64
	// When the last wait ended; a lookup must start after it for indexed to hold.
	wokeAt time.Time
}

func (b *Backend) newReceiptWaiter() *receiptWaiter {
	w := &receiptWaiter{indexer: b.indexer}
	if b.headerStream == nil {
		return w
	}
	headers, err := b.headerStream()
	if err != nil {
		// Handled by polling instead; the stream logs each failed subscribe attempt.
		return w
	}
	w.headers = headers
	_, w.offset = w.headers.ReadNonBlocking(-1)
	return w
}

func (w *receiptWaiter) wait(ctx context.Context) bool {
	if w.headers == nil {
		pollCtx, cancel := context.WithTimeout(ctx, _receiptPollInterval)
		defer cancel()
		<-pollCtx.Done()
		w.wokeAt = time.Now()
		return ctx.Err() == nil
	}
	delay := _headerStallPollInterval
	if w.retryIndexLag() {
		delay = _indexLagRetryDelay
	}
	waitCtx, cancel := context.WithTimeout(ctx, delay)
	defer cancel()
	// Blocks without EVM txs cannot hold the tx, so their headers don't end the wait.
	for {
		items, next := w.headers.ReadBlocking(waitCtx, w.offset)
		if len(items) == 0 {
			break
		}
		w.offset = next
		if w.armIndexLagRetries(items) {
			break
		}
	}
	w.recordIndexed()
	w.wokeAt = time.Now()
	return ctx.Err() == nil
}

// armIndexLagRetries reports whether items include a block with EVM txs, and starts lag
// retries for it; the KV indexer's last height only advances on such blocks.
func (w *receiptWaiter) armIndexLagRetries(items []stream.RPCHeader) bool {
	found := false
	for _, item := range items {
		h := item.EthHeader
		if h == nil || h.Number == nil || h.TxHash == ethtypes.EmptyRootHash {
			continue
		}
		found = true
		w.height = h.Number.Int64()
		w.retries = _cometIndexLagRetries
		if w.indexer != nil {
			w.retries = _evmIndexLagRetries
		}
	}
	return found
}

func (w *receiptWaiter) recordIndexed() {
	if w.indexer == nil || w.retries == 0 {
		return
	}
	last, err := w.indexer.LastIndexedBlock()
	if err != nil {
		// An erroring indexer is not making progress, so retrying would only spin until the next block.
		last = math.MaxInt64
	}
	w.indexed = last
}

// retryIndexLag reports whether to look the receipt up again shortly, rather than
// wait for the next block, because the last lookup may have run before the block was indexed.
func (w *receiptWaiter) retryIndexLag() bool {
	if w.retries == 0 || (w.indexer != nil && w.indexed >= w.height) {
		return false
	}
	w.retries--
	return true
}

func (b *Backend) receiptWaitTimeout(timeoutMs *uint64) time.Duration {
	maxTimeout := b.cfg.JSONRPC.ReceiptWaitMaxTimeout
	timeout := min(b.cfg.JSONRPC.ReceiptWaitTimeout, maxTimeout)
	if timeoutMs != nil && *timeoutMs > 0 {
		timeout = maxTimeout
		// Compared in ms so huge client values cannot overflow time.Duration.
		if ms, err := ethermint.SafeInt64(*timeoutMs); err == nil && ms < maxTimeout.Milliseconds() {
			timeout = time.Duration(ms) * time.Millisecond
		}
	}
	return timeout
}

// receiptRequestBudget returns how long the call may run and still reply before the RPC
// request timeout, and false when the request has no timeout.
func receiptRequestBudget(ctx context.Context) (time.Duration, bool) {
	reqTimeout, ok := rpc.ContextRequestTimeout(ctx)
	if !ok {
		return 0, false
	}
	return reqTimeout - _receiptWaitResponseMargin, true
}

// SetTxDefaults populates tx message with default values in case they are not
// provided on the args
func (b *Backend) SetTxDefaults(args evmtypes.TransactionArgs) (evmtypes.TransactionArgs, error) {
	if args.GasPrice != nil && (args.MaxFeePerGas != nil || args.MaxPriorityFeePerGas != nil) {
		return args, errors.New("both gasPrice and (maxFeePerGas or maxPriorityFeePerGas) specified")
	}

	head, err := b.CurrentHeader()
	if err != nil {
		return args, err
	}
	if head == nil {
		return args, errors.New("latest header is nil")
	}

	// If user specifies both maxPriorityfee and maxFee, then we do not
	// need to consult the chain for defaults. It's definitely a London tx.
	if args.MaxPriorityFeePerGas == nil || args.MaxFeePerGas == nil {
		// In this clause, user left some fields unspecified.
		if head.BaseFee != nil && args.GasPrice == nil {
			if args.MaxPriorityFeePerGas == nil {
				tip, err := b.SuggestGasTipCap(head.BaseFee)
				if err != nil {
					return args, err
				}
				args.MaxPriorityFeePerGas = (*hexutil.Big)(tip)
			}

			if args.MaxFeePerGas == nil {
				gasFeeCap := new(big.Int).Add(
					(*big.Int)(args.MaxPriorityFeePerGas),
					new(big.Int).Mul(head.BaseFee, big.NewInt(2)),
				)
				args.MaxFeePerGas = (*hexutil.Big)(gasFeeCap)
			}

			if args.MaxFeePerGas.ToInt().Cmp(args.MaxPriorityFeePerGas.ToInt()) < 0 {
				return args, fmt.Errorf("maxFeePerGas (%v) < maxPriorityFeePerGas (%v)", args.MaxFeePerGas, args.MaxPriorityFeePerGas)
			}
		} else {
			if args.MaxFeePerGas != nil || args.MaxPriorityFeePerGas != nil {
				return args, errors.New("maxFeePerGas or maxPriorityFeePerGas specified but london is not active yet")
			}

			if args.GasPrice == nil {
				price, err := b.SuggestGasTipCap(head.BaseFee)
				if err != nil {
					return args, err
				}
				if head.BaseFee != nil {
					// The legacy tx gas price suggestion should not add 2x base fee
					// because all fees are consumed, so it would result in a spiral
					// upwards.
					price.Add(price, head.BaseFee)
				}
				args.GasPrice = (*hexutil.Big)(price)
			}
		}
	} else {
		// Both maxPriorityfee and maxFee set by caller. Sanity-check their internal relation
		if args.MaxFeePerGas.ToInt().Cmp(args.MaxPriorityFeePerGas.ToInt()) < 0 {
			return args, fmt.Errorf("maxFeePerGas (%v) < maxPriorityFeePerGas (%v)", args.MaxFeePerGas, args.MaxPriorityFeePerGas)
		}
	}

	if args.Value == nil {
		args.Value = new(hexutil.Big)
	}
	if args.Nonce == nil {
		if args.From == nil {
			return args, errors.New("missing from in argument")
		}
		// get the nonce from the account retriever
		// ignore error in case tge account doesn't exist yet
		nonce, _ := b.getAccountNonce(*args.From, true, 0, b.logger)
		args.Nonce = (*hexutil.Uint64)(&nonce)
	}

	if args.Data != nil && args.Input != nil && !bytes.Equal(*args.Data, *args.Input) {
		return args, errors.New("both 'data' and 'input' are set and not equal. Please use 'input' to pass transaction call data")
	}

	if args.To == nil {
		// Contract creation
		var input []byte
		if args.Data != nil {
			input = *args.Data
		} else if args.Input != nil {
			input = *args.Input
		}

		if len(input) == 0 {
			return args, errors.New("contract creation without any data provided")
		}
	}

	if args.Gas == nil {
		// For backwards-compatibility reason, we try both input and data
		// but input is preferred.
		input := args.Input
		if input == nil {
			input = args.Data
		}

		callArgs := evmtypes.TransactionArgs{
			From:                 args.From,
			To:                   args.To,
			Gas:                  args.Gas,
			GasPrice:             args.GasPrice,
			MaxFeePerGas:         args.MaxFeePerGas,
			MaxPriorityFeePerGas: args.MaxPriorityFeePerGas,
			Value:                args.Value,
			Data:                 input,
			AccessList:           args.AccessList,
			ChainID:              args.ChainID,
			Nonce:                args.Nonce,
		}

		blockNr := rpctypes.NewBlockNumber(big.NewInt(0))
		estimated, err := b.EstimateGas(callArgs, &blockNr, nil)
		if err != nil {
			return args, err
		}
		args.Gas = &estimated
		b.logger.Debug("estimate gas usage automatically", "gas", args.Gas)
	}

	if args.ChainID == nil {
		args.ChainID = (*hexutil.Big)(b.chainID)
	}

	return args, nil
}

// handleRevertError returns revert related error.
func (b *Backend) handleRevertError(vmError string, ret []byte) error {
	if len(vmError) > 0 {
		if vmError != vm.ErrExecutionReverted.Error() {
			return status.Error(codes.Internal, vmError)
		}
		return evmtypes.NewExecErrorWithReason(ret)
	}
	return nil
}

// EstimateGas returns an estimate of gas usage for the given smart contract call.
func (b *Backend) EstimateGas(
	args evmtypes.TransactionArgs,
	blockNrOptional *rpctypes.BlockNumber,
	overrides *json.RawMessage,
) (hexutil.Uint64, error) {
	blockNr := rpctypes.EthPendingBlockNumber
	if blockNrOptional != nil {
		blockNr = *blockNrOptional
	}

	bz, err := json.Marshal(&args)
	if err != nil {
		return 0, err
	}

	header, err := b.TendermintHeaderByNumber(blockNr)
	if err != nil {
		// the error message imitates geth behavior
		return 0, errors.New("header not found")
	}

	var bzOverrides []byte
	if overrides != nil {
		bzOverrides = *overrides
	}

	req := evmtypes.EthCallRequest{
		Args:            bz,
		GasCap:          b.RPCGasCap(),
		ProposerAddress: sdk.ConsAddress(header.Header.ProposerAddress),
		ChainId:         b.chainID.Int64(),
		Overrides:       bzOverrides,
	}

	// From ContextWithHeight: if the provided height is 0,
	// it will return an empty context and the gRPC query will use
	// the latest block height for querying.
	res, err := b.queryClient.EstimateGas(rpctypes.ContextWithHeight(blockNr.Int64()), &req)
	if err != nil {
		return 0, err
	}
	if err = b.handleRevertError(res.VmError, res.Ret); err != nil {
		return 0, err
	}
	return hexutil.Uint64(res.Gas), nil
}

// DoCall performs a simulated call operation through the evmtypes. It returns the
// EthCallResponse containing the result of the call or an error if fails.
func (b *Backend) DoCall(
	args evmtypes.TransactionArgs,
	blockNr rpctypes.BlockNumber,
	overrides *json.RawMessage,
) (*evmtypes.EthCallResponse, error) {
	bz, err := json.Marshal(&args)
	if err != nil {
		return nil, err
	}
	header, err := b.TendermintHeaderByNumber(blockNr)
	if err != nil {
		// the error message imitates geth behavior
		return nil, errors.New("header not found")
	}
	var bzOverrides []byte
	if overrides != nil {
		bzOverrides = *overrides
	}

	req := evmtypes.EthCallRequest{
		Args:            bz,
		GasCap:          b.RPCGasCap(),
		ProposerAddress: sdk.ConsAddress(header.Header.ProposerAddress),
		ChainId:         b.chainID.Int64(),
		Overrides:       bzOverrides,
	}

	// From ContextWithHeight: if the provided height is 0,
	// it will return an empty context and the gRPC query will use
	// the latest block height for querying.
	ctx := rpctypes.ContextWithHeight(blockNr.Int64())
	timeout := b.RPCEVMTimeout()

	// Setup context so it may be canceled the call has completed
	// or, in case of unmetered gas, setup a context with a timeout.
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}

	// Make sure the context is canceled when the call has completed
	// this makes sure resources are cleaned up.
	defer cancel()

	res, err := b.queryClient.EthCall(ctx, &req)
	if err != nil {
		return nil, err
	}
	length := len(res.Ret)
	if length > int(b.cfg.JSONRPC.ReturnDataLimit) && b.cfg.JSONRPC.ReturnDataLimit != 0 {
		return nil, fmt.Errorf("call returned result of length %d exceeding limit %d", length, b.cfg.JSONRPC.ReturnDataLimit)
	}

	if err = b.handleRevertError(res.VmError, res.Ret); err != nil {
		return nil, err
	}
	return res, nil
}

// GasPrice returns the current gas price based on Ethermint's gas price oracle.
func (b *Backend) GasPrice() (*hexutil.Big, error) {
	var (
		result *big.Int
		err    error
	)
	head, err := b.CurrentHeader()
	if err != nil {
		return nil, err
	}
	if head.BaseFee != nil {
		result, err = b.SuggestGasTipCap(head.BaseFee)
		if err != nil {
			return nil, err
		}
		result = result.Add(result, head.BaseFee)
	} else {
		result = b.RPCMinGasPrice()
	}

	// return at least GlobalMinGasPrice from FeeMarket module
	minGasPrice, err := b.GlobalMinGasPrice()
	if err != nil {
		return nil, err
	}
	minGasPriceInt := minGasPrice.TruncateInt().BigInt()
	if result.Cmp(minGasPriceInt) < 0 {
		result = minGasPriceInt
	}

	return (*hexutil.Big)(result), nil
}

// CreateAccessListCall performs a simulated call operation through the evmtypes. It returns a
// list of accessed slot and an estimated gas used on the operation or an error if fails.
func (b *Backend) CreateAccessListCall(
	args evmtypes.TransactionArgs,
	blockNr rpctypes.BlockNumber,
	overrides *json.RawMessage,
) (*evmtypes.AccessListResult, error) {
	bz, err := json.Marshal(&args)
	if err != nil {
		return nil, err
	}
	header, err := b.TendermintHeaderByNumber(blockNr)
	if err != nil {
		// the error message imitates geth behavior
		return nil, errors.New("header not found")
	}
	var bzOverrides []byte
	if overrides != nil {
		bzOverrides = *overrides
	}

	req := evmtypes.EthCallRequest{
		Args:            bz,
		GasCap:          b.RPCGasCap(),
		ProposerAddress: sdk.ConsAddress(header.Header.ProposerAddress),
		ChainId:         b.chainID.Int64(),
		Overrides:       bzOverrides,
	}

	// From ContextWithHeight: if the provided height is 0,
	// it will return an empty context and the gRPC query will use
	// the latest block height for querying.
	ctx := rpctypes.ContextWithHeight(blockNr.Int64())
	timeout := b.RPCEVMTimeout()

	// Setup context so it may be canceled the call has completed
	// or, in case of unmetered gas, setup a context with a timeout.
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}

	// Make sure the context is canceled when the call has completed
	// this makes sure resources are cleaned up.
	defer cancel()

	res, err := b.queryClient.CreateAccessList(ctx, &req)
	if err != nil {
		b.logger.Error("error access list call", "err", err)
		return nil, err
	}
	if res == nil {
		return nil, errors.New("result is nil")
	}
	var accessListResult evmtypes.AccessListResult
	if err := json.Unmarshal(res.GetData(), &accessListResult); err != nil {
		return nil, err
	}
	return &accessListResult, nil
}
