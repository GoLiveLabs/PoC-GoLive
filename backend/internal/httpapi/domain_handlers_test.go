package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/ingest"
	"live-orchestrator/backend/internal/liveid"
	"live-orchestrator/backend/internal/pagination"
	"live-orchestrator/backend/internal/streamplatform"
	"live-orchestrator/backend/internal/testdb"
)

func authedRequest(method, target string, body []byte) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	r.Header.Set("X-Api-Token", testToken)
	return r
}

// --- clients ---

func TestCreateClient_Success(t *testing.T) {
	now := time.Now()
	fake := &fakeClientService{client: &client.Client{ID: uuid.New(), Name: "Acme", CreatedAt: now, UpdatedAt: now}}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodPost, "/api/v1/clients", []byte(`{"name":"Acme"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateClient_DuplicateName_409(t *testing.T) {
	fake := &fakeClientService{err: client.ErrDuplicateName}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodPost, "/api/v1/clients", []byte(`{"name":"Acme"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestCreateClient_InvalidName_422(t *testing.T) {
	fake := &fakeClientService{err: client.ErrInvalidName}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodPost, "/api/v1/clients", []byte(`{"name":""}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if _, ok := body["errors"]; !ok {
		t.Fatalf("expected an \"errors\" field map in a 422 response, got %v", body)
	}
}

func TestGetClient_NotFound_404(t *testing.T) {
	fake := &fakeClientService{err: client.ErrNotFound}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodGet, "/api/v1/clients/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetClient_InvalidUUID_400(t *testing.T) {
	fake := &fakeClientService{}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodGet, "/api/v1/clients/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListClients_PassesThroughPage(t *testing.T) {
	page := pagination.Page[client.Response]{Data: []client.Response{{Name: "Acme"}}, HasMore: false}
	fake := &fakeClientService{page: page}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodGet, "/api/v1/clients?limit=10", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got pagination.Page[client.Response]
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Data) != 1 || got.Data[0].Name != "Acme" {
		t.Fatalf("unexpected page: %+v", got)
	}
}

func TestUpdateClient_NullEmail_ClearsField(t *testing.T) {
	fake := &fakeClientService{client: &client.Client{ID: uuid.New(), Name: "Acme"}}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodPatch, "/api/v1/clients/"+uuid.New().String(), []byte(`{"email":null}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateClient_MalformedBody_400(t *testing.T) {
	fake := &fakeClientService{}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodPatch, "/api/v1/clients/"+uuid.New().String(), []byte(`{not-json`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteClient_204(t *testing.T) {
	fake := &fakeClientService{}
	srv := newDomainTestServer(fake, nil, nil, nil)

	req := authedRequest(http.MethodDelete, "/api/v1/clients/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

// --- ingests ---

func TestCreateIngest_Success(t *testing.T) {
	fake := &fakeIngestService{ingest: &ingest.Ingest{ID: uuid.New(), Protocol: "https"}}
	srv := newDomainTestServer(nil, fake, nil, nil)

	req := authedRequest(http.MethodPost, "/api/v1/clients/"+uuid.New().String()+"/ingests", []byte(`{"url":"https://acme.com/feed"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateIngest_ClientNotFound_404(t *testing.T) {
	fake := &fakeIngestService{err: ingest.ErrClientNotFound}
	srv := newDomainTestServer(nil, fake, nil, nil)

	req := authedRequest(http.MethodPost, "/api/v1/clients/"+uuid.New().String()+"/ingests", []byte(`{"url":"https://acme.com/feed"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCreateIngest_DuplicateURL_409(t *testing.T) {
	fake := &fakeIngestService{err: ingest.ErrDuplicateURL}
	srv := newDomainTestServer(nil, fake, nil, nil)

	req := authedRequest(http.MethodPost, "/api/v1/clients/"+uuid.New().String()+"/ingests", []byte(`{"url":"https://acme.com/feed"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestUpdateIngest_NoFields_400(t *testing.T) {
	fake := &fakeIngestService{err: ingest.ErrURLRequired}
	srv := newDomainTestServer(nil, fake, nil, nil)

	req := authedRequest(http.MethodPatch, "/api/v1/ingests/"+uuid.New().String(), []byte(`{}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteIngest_204(t *testing.T) {
	fake := &fakeIngestService{}
	srv := newDomainTestServer(nil, fake, nil, nil)

	req := authedRequest(http.MethodDelete, "/api/v1/ingests/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

// --- streaming platforms ---

func TestCreatePlatform_Success(t *testing.T) {
	fake := &fakePlatformService{platform: &streamplatform.Platform{ID: uuid.New(), Slug: "youtube"}}
	srv := newDomainTestServer(nil, nil, fake, nil)

	req := authedRequest(http.MethodPost, "/api/v1/streaming-platforms", []byte(`{"slug":"youtube","displayName":"YouTube"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeletePlatform_InUse_409(t *testing.T) {
	fake := &fakePlatformService{err: streamplatform.ErrPlatformInUse}
	srv := newDomainTestServer(nil, nil, fake, nil)

	req := authedRequest(http.MethodDelete, "/api/v1/streaming-platforms/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

// --- live ids ---

func TestCreateLiveID_PlatformNotFound_404(t *testing.T) {
	fake := &fakeLiveIDService{err: liveid.ErrPlatformNotFound}
	srv := newDomainTestServer(nil, nil, nil, fake)

	body := []byte(`{"platformId":"` + uuid.New().String() + `","liveId":"abc"}`)
	req := authedRequest(http.MethodPost, "/api/v1/clients/"+uuid.New().String()+"/live-ids", body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCreateLiveID_Success(t *testing.T) {
	fake := &fakeLiveIDService{liveID: &liveid.ClientLiveID{ID: uuid.New(), LiveID: "abc"}}
	srv := newDomainTestServer(nil, nil, nil, fake)

	body := []byte(`{"platformId":"` + uuid.New().String() + `","liveId":"abc"}`)
	req := authedRequest(http.MethodPost, "/api/v1/clients/"+uuid.New().String()+"/live-ids", body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListLiveIDsFlat_InvalidPlatformIDQuery_400(t *testing.T) {
	fake := &fakeLiveIDService{}
	srv := newDomainTestServer(nil, nil, nil, fake)

	req := authedRequest(http.MethodGet, "/api/v1/live-ids?platformId=not-a-uuid", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteLiveID_204(t *testing.T) {
	fake := &fakeLiveIDService{}
	srv := newDomainTestServer(nil, nil, nil, fake)

	req := authedRequest(http.MethodDelete, "/api/v1/live-ids/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

// IT-029: PATCH streaming-platforms updates ingestUrlTemplate; GET reflects it.
func TestIntegration_PatchPlatform_IngestURLTemplate(t *testing.T) {
	db := testdb.New(t)
	platformSvc := streamplatform.NewService(db)
	srv := newDomainTestServer(nil, nil, platformSvc, nil)
	ctx := context.Background()

	p, err := platformSvc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	body := []byte(`{"ingestUrlTemplate":"rtmp://a.rtmp.youtube.com/live2"}`)
	req := authedRequest(http.MethodPatch, "/api/v1/streaming-platforms/"+p.ID.String(), body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req = authedRequest(http.MethodGet, "/api/v1/streaming-platforms/"+p.ID.String(), nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got streamplatform.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.IngestURLTemplate != "rtmp://a.rtmp.youtube.com/live2" {
		t.Fatalf("expected ingestUrlTemplate updated, got %q", got.IngestURLTemplate)
	}
}

// IT-030: PATCH live-ids streamKey; response is masked, never raw.
func TestIntegration_PatchLiveID_StreamKeyMasked(t *testing.T) {
	db := testdb.New(t)
	clientSvc := client.NewService(db)
	platformSvc := streamplatform.NewService(db)
	liveIDSvc := liveid.NewService(db, clientSvc, platformSvc)
	srv := newDomainTestServer(clientSvc, nil, platformSvc, liveIDSvc)
	ctx := context.Background()

	c, err := clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	p, err := platformSvc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"})
	if err != nil {
		t.Fatalf("create platform: %v", err)
	}
	l, err := liveIDSvc.Create(ctx, c.ID, liveid.CreateRequest{PlatformID: p.ID, LiveID: "abc", StreamKey: "oldkey"})
	if err != nil {
		t.Fatalf("create live id: %v", err)
	}

	const raw = "newkey1234"
	body := []byte(`{"streamKey":"` + raw + `"}`)
	// Existing route is flat PATCH /live-ids/{id} (no nested client path).
	req := authedRequest(http.MethodPatch, "/api/v1/live-ids/"+l.ID.String(), body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got liveid.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StreamKey == raw {
		t.Fatalf("response leaked raw streamKey")
	}
	if got.StreamKey != "****1234" && !strings.HasSuffix(got.StreamKey, "1234") {
		t.Fatalf("expected masked key ending in 1234, got %q", got.StreamKey)
	}
	for _, r := range got.StreamKey[:len(got.StreamKey)-4] {
		if r != '*' {
			t.Fatalf("expected '*' mask prefix, got %q", got.StreamKey)
		}
	}
}
