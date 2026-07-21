package engine

import (
	"encoding/xml"
	"fmt"
	"math"
	"strings"

	"github.com/neko233-com/buildworld/internal/store"
)

// TestReportParser 解析 JUnit XML 测试报告
type TestReportParser struct{}

// junitXML 是 JUnit XML 顶层结构
type junitXML struct {
	XMLName    xml.Name     `xml:"testsuites"`
	TestSuites []junitSuite `xml:"testsuite"`
}

// junitSuite 兼容单 testsuite 根元素
type junitSuite struct {
	XMLName    xml.Name     `xml:"testsuite"`
	Name       string       `xml:"name,attr"`
	Tests      int          `xml:"tests,attr"`
	Failures   int          `xml:"failures,attr"`
	Errors     int          `xml:"errors,attr"`
	Skipped    int          `xml:"skipped,attr"`
	Time       float64      `xml:"time,attr"`
	TestCases  []junitCase  `xml:"testcase"`
	TestSuites []junitSuite `xml:"testsuite"`
}

type junitCase struct {
	Name      string      `xml:"name,attr"`
	ClassName string      `xml:"classname,attr"`
	Time      float64     `xml:"time,attr"`
	Failure   *junitIssue `xml:"failure"`
	Error     *junitIssue `xml:"error"`
	Skipped   *junitIssue `xml:"skipped"`
}

type junitIssue struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// TestCaseResult is the stable API representation of one JUnit testcase.
type TestCaseResult struct {
	Name       string `json:"name"`
	ClassName  string `json:"classname,omitempty"`
	SuiteName  string `json:"suite_name,omitempty"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Message    string `json:"message,omitempty"`
	Type       string `json:"type,omitempty"`
	Details    string `json:"details,omitempty"`
}

// ParsedTestReport contains both the persisted summary and testcase details.
type ParsedTestReport struct {
	Summary *store.TestResult
	Cases   []TestCaseResult
}

// ParseJUnitReport parses either a testsuites document or a single testsuite.
func ParseJUnitReport(xmlData []byte) (*ParsedTestReport, error) {
	var rootName struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(xmlData, &rootName); err != nil {
		return nil, fmt.Errorf("invalid JUnit XML: %w", err)
	}

	var suites []junitSuite
	switch rootName.XMLName.Local {
	case "testsuites":
		var root junitXML
		if err := xml.Unmarshal(xmlData, &root); err != nil {
			return nil, fmt.Errorf("invalid JUnit XML: %w", err)
		}
		suites = root.TestSuites
	case "testsuite":
		var single junitSuite
		if err := xml.Unmarshal(xmlData, &single); err != nil {
			return nil, fmt.Errorf("invalid JUnit XML: %w", err)
		}
		suites = []junitSuite{single}
	default:
		return nil, fmt.Errorf("invalid JUnit XML: expected testsuite or testsuites root, got %q", rootName.XMLName.Local)
	}

	report := &ParsedTestReport{
		Summary: &store.TestResult{},
		Cases:   make([]TestCaseResult, 0),
	}
	for _, suite := range suites {
		appendJUnitSuite(report, suite)
	}
	return report, nil
}

func appendJUnitSuite(report *ParsedTestReport, suite junitSuite) {
	if len(suite.TestCases) > 0 {
		var caseDuration int64
		for _, testCase := range suite.TestCases {
			parsed := parseJUnitCase(suite.Name, testCase)
			report.Cases = append(report.Cases, parsed)
			report.Summary.Total++
			caseDuration += parsed.DurationMS
			switch parsed.Status {
			case "failed":
				report.Summary.Failed++
			case "skipped":
				report.Summary.Skipped++
			default:
				report.Summary.Passed++
			}
		}
		if suite.Time > 0 {
			report.Summary.Duration += durationMilliseconds(suite.Time)
		} else {
			report.Summary.Duration += caseDuration
		}
	} else if len(suite.TestSuites) == 0 {
		failed := suite.Failures + suite.Errors
		passed := suite.Tests - failed - suite.Skipped
		if passed < 0 {
			passed = 0
		}
		report.Summary.Total += suite.Tests
		report.Summary.Passed += passed
		report.Summary.Failed += failed
		report.Summary.Skipped += suite.Skipped
		report.Summary.Duration += durationMilliseconds(suite.Time)
	}

	for _, child := range suite.TestSuites {
		appendJUnitSuite(report, child)
	}
}

func parseJUnitCase(suiteName string, testCase junitCase) TestCaseResult {
	result := TestCaseResult{
		Name:       testCase.Name,
		ClassName:  testCase.ClassName,
		SuiteName:  suiteName,
		Status:     "passed",
		DurationMS: durationMilliseconds(testCase.Time),
	}
	if testCase.Failure != nil {
		result.Status = "failed"
		result.Message = strings.TrimSpace(testCase.Failure.Message)
		result.Type = strings.TrimSpace(testCase.Failure.Type)
		result.Details = strings.TrimSpace(testCase.Failure.Body)
	} else if testCase.Error != nil {
		result.Status = "failed"
		result.Message = strings.TrimSpace(testCase.Error.Message)
		result.Type = strings.TrimSpace(testCase.Error.Type)
		result.Details = strings.TrimSpace(testCase.Error.Body)
	} else if testCase.Skipped != nil {
		result.Status = "skipped"
		result.Message = strings.TrimSpace(testCase.Skipped.Message)
		result.Details = strings.TrimSpace(testCase.Skipped.Body)
	}
	return result
}

func durationMilliseconds(seconds float64) int64 {
	if seconds <= 0 {
		return 0
	}
	return int64(math.Round(seconds * 1000))
}

// ParseJUnitXML parses JUnit XML and returns its summary.
func ParseJUnitXML(xmlData []byte) (*store.TestResult, error) {
	report, err := ParseJUnitReport(xmlData)
	if err != nil {
		return nil, err
	}
	return report.Summary, nil
}

// ParseAndSave 解析 XML 并持久化到 store，同时把 test_result_id 关联到 build。
func (p *TestReportParser) ParseAndSave(buildID int64, xmlData []byte, s *store.Store) (*store.TestResult, error) {
	result, err := ParseJUnitXML(xmlData)
	if err != nil {
		return nil, err
	}
	result.BuildID = buildID
	result.ReportXML = string(xmlData)
	saved, err := s.CreateTestResult(buildID, result.Total, result.Passed, result.Failed, result.Skipped, result.Duration, result.ReportXML)
	if err != nil {
		return nil, err
	}
	_ = s.SetBuildTestResult(buildID, saved.ID)
	return saved, nil
}

// NewTestReportParser 工厂函数，方便外部使用。
func NewTestReportParser() *TestReportParser { return &TestReportParser{} }
