package store

import (
	"strings"
	"unicode/utf8"
)

const (
	// BuildLogViewMaxCharacters and BuildLogViewMaxLines bound the interactive
	// log window. Downloads and server-side diagnostics continue to use the
	// durable retention limit instead of this smaller view window.
	BuildLogViewMaxCharacters = 160_000
	BuildLogViewMaxLines      = 2_000
)

// BuildLogTail is the bounded payload used by the interactive log viewer.
// PersistedTruncated describes durable retention; WindowTruncated describes
// only the smaller operator-facing tail returned by this method.
type BuildLogTail struct {
	Log                string
	PersistedTruncated bool
	WindowTruncated    bool
}

// GetBuildLogTail reads only a small suffix from SQLite before applying the
// line boundary. This keeps both the database-to-process and HTTP payloads
// bounded when a build has a large retained log.
func (s *Store) GetBuildLogTail(id int64, maxLines, maxCharacters int) (BuildLogTail, error) {
	if maxLines <= 0 || maxLines > BuildLogViewMaxLines {
		maxLines = BuildLogViewMaxLines
	}
	if maxCharacters <= 0 || maxCharacters > BuildLogRetentionCharacters {
		maxCharacters = BuildLogViewMaxCharacters
	}

	// Read one extra character so a suffix that starts in the middle of a line
	// can be advanced to the next complete line without losing the boundary.
	sampleCharacters := maxCharacters + 1
	prefixCharacters := utf8.RuneCountInString(BuildLogTruncationMarker) + utf8.RuneCountInString(BuildLogOversizedMarker)
	var sample, prefix string
	var totalCharacters int
	err := s.db.QueryRow(
		`SELECT substr(COALESCE(log, ''), -?),
		        substr(COALESCE(log, ''), 1, ?),
		        length(COALESCE(log, ''))
		 FROM builds WHERE id = ?`,
		sampleCharacters, prefixCharacters, id,
	).Scan(&sample, &prefix, &totalCharacters)
	if err != nil {
		return BuildLogTail{}, err
	}

	window, lineTrimmed := retainBuildLogTail(sample, maxLines, maxCharacters)
	return BuildLogTail{
		Log:                window,
		PersistedTruncated: IsBuildLogTruncated(prefix),
		WindowTruncated:    totalCharacters > maxCharacters || lineTrimmed,
	}, nil
}

func retainBuildLogTail(log string, maxLines, maxCharacters int) (string, bool) {
	if log == "" {
		return "", false
	}

	start := 0
	truncated := false
	if utf8.RuneCountInString(log) > maxCharacters {
		start = byteOffsetAfterRunes(log, utf8.RuneCountInString(log)-maxCharacters)
		truncated = true
	}
	if start > 0 && log[start-1] != '\n' {
		if nextLine := strings.IndexByte(log[start:], '\n'); nextLine >= 0 {
			start += nextLine + 1
		}
	}

	lineStart := tailLineStart(log, maxLines)
	if lineStart > start {
		start = lineStart
		truncated = true
	}
	return log[start:], truncated
}

func tailLineStart(log string, maxLines int) int {
	lines := 1
	if strings.HasSuffix(log, "\n") {
		// A trailing newline terminates the last real line; it does not create
		// an extra empty log entry for tail-window purposes.
		lines = 0
	}
	for index := len(log) - 1; index >= 0; index-- {
		if log[index] != '\n' {
			continue
		}
		lines++
		if lines > maxLines {
			return index + 1
		}
	}
	return 0
}

func byteOffsetAfterRunes(text string, runes int) int {
	if runes <= 0 {
		return 0
	}
	for index := range text {
		if runes == 0 {
			return index
		}
		runes--
	}
	return len(text)
}
