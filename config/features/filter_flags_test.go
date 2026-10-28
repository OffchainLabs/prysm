package features

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/urfave/cli/v2"
)

func TestWithoutFlags(t *testing.T) {
	a := &cli.BoolFlag{Name: "a"}
	b := &cli.BoolFlag{Name: "b"}
	c := &cli.BoolFlag{Name: "c"}

	t.Run("no excluded flags", func(t *testing.T) {
		assert.DeepEqual(t, []cli.Flag{a, b, c}, WithoutFlags([]cli.Flag{a, b, c}, nil))
	})

	t.Run("some excluded flags", func(t *testing.T) {
		assert.DeepEqual(t, []cli.Flag{b}, WithoutFlags([]cli.Flag{a, b, c}, []cli.Flag{a, c}))
	})

	t.Run("same name but different flag", func(t *testing.T) {
		other := &cli.BoolFlag{Name: "a"}
		assert.DeepEqual(t, []cli.Flag{a}, WithoutFlags([]cli.Flag{a}, []cli.Flag{other}))
	})
}

func TestSplitAliases(t *testing.T) {
	t.Run("flag without alias", func(t *testing.T) {
		flag := &cli.BoolFlag{Name: "a"}
		withoutAliases, aliases := SplitAliases([]cli.Flag{flag}, []string{"b"})
		assert.DeepEqual(t, []cli.Flag{flag}, withoutAliases)
		assert.Equal(t, 0, len(aliases))
	})

	t.Run("flag without deprecated alias", func(t *testing.T) {
		flag := &cli.BoolFlag{Name: "a", Aliases: []string{"b"}}
		withoutAliases, aliases := SplitAliases([]cli.Flag{flag}, []string{"c"})
		assert.DeepEqual(t, []cli.Flag{flag}, withoutAliases)
		assert.Equal(t, 0, len(aliases))
	})

	t.Run("flag with deprecated aliases", func(t *testing.T) {
		flag := &cli.StringFlag{Name: "a", Aliases: []string{"b", "c", "d"}, Usage: "Usage of a.", Value: "value"}
		withoutAliases, aliases := SplitAliases([]cli.Flag{flag}, []string{"b", "d"})

		assert.DeepEqual(t, []cli.Flag{&cli.StringFlag{Name: "a", Aliases: []string{"c"}, Usage: "Usage of a.", Value: "value"}}, withoutAliases)
		assert.DeepEqual(t, []cli.Flag{
			&cli.StringFlag{Name: "b", Usage: "Deprecated alias of --a.", Value: "value"},
			&cli.StringFlag{Name: "d", Usage: "Deprecated alias of --a.", Value: "value"},
		}, aliases)

		// The original flag is left untouched.
		assert.DeepEqual(t, []string{"b", "c", "d"}, flag.Aliases)
		assert.Equal(t, "Usage of a.", flag.Usage)
	})
}
