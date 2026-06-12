package main

import (
	"net/http"
	"testing"
)

func TestNewHTTPServerSetsRuntimeLimits(t *testing.T) {
	handler := http.NewServeMux()

	server := newHTTPServer(":8080", handler)

	if server.Addr != ":8080" {
		t.Fatalf("Addr = %q, want :8080", server.Addr)
	}
	if server.Handler != handler {
		t.Fatalf("Handler = %#v, want test handler", server.Handler)
	}
	if server.ReadHeaderTimeout != serverReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", server.ReadHeaderTimeout, serverReadHeaderTimeout)
	}
	if server.ReadTimeout != serverReadTimeout {
		t.Fatalf("ReadTimeout = %s, want %s", server.ReadTimeout, serverReadTimeout)
	}
	if server.WriteTimeout != serverWriteTimeout {
		t.Fatalf("WriteTimeout = %s, want %s", server.WriteTimeout, serverWriteTimeout)
	}
	if server.IdleTimeout != serverIdleTimeout {
		t.Fatalf("IdleTimeout = %s, want %s", server.IdleTimeout, serverIdleTimeout)
	}
	if server.MaxHeaderBytes != serverMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, serverMaxHeaderBytes)
	}
}
