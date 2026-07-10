package engine

import (
	"encoding/xml"
	"fmt"

	"github.com/neko233-com/buildworld233/internal/store"
)

// TestReportParser 解析 JUnit XML 测试报告
type TestReportParser struct{}

// junitXML 是 JUnit XML 顶层结构
type junitXML struct {
	XMLName    xml.Name      `xml:"testsuites"`
	TestSuites []junitSuite  `xml:"testsuite"`
}

// junitSuite 兼容单 testsuite 根元素
type junitSuite struct {
	XMLName    xml.Name      `xml:"testsuite"`
	Name       string        `xml:"name,attr"`
	Tests      int           `xml:"tests,attr"`
	Failures   int           `xml:"failures,attr"`
	Errors     int           `xml:"errors,attr"`
	Skipped    int           `xml:"skipped,attr"`
	Time       float64       `xml:"time,attr"`
	TestCases  []junitCase   `xml:"testcase"`
}

type junitCase struct {
	Name      string  `xml:"name,attr"`
	ClassName string  `xml:"classname,attr"`
	Time      float64 `xml:"time,attr"`
	Failure   *struct {
		Message string `xml:"message,attr"`
		Type    string `xml:"type,attr"`
		Body    string `xml:",chardata"`
	} `xml:"failure"`
	Error *struct {
		Message string `xml:"message,attr"`
		Type    string `xml:"type,attr"`
		Body    string `xml:",chardata"`
	} `xml:"error"`
	Skipped *struct {
		Message string `xml:"message,attr"`
	} `xml:"skipped"`
}

// ParseJUnitXML 解析 JUnit XML 并汇总测试结果。
func ParseJUnitXML(xmlData []byte) (*store.TestResult, error) {
	// 先尝试顶层为 testsuites
	var root junitXML
	if err := xml.Unmarshal(xmlData, &root); err != nil {
		// 退化为单 testsuite 根
		var single junitSuite
		if err2 := xml.Unmarshal(xmlData, &single); err2 != nil {
			return nil, fmt.Errorf("invalid JUnit XML: %w", err)
		}
		root.TestSuites = []junitSuite{single}
	}

	var total, passed, failed, skipped int
	var totalDuration float64
	for _, ts := range root.TestSuites {
		// 优先使用属性，否则用例计数
		suiteTests := ts.Tests
		suiteFailures := ts.Failures + ts.Errors
		suiteSkipped := ts.Skipped
		if suiteTests == 0 && len(ts.TestCases) > 0 {
			suiteTests = len(ts.TestCases)
		}
		suitePassed := suiteTests - suiteFailures - suiteSkipped
		if suitePassed < 0 {
			suitePassed = 0
		}
		total += suiteTests
		failed += suiteFailures
		skipped += suiteSkipped
		passed += suitePassed
		totalDuration += ts.Time
	}
	if total == 0 {
		// 没有属性，按用例统计
		for _, ts := range root.TestSuites {
			for _, tc := range ts.TestCases {
				total++
				totalDuration += tc.Time
				switch {
				case tc.Failure != nil || tc.Error != nil:
					failed++
				case tc.Skipped != nil:
					skipped++
				default:
					passed++
				}
			}
		}
	}

	return &store.TestResult{
		Total:    total,
		Passed:   passed,
		Failed:   failed,
		Skipped:  skipped,
		Duration: int64(totalDuration * 1000), // s -> ms
	}, nil
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
