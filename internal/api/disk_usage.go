package api

import (
	"net/http"
	"path/filepath"
)

type diskUsage struct {
	TotalBytes uint64
	FreeBytes  uint64
}

func (h *handlers) systemStorage(w http.ResponseWriter, _ *http.Request) {
	target := "."
	if h.d.Cfg != nil && h.d.Cfg.Storage.BuildTemp != "" {
		target = h.d.Cfg.Storage.BuildTemp
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "resolve build cache storage")
		return
	}
	usage, err := readDiskUsage(absoluteTarget)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "read build cache storage")
		return
	}

	usedBytes := usage.TotalBytes - usage.FreeBytes
	usedPercent := 0.0
	if usage.TotalBytes > 0 {
		usedPercent = float64(usedBytes) / float64(usage.TotalBytes) * 100
	}
	volume := filepath.VolumeName(absoluteTarget)
	if volume == "" {
		volume = string(filepath.Separator)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"executor":     "builtin",
		"volume":       volume,
		"total_bytes":  usage.TotalBytes,
		"used_bytes":   usedBytes,
		"free_bytes":   usage.FreeBytes,
		"used_percent": usedPercent,
	})
}
