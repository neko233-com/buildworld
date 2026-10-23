package api

import (
	"net/http"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

type releaseTargetOverview struct {
	Channel     string               `json:"channel"`
	Environment string               `json:"environment"`
	AppID       string               `json:"appId,omitempty"`
	Latest      *store.BuildOverview `json:"latest"`
	LastSuccess *store.BuildOverview `json:"last_success"`
}

func (h *handlers) listReleaseTargets(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := h.d.Store.GetProject(id); err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	history, err := h.d.Store.ListReleaseTargetOverviews(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	targets := make([]releaseTargetOverview, 0, len(history))
	byKey := make(map[string]int, len(history))
	for _, target := range history {
		key := target.Channel + "\x00" + target.Environment
		byKey[key] = len(targets)
		targets = append(targets, releaseTargetOverview{
			Channel: target.Channel, Environment: target.Environment,
			Latest: target.Latest, LastSuccess: target.LastSuccess,
		})
	}
	variables, err := h.d.Store.ListEnvVars("project", &id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "release target metadata is unavailable")
		return
	}
	var declaration *store.EnvVar
	for _, variable := range variables {
		if variable == nil || variable.Name != engine.ReleaseTargetsVariable {
			continue
		}
		if declaration != nil {
			declaration = nil
			break
		}
		declaration = variable
	}
	if declaration != nil && !declaration.IsSecret && declaration.Scope == "project" {
		catalog, parseErr := engine.ParseReleaseTargetCatalog(declaration.Value)
		if parseErr == nil {
			for _, configured := range catalog.Targets {
				key := configured.Channel + "\x00" + configured.Environment
				if index, exists := byKey[key]; exists {
					targets[index].AppID = configured.AppID
					continue
				}
				byKey[key] = len(targets)
				targets = append(targets, releaseTargetOverview{
					Channel: configured.Channel, Environment: configured.Environment,
					AppID: configured.AppID,
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, targets)
}

func (h *handlers) listReleaseRecords(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := h.d.Store.GetProject(id); err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	records, err := h.d.Store.ListReleaseRecords(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "release records unavailable")
		return
	}
	writeJSON(w, http.StatusOK, records)
}
