package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neko233-com/buildworld233/internal/store"
)

// LogEntry 是结构化日志行
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // INFO/WARN/ERROR/DEBUG
	Stage     string    `json:"stage"`
	Message   string    `json:"message"`
}

// BuildLogManager 提供结构化日志追加、检索、下载
type BuildLogManager struct {
	store *store.Store
}

func NewBuildLogManager(s *store.Store) *BuildLogManager {
	return &BuildLogManager{store: s}
}

// AppendLog 以 `[ts] [LEVEL] [stage] msg` 形式追加到 builds.log。
func (m *BuildLogManager) AppendLog(buildID int64, level, stage, message string) error {
	ts := time.Now().Format("15:04:05")
	if level == "" {
		level = "INFO"
	}
	line := fmt.Sprintf("[%s] [%s] [%s] %s\n", ts, level, stage, message)
	return m.store.AppendBuildLog(buildID, line)
}

// GetStructuredLogs 解析构建日志为结构化条目
func (m *BuildLogManager) GetStructuredLogs(buildID int64) ([]LogEntry, error) {
	build, err := m.store.GetBuild(buildID)
	if err != nil {
		return nil, err
	}
	return parseLogEntries(build.Log), nil
}

// DownloadLogs 返回 (data, filename)，format 为 "txt" 或 "json"。
func (m *BuildLogManager) DownloadLogs(buildID int64, format string) ([]byte, string) {
	build, err := m.store.GetBuild(buildID)
	if err != nil {
		return nil, ""
	}
	filename := fmt.Sprintf("build-%d-logs.%s", buildID, format)
	switch strings.ToLower(format) {
	case "json":
		entries := parseLogEntries(build.Log)
		data, _ := json.MarshalIndent(entries, "", "  ")
		return data, filename
	default:
		return []byte(build.Log), filename
	}
}

// SearchLogs 在日志消息中模糊匹配 query（大小写不敏感）。
func (m *BuildLogManager) SearchLogs(buildID int64, query string) ([]LogEntry, error) {
	entries, err := m.GetStructuredLogs(buildID)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return entries, nil
	}
	q := strings.ToLower(query)
	var matched []LogEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Message), q) ||
			strings.Contains(strings.ToLower(e.Stage), q) ||
			strings.Contains(strings.ToLower(e.Level), q) {
			matched = append(matched, e)
		}
	}
	return matched, nil
}

// parseLogEntries 解析已有的日志文本。兼容旧格式 `[ts] [stage] msg` 与新格式 `[ts] [LEVEL] [stage] msg`。
func parseLogEntries(log string) []LogEntry {
	if log == "" {
		return nil
	}
	var entries []LogEntry
	for _, line := range strings.Split(log, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		e := LogEntry{Level: "INFO"}
		s := line
		// 去掉开头的 [
		if strings.HasPrefix(s, "[") {
			if end := strings.Index(s, "]"); end > 0 {
				if t, err := time.Parse("15:04:05", s[1:end]); err == nil {
					e.Timestamp = t
				}
				s = s[end+1:]
			}
		}
		s = strings.TrimLeft(s, " ")
		// 后续可能有 [LEVEL] 与 [stage]
		for i := 0; i < 2; i++ {
			if !strings.HasPrefix(s, "[") {
				break
			}
			end := strings.Index(s, "]")
			if end <= 0 {
				break
			}
			tok := s[1:end]
			s = s[end+1:]
			s = strings.TrimLeft(s, " ")
			// 如果是 LEVEL 关键字则赋值 level，否则当作 stage
			upper := strings.ToUpper(tok)
			switch upper {
			case "INFO", "WARN", "ERROR", "DEBUG":
				e.Level = upper
			default:
				if e.Stage == "" {
					e.Stage = tok
				}
			}
		}
		e.Message = strings.TrimSpace(s)
		entries = append(entries, e)
	}
	return entries
}
