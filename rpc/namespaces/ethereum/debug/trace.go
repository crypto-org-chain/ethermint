// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

//go:build go1.5
// +build go1.5

package debug

import (
	"errors"
	"os"
	"path/filepath"
	"runtime/trace"
	"strings"

	stderrors "github.com/pkg/errors"

	srvflags "github.com/evmos/ethermint/server/flags"
)

// StartGoTrace turns on tracing, writing to the given file.
func (a *API) StartGoTrace(file string) error {
	a.logger.Debug("debug_startGoTrace", "file", file)
	a.handler.mu.Lock()
	defer a.handler.mu.Unlock()

	restrictUserInput := a.ctx.Viper.GetBool(srvflags.JSONRPCRestrictUserInput)

	if a.handler.traceFile != nil {
		a.logger.Debug("trace already in progress")
		return errors.New("trace already in progress")
	}
	var err error
	file, err = ExpandHome(file)
	if err != nil {
		a.logger.Debug("failed to get filepath for the CPU profile file", "error", err.Error())
		return err
	}

	file, err = filepath.Abs(file)
	if err != nil {
		a.logger.Debug("failed to get absolute path for the CPU profile file", "error", err.Error())
		return err
	}

	if restrictUserInput {
		// Ensure that the trace file is in the data directory.
		absDataDir, err := filepath.Abs(a.ctx.Config.RootDir)
		if err != nil {
			a.logger.Debug("failed to get absolute path for the data directory", "error", err.Error())
			return err
		}

		if !strings.HasPrefix(file, absDataDir) {
			a.logger.Debug("trace file must be in the data directory")
			return errors.New("trace file must be in the data directory")
		}
	}

	var f *os.File
	if restrictUserInput {
		// Create the file with O_EXCL to ensure that the file does not exist.
		f, err = os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
	} else {
		f, err = os.Create(file)
	}
	if err != nil {
		a.logger.Debug("failed to create go trace file", "error", err.Error())
		return err
	}
	if err := trace.Start(f); err != nil {
		a.logger.Debug("Go tracing already started", "error", err.Error())
		if err := f.Close(); err != nil {
			a.logger.Debug("failed to close trace file")
			return stderrors.Wrap(err, "failed to close trace file")
		}

		return err
	}
	a.handler.traceFile = f
	a.handler.traceFilename = file
	a.logger.Info("Go tracing started", "dump", a.handler.traceFilename)
	return nil
}

// StopGoTrace stops an ongoing trace.
func (a *API) StopGoTrace() error {
	a.logger.Debug("debug_stopGoTrace")
	a.handler.mu.Lock()
	defer a.handler.mu.Unlock()

	trace.Stop()
	if a.handler.traceFile == nil {
		a.logger.Debug("trace not in progress")
		return errors.New("trace not in progress")
	}
	a.logger.Info("Done writing Go trace", "dump", a.handler.traceFilename)
	if err := a.handler.traceFile.Close(); err != nil {
		a.logger.Debug("failed to close trace file")
		return stderrors.Wrap(err, "failed to close trace file")
	}
	a.handler.traceFile = nil
	a.handler.traceFilename = ""
	return nil
}
