package namepublish

import (
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestFilterLocalNames(t *testing.T) {
	names := []string{"foo.local", "bar.example.com", "BAZ.LOCAL"}
	require.Equal(t, []string{"foo.local", "BAZ.LOCAL"}, filterLocalNames(names))
}

func TestRecordsMatchLocalName(t *testing.T) {
	publisher := &AvahiPublisher{}
	publisher.desiredNames = []string{"foo.local"}
	publisher.lastIPs = []string{"10.0.0.1", "2001:db8::1"}

	records := publisher.Records(dns.Question{Name: "foo.local.", Qtype: dns.TypeA})
	require.Len(t, records, 1)

	records = publisher.Records(dns.Question{Name: "foo.local.", Qtype: dns.TypeAAAA})
	require.Len(t, records, 1)

	records = publisher.Records(dns.Question{Name: "bar.local.", Qtype: dns.TypeA})
	require.Len(t, records, 0)
}
