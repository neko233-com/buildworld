package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRemovedFeatureRoutesStayAbsentWhileProviderWebhooksRemain(t *testing.T) {
	router, ok := newTestRouter().(chi.Routes)
	if !ok {
		t.Fatal("router does not expose chi routes")
	}
	wantWebhooks := map[string]bool{
		"POST /api/webhooks/github": false,
		"POST /api/webhooks/gitlab": false,
		"POST /api/webhooks/gitea":  false,
	}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.Contains(route, "/deployment-envs") ||
			strings.Contains(route, "/projects/{id}/hooks") ||
			strings.HasPrefix(route, "/api/hooks/") {
			return fmt.Errorf("removed feature route remains: %s %s", method, route)
		}
		key := method + " " + route
		if _, tracked := wantWebhooks[key]; tracked {
			wantWebhooks[key] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for route, found := range wantWebhooks {
		if !found {
			t.Fatalf("signed provider webhook route missing: %s", route)
		}
	}
}
