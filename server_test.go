package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestRunServerReturnsListenError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Addr: listener.Addr().String()}
	defer server.Close()
	stop := make(chan os.Signal)
	defer close(stop)
	result := make(chan error, 1)
	go func() { result <- runServer(server, stop) }()

	select {
	case err := <-result:
		var networkError *net.OpError
		if !errors.As(err, &networkError) || networkError.Op != "listen" {
			t.Fatalf("expected a listen error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not return after failing to bind the occupied port")
	}
}

func TestRunServerShutsDownOnSignal(t *testing.T) {
	started := make(chan struct{})
	server := &http.Server{
		Addr: "127.0.0.1:0",
		BaseContext: func(net.Listener) context.Context {
			close(started)
			return context.Background()
		},
	}
	defer server.Close()
	stop := make(chan os.Signal, 1)
	defer close(stop)
	result := make(chan error, 1)
	go func() { result <- runServer(server, stop) }()

	select {
	case <-started:
	case err := <-result:
		t.Fatalf("server stopped before receiving a signal: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	stop <- os.Interrupt
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("shutdown failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after receiving a signal")
	}
}

func TestRunServerWaitsForActiveRequest(t *testing.T) {
	address := make(chan string, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	server := &http.Server{
		Addr: "127.0.0.1:0",
		BaseContext: func(listener net.Listener) context.Context {
			address <- listener.Addr().String()
			return context.Background()
		},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			select {
			case <-release:
				_, _ = io.WriteString(w, "finished")
			case <-r.Context().Done():
			}
		}),
	}
	defer server.Close()
	stop := make(chan os.Signal, 1)
	defer close(stop)
	result := make(chan error, 1)
	go func() { result <- runServer(server, stop) }()
	var url string
	select {
	case addr := <-address:
		url = "http://" + addr
	case err := <-result:
		t.Fatalf("server failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	clientResult := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get(url)
		if err == nil {
			defer response.Body.Close()
			var body []byte
			body, err = io.ReadAll(response.Body)
			if err == nil && string(body) != "finished" {
				err = errors.New("active request did not finish")
			}
		}
		clientResult <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach the handler")
	}
	stop <- os.Interrupt
	select {
	case err := <-result:
		t.Fatalf("shutdown returned before the active request finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case err := <-clientResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client did not receive the completed response")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish shutdown")
	}
}
