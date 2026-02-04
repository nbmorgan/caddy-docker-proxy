package namepublish

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractDesiredNames(t *testing.T) {
	caddyfile := []byte(`{
	debug
}
example.com, example.org http://bar.example.com:443 {
	respond "ok"
}
(foo) {
	respond "no"
}
@matcher {
	not
}
localhost:8080 {
	respond "ok"
}
:80 {
	respond "skip"
}
`)

	names, err := ExtractDesiredNames(caddyfile)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"example.com",
		"example.org",
		"bar.example.com",
		"localhost",
	}, names)
}
