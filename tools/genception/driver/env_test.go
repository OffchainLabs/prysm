package driver

import (
	"fmt"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestJsonIndexPathFromEnv(t *testing.T) {
	getIdxFile := func(env *environment) string { return env.inventoryIndexPath }
	pkgBase := func(env *environment) string { return env.packagesBase }
	pwd := func(env *environment) string { return env.pwd }
	setAll := map[string]string{
		ENV_JSON_INDEX_PATH: "/path/to/file",
		ENV_PACKAGES_BASE:   "/path/to/base",
		ENV_PWD:             "derp",
	}
	cases := []struct {
		val     string
		err     error
		envname string
		set     map[string]string
		getter  func(*environment) string
	}{
		{
			getter: getIdxFile,
			set: map[string]string{
				ENV_PACKAGES_BASE: "/path/to/base",
				ENV_PWD:           "derp",
			},
			err: errUnsetEnvVar,
		},
		{
			getter: getIdxFile,
			set:    setAll,
			val:    "/path/to/file",
		},
		{
			getter: pkgBase,
			set: map[string]string{
				ENV_JSON_INDEX_PATH: "/path/to/file",
				ENV_PWD:             "derp",
			},
			err: errUnsetEnvVar,
		},
		{
			getter: pkgBase,
			val:    "/path/to/base",
			set:    setAll,
		},
		{
			getter: pwd,
			set: map[string]string{
				ENV_JSON_INDEX_PATH: "/path/to/file",
				ENV_PACKAGES_BASE:   "/path/to/base",
			},
			val: os.Getenv("PWD"), // PWD is a special case because it's ya know THE pwd
		},
		{
			getter: pwd,
			val:    "derp",
			set:    setAll,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			reset := make(map[string]string)
			defer func() {
				for k, v := range reset {
					if v == "" {
						require.NoError(t, os.Unsetenv(k))
					} else {
						t.Setenv(k, v)
					}
				}
			}()
			for k, v := range c.set {
				reset[k] = os.Getenv(k)
				t.Setenv(k, v)
			}
			v, err := loadEnv()
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.val, c.getter(v))
		})
	}
}
