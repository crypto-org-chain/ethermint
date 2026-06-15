package debug

import (
	"os"
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

func TestRestrictedCreate(t *testing.T) {
	tests := []struct {
		name       string
		restricted bool
		preCreate  bool // whether the file already exists
		wantErr    bool
	}{
		{
			name:       "restricted allows creating new file",
			restricted: true,
			preCreate:  false,
		},
		{
			name:       "restricted rejects overwriting existing file",
			restricted: true,
			preCreate:  true,
			wantErr:    true,
		},
		{
			name:       "unrestricted allows overwriting existing file",
			restricted: false,
			preCreate:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, dataDir := newTestContext(t, tc.restricted)
			fp := filepath.Join(dataDir, "profile.out")
			if tc.preCreate {
				f, err := os.Create(fp)
				require.NoError(t, err)
				f.Close()
			}
			f, err := restrictedCreate(ctx, fp)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				f.Close()
			}
		})
	}
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
			pathFn: func(dataDir string) string {
				sub := filepath.Join(dataDir, "pprof")
				_ = os.MkdirAll(sub, 0o700)
				return filepath.Join(sub, "profile.out")
			},
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
			name:       "restricted rejects symlink at the final path component pointing outside",
			restricted: true,
			pathFn: func(dataDir string) string {
				outside := t.TempDir()
				target := filepath.Join(outside, "profile.out")
				link := filepath.Join(dataDir, "profile.out")
				_ = os.Symlink(target, link)
				return link
			},
			wantErr: true,
		},
		{
			name:       "restricted rejects symlink inside data dir pointing outside",
			restricted: true,
			pathFn: func(dataDir string) string {
				outside := filepath.Dir(dataDir)
				link := filepath.Join(dataDir, "link")
				_ = os.Symlink(outside, link)
				return filepath.Join(link, "profile.out")
			},
			wantErr: true,
		},
		{
			name:       "restricted allows symlink inside data dir pointing to subdir within data dir",
			restricted: true,
			pathFn: func(dataDir string) string {
				inner := filepath.Join(dataDir, "inner")
				_ = os.MkdirAll(inner, 0o700)
				link := filepath.Join(dataDir, "link")
				_ = os.Symlink(inner, link)
				return filepath.Join(link, "profile.out")
			},
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
