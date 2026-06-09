package debug

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/server"
	"github.com/stretchr/testify/require"

	srvflags "github.com/evmos/ethermint/server/flags"
)

func newTestContext(t *testing.T, restrictUserInput bool) (*server.Context, string) {
	t.Helper()
	ctx := server.NewDefaultContext()
	dataDir := t.TempDir()
	ctx.Config.RootDir = dataDir
	ctx.Viper.Set(srvflags.JSONRPCRestrictUserInput, restrictUserInput)
	return ctx, dataDir
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name       string
		restricted bool
		pathFn     func(dataDir string) string
		wantErr    bool
		checkFn    func(t *testing.T, got string)
	}{
		{
			name:       "unrestricted allows path outside data dir",
			restricted: false,
			pathFn:     func(_ string) string { return "/tmp/profile.out" },
		},
		{
			name:       "unrestricted allows path inside data dir",
			restricted: false,
			pathFn:     func(dataDir string) string { return filepath.Join(dataDir, "profile.out") },
		},
		{
			name:       "restricted allows path inside data dir",
			restricted: true,
			pathFn:     func(dataDir string) string { return filepath.Join(dataDir, "profile.out") },
		},
		{
			name:       "restricted allows nested subdir inside data dir",
			restricted: true,
			pathFn:     func(dataDir string) string { return filepath.Join(dataDir, "pprof", "profile.out") },
		},
		{
			name:       "restricted rejects path outside data dir",
			restricted: true,
			pathFn:     func(_ string) string { return "/tmp/profile.out" },
			wantErr:    true,
		},
		{
			name:       "restricted rejects path using data dir as string prefix (traversal bypass)",
			restricted: true,
			pathFn:     func(dataDir string) string { return dataDir + "-evil/profile.out" },
			wantErr:    true,
		},
		{
			name:       "unrestricted expands home directory",
			restricted: false,
			pathFn:     func(_ string) string { return "~/profile.out" },
			checkFn: func(t *testing.T, got string) {
				require.False(t, strings.HasPrefix(got, "~"), "home dir should be expanded, got: %s", got)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, dataDir := newTestContext(t, tc.restricted)
			got, err := validatePath(ctx, tc.pathFn(dataDir))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, got)
			if tc.checkFn != nil {
				tc.checkFn(t, got)
			}
		})
	}
}
