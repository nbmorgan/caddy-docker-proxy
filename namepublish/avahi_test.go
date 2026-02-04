package namepublish

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterLocalNames(t *testing.T) {
	names := []string{"foo.local", "bar.example.com", "BAZ.LOCAL"}
	require.Equal(t, []string{"foo.local", "BAZ.LOCAL"}, filterLocalNames(names))
}
