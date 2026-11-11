package engine

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dlclark/regexp2/v2"
)

// Java's Unicode and UNIX_LINES flags differ from both Go and the fallback
// engine. Reject these flags rather than evaluating them with other semantics.
var unsupportedJenkinsRegexFlags = regexp.MustCompile(`\(\?[A-Za-z-]*[Uudnre][A-Za-z-]*[:)]`)
var unsupportedJenkinsBalancingGroup = regexp.MustCompile(`\(\?<[^=!][^>]*-[^>]*>`)
var jenkinsRegexIgnoreCase = regexp.MustCompile(`\(\?[imsx-]*i[imsx-]*[:)]`)

// jenkinsRegex keeps ordinary patterns on RE2 and bounds backtracking for
// supported Jenkins patterns such as lookarounds and backreferences.
type jenkinsRegex struct {
	linear        *regexp.Regexp
	extended      *regexp2.Regexp
	asciiCaseOnly bool
}

func compileJenkinsRegex(pattern string) (*jenkinsRegex, error) {
	if unsupportedJenkinsRegexFlags.MatchString(pattern) {
		return nil, fmt.Errorf("unsupported Java regex flags")
	}
	if strings.Contains(pattern, "(?P<") || strings.Contains(pattern, "(?P=") {
		return nil, fmt.Errorf("unsupported Java regex group")
	}
	asciiCaseOnly := jenkinsRegexIgnoreCase.MatchString(pattern)
	if asciiCaseOnly && (strings.IndexFunc(pattern, func(r rune) bool { return r > 127 }) >= 0 ||
		strings.Contains(pattern, `\u`) || strings.Contains(pattern, `\x`) ||
		strings.Contains(pattern, `\p`) || strings.Contains(pattern, `\P`)) {
		return nil, fmt.Errorf("Unicode case-insensitive Java regex is unsupported")
	}
	if re, err := regexp.Compile(pattern); err == nil {
		return &jenkinsRegex{linear: re, asciiCaseOnly: asciiCaseOnly}, nil
	}
	if err := validateJenkinsExtendedRegex(pattern); err != nil {
		return nil, err
	}
	re, err := regexp2.Compile(pattern, regexp2.RE2, regexp2.OptionMaxBacktrackingStackSize(1<<16))
	if err != nil {
		return nil, err
	}
	re.MatchTimeout = 100 * time.Millisecond
	return &jenkinsRegex{extended: re, asciiCaseOnly: asciiCaseOnly}, nil
}

// RE2 compatibility mode otherwise treats unknown escapes as literals,
// whereas Java rejects them. Refuse unsupported Java/.NET-only constructs.
func validateJenkinsExtendedRegex(pattern string) error {
	if strings.Contains(pattern, "(?(") || strings.Contains(pattern, "(?'") ||
		strings.Contains(pattern, "(?#") || unsupportedJenkinsBalancingGroup.MatchString(pattern) {
		return fmt.Errorf("unsupported Java regex group")
	}
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' || i+1 >= len(pattern) {
			continue
		}
		i++
		ch := pattern[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			if !strings.ContainsRune("AabBcdDefknPprsStuwWxz", rune(ch)) {
				return fmt.Errorf("unsupported Java regex escape")
			}
		}
	}
	return nil
}

func (re *jenkinsRegex) match(value string) (bool, error) {
	if re.asciiCaseOnly && strings.IndexFunc(value, func(r rune) bool { return r > 127 }) >= 0 {
		return false, fmt.Errorf("Unicode case-insensitive Java regex is unsupported")
	}
	if re.linear != nil {
		return re.linear.MatchString(value), nil
	}
	return re.extended.MatchString(value)
}
