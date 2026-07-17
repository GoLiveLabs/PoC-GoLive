package mediaserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListActiveStreams_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"itemCount":0,"pageCount":0,"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	streams, err := c.ListActiveStreams(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(streams) != 0 {
		t.Fatalf("expected 0 streams, got %d", len(streams))
	}
}

func TestListActiveStreams_TwoActive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"itemCount":2,"pageCount":1,"items":[
			{"name":"camera1","ready":true},
			{"name":"camera2","ready":true}
		]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	streams, err := c.ListActiveStreams(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(streams))
	}
	if streams[0].Name != "camera1" || streams[1].Name != "camera2" {
		t.Fatalf("unexpected stream names: %+v", streams)
	}
}

func TestListActiveStreams_FiltersNotReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"itemCount":2,"pageCount":1,"items":[
			{"name":"camera1","ready":true},
			{"name":"camera2","ready":false}
		]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	streams, err := c.ListActiveStreams(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(streams) != 1 || streams[0].Name != "camera1" {
		t.Fatalf("expected only camera1 to be returned, got %+v", streams)
	}
}

func TestListActiveStreams_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.ListActiveStreams(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestListActiveStreams_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.httpClient.Timeout = 10 * time.Millisecond

	_, err := c.ListActiveStreams(context.Background())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
