package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"crm/pkg/config"
)

func TestServeDrainsInFlightRequestsOnCancel(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		io.WriteString(w, "finished")
	})}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serve(ctx, srv, ln) }()

	type result struct {
		body string
		err  error
	}
	response := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			response <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		response <- result{string(b), err}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request never reached the handler")
	}

	cancel()
	select {
	case err := <-served:
		t.Fatalf("serve returned with a request still in flight: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	select {
	case r := <-response:
		if r.err != nil || r.body != "finished" {
			t.Errorf("in-flight response = %q, %v", r.body, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight request never finished")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Errorf("serve = %v, want nil after a clean shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after shutdown")
	}
}

func TestServeReturnsListenerErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()

	if err := serve(context.Background(), &http.Server{}, ln); err == nil {
		t.Fatal("expected an error from a closed listener")
	}
}

func TestNewFailsWhenDatabaseUnreachable(t *testing.T) {
	_, err := New(context.Background(), config.Config{
		DatabaseURL: "postgres://crm:x@127.0.0.1:1/crm?sslmode=disable&connect_timeout=1",
		Port:        "0",
	})
	if err == nil {
		t.Fatal("expected an error when migrations cannot reach the database")
	}
}
