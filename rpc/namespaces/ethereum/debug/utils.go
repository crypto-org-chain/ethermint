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
package debug

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime/pprof"
	"strings"

	"cosmossdk.io/log/v2"
	"github.com/cosmos/cosmos-sdk/server"
	srvflags "github.com/evmos/ethermint/server/flags"
)

// isCPUProfileConfigurationActivated checks if cpuprofile was configured via flag
func isCPUProfileConfigurationActivated(ctx *server.Context) bool {
	// TODO: use same constants as server/start.go
	// constant declared in start.go cannot be imported (cyclical dependency)
	const flagCPUProfile = "cpu-profile"
	if cpuProfile := ctx.Viper.GetString(flagCPUProfile); cpuProfile != "" {
		return true
	}
	return false
}

// ExpandHome expands home directory in file paths.
// ~someuser/tmp will not be expanded.
func ExpandHome(p string) (string, error) {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		usr, err := user.Current()
		if err != nil {
			return p, err
		}
		home := usr.HomeDir
		p = home + p[1:]
	}
	return filepath.Clean(p), nil
}

// validatePath expands home and resolves to an absolute path. When
// restrict-user-input is enabled it also enforces that the resolved path is
// inside the node's data directory and is not a symlink.
func validatePath(ctx *server.Context, file string) (string, error) {
	fp, err := ExpandHome(file)
	if err != nil {
		return "", err
	}
	fp, err = filepath.Abs(fp)
	if err != nil {
		return "", err
	}
	if ctx.Viper.GetBool(srvflags.JSONRPCRestrictUserInput) {
		absDataDir, err := filepath.Abs(ctx.Config.RootDir)
		if err != nil {
			return "", err
		}
		realDataDir, err := filepath.EvalSymlinks(absDataDir)
		if err != nil {
			return "", err
		}
		// Resolve parent dir symlinks; the file itself may not exist yet.
		realParent, err := filepath.EvalSymlinks(filepath.Dir(fp))
		if err != nil {
			return "", err
		}
		fp = filepath.Join(realParent, filepath.Base(fp))
		if !strings.HasPrefix(fp, realDataDir+string(filepath.Separator)) {
			return "", errors.New("file path must be in the data directory")
		}
		// Reject a pre-existing symlink at the final component.
		if fi, err := os.Lstat(fp); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("file path must not be a symlink")
		}
	}
	return fp, nil
}

// restrictedCreate opens fp for writing. In restricted mode it uses O_EXCL to
// prevent overwriting existing files; otherwise it truncates like os.Create.
func restrictedCreate(ctx *server.Context, fp string) (*os.File, error) {
	if ctx.Viper.GetBool(srvflags.JSONRPCRestrictUserInput) {
		return os.OpenFile(fp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
	}
	return os.Create(fp)
}

// writeProfile writes the data to a file
func writeProfile(name, file string, ctx *server.Context, log log.Logger) error {
	p := pprof.Lookup(name)
	log.Info("Writing profile records", "count", p.Count(), "type", name, "dump", file)
	fp, err := validatePath(ctx, file)
	if err != nil {
		return err
	}
	f, err := restrictedCreate(ctx, fp)
	if err != nil {
		return err
	}

	if err := p.WriteTo(f, 0); err != nil {
		if err := f.Close(); err != nil {
			return err
		}
		return err
	}

	return f.Close()
}
