package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestBuildTestResultsAPIUploadsAndReturnsStableReport(t *testing.T) {
	database, buildID := newTestReportStore(t)
	defer database.Close()
	handler := &handlers{d: Deps{Store: database}}
	xmlReport := `<testsuites><testsuite name="unit"><testcase classname="pkg.Example" name="works" time="0.125"/><testcase classname="pkg.Example" name="fails"><failure message="nope" type="AssertionError">details</failure></testcase></testsuite></testsuites>`

	uploadResponse := httptest.NewRecorder()
	uploadRequest := testReportRequest(http.MethodPost, buildID, bytes.NewBufferString(xmlReport))
	uploadRequest.Header.Set("Content-Type", "application/xml")
	handler.uploadTestResults(uploadResponse, uploadRequest)
	if uploadResponse.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", uploadResponse.Code, uploadResponse.Body.String())
	}

	getResponse := httptest.NewRecorder()
	handler.getBuildTestResults(getResponse, testReportRequest(http.MethodGet, buildID, nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	var payload buildTestResultsResponse
	if err := json.Unmarshal(getResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Summary == nil || payload.Summary.Total != 2 || payload.Summary.Passed != 1 || payload.Summary.Failed != 1 || payload.Summary.Duration != 125 {
		t.Fatalf("summary = %#v", payload.Summary)
	}
	if len(payload.Cases) != 2 || payload.Cases[0].Status != "passed" || payload.Cases[1].Status != "failed" || payload.Cases[1].Details != "details" {
		t.Fatalf("cases = %#v", payload.Cases)
	}
	if len(payload.Results) != 1 || payload.Results[0].ID != payload.Summary.ID {
		t.Fatalf("results = %#v", payload.Results)
	}
	if bytes.Contains(getResponse.Body.Bytes(), []byte("report_xml")) || bytes.Contains(getResponse.Body.Bytes(), []byte(xmlReport)) {
		t.Fatalf("raw XML leaked in response: %s", getResponse.Body.String())
	}
}

func TestBuildTestResultsAPIHasStableEmptyState(t *testing.T) {
	database, buildID := newTestReportStore(t)
	defer database.Close()
	handler := &handlers{d: Deps{Store: database}}

	response := httptest.NewRecorder()
	handler.getBuildTestResults(response, testReportRequest(http.MethodGet, buildID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if string(payload["summary"]) != "null" || string(payload["cases"]) != "[]" || string(payload["results"]) != "[]" {
		t.Fatalf("empty payload = %s", response.Body.String())
	}
}

func TestBuildTestResultsAPIPresentsNewestUploadAndKeepsHistory(t *testing.T) {
	database, buildID := newTestReportStore(t)
	defer database.Close()
	handler := &handlers{d: Deps{Store: database}}

	for _, xmlReport := range []string{
		`<testsuite name="first"><testcase name="old"/></testsuite>`,
		`<testsuite name="second"><testcase name="new"><skipped message="later"/></testcase></testsuite>`,
	} {
		response := httptest.NewRecorder()
		handler.uploadTestResults(response, testReportRequest(http.MethodPost, buildID, bytes.NewBufferString(xmlReport)))
		if response.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	handler.getBuildTestResults(response, testReportRequest(http.MethodGet, buildID, nil))
	var payload buildTestResultsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Summary == nil || payload.Summary.Skipped != 1 || len(payload.Cases) != 1 || payload.Cases[0].Name != "new" {
		t.Fatalf("latest report = %#v %#v", payload.Summary, payload.Cases)
	}
	if len(payload.Results) != 2 || payload.Results[0].ID <= payload.Results[1].ID || payload.Results[0].ID != payload.Summary.ID {
		t.Fatalf("history = %#v", payload.Results)
	}
}

func TestBuildTestResultsAPIRejectsInvalidXML(t *testing.T) {
	database, buildID := newTestReportStore(t)
	defer database.Close()
	handler := &handlers{d: Deps{Store: database}}

	response := httptest.NewRecorder()
	handler.uploadTestResults(response, testReportRequest(http.MethodPost, buildID, bytes.NewBufferString(`{"xml":"not raw XML"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func newTestReportStore(t *testing.T) (*store.Store, int64) {
	t.Helper()
	database, err := store.New(filepath.Join(t.TempDir(), "test-reports.db"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject("reports", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	return database, build.ID
}

func testReportRequest(method string, buildID int64, body *bytes.Buffer) *http.Request {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, "/api/builds/"+strconv.FormatInt(buildID, 10)+"/test-results", nil)
	} else {
		request = httptest.NewRequest(method, "/api/builds/"+strconv.FormatInt(buildID, 10)+"/test-results", body)
	}
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", strconv.FormatInt(buildID, 10))
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
