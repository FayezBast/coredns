package test

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadyChecksBothInstancesOfPlugin(t *testing.T) {
	readyAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	corefile := fmt.Sprintf(`first.example.org:0 {
		bind 127.0.0.1
		ready %[1]s {
			monitor continuously
		}
		erratic {
			drop 0
		}
	}
	second.example.org:0 {
		bind 127.0.0.1
		ready %[1]s {
			monitor continuously
		}
		erratic {
			drop 0
		}
	}`, readyAddr)

	instance, udp, _, err := CoreDNSServerAndPorts(corefile)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, err := range instance.ShutdownCallbacks() {
			assert.NoError(t, err)
		}
		assert.NoError(t, instance.Stop())
	})

	client := http.Client{Timeout: time.Second}
	probe := func(wantStatus int, wantBody string) {
		t.Helper()
		response, err := client.Get("http://" + readyAddr + "/ready")
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, wantStatus, response.StatusCode)
		assert.Equal(t, wantBody, string(body))
	}
	query := func(name string, count int) {
		t.Helper()
		client := dns.Client{Timeout: time.Second}
		for range count {
			message := new(dns.Msg).SetQuestion(name, dns.TypeA)
			response, _, err := client.Exchange(message, udp)
			require.NoError(t, err)
			require.Equal(t, dns.RcodeSuccess, response.Rcode)
		}
	}

	// Erratic reports ready after three queries and unready after five.
	probe(http.StatusServiceUnavailable, "erratic")
	query("first.example.org.", 3)
	probe(http.StatusServiceUnavailable, "erratic")
	query("second.example.org.", 3)
	probe(http.StatusOK, "OK")
	query("first.example.org.", 2)
	probe(http.StatusServiceUnavailable, "erratic")
}
