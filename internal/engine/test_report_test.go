package engine

import "testing"

func TestParseJUnitReportSupportsTestSuitesAndCaseDetails(t *testing.T) {
	report, err := ParseJUnitReport([]byte(`<?xml version="1.0"?>
<testsuites>
  <testsuite name="unit" tests="3" failures="1" skipped="1" time="0.75">
    <testcase classname="pkg.Math" name="adds" time="0.10"/>
    <testcase classname="pkg.Math" name="subtracts" time="0.20"><skipped message="not supported"/></testcase>
    <testcase classname="pkg.Math" name="divides" time="0.45"><failure message="want 2, got 3" type="AssertionError">stack trace</failure></testcase>
  </testsuite>
</testsuites>`))
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Total != 3 || report.Summary.Passed != 1 || report.Summary.Failed != 1 || report.Summary.Skipped != 1 || report.Summary.Duration != 750 {
		t.Fatalf("summary = %#v", report.Summary)
	}
	if len(report.Cases) != 3 {
		t.Fatalf("cases = %#v", report.Cases)
	}
	if got := report.Cases[0]; got.Name != "adds" || got.ClassName != "pkg.Math" || got.SuiteName != "unit" || got.Status != "passed" || got.DurationMS != 100 {
		t.Fatalf("passed case = %#v", got)
	}
	if got := report.Cases[1]; got.Status != "skipped" || got.Message != "not supported" {
		t.Fatalf("skipped case = %#v", got)
	}
	if got := report.Cases[2]; got.Status != "failed" || got.Message != "want 2, got 3" || got.Type != "AssertionError" || got.Details != "stack trace" {
		t.Fatalf("failed case = %#v", got)
	}
}

func TestParseJUnitReportSupportsSingleSuiteWithoutSummaryAttributes(t *testing.T) {
	report, err := ParseJUnitReport([]byte(`<testsuite name="integration">
  <testcase name="connects" time="0.0014"/>
  <testcase name="times out" time="0.0026"><error message="deadline" type="TimeoutError">timed out</error></testcase>
  <testcase name="windows only"><skipped/></testcase>
</testsuite>`))
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Total != 3 || report.Summary.Passed != 1 || report.Summary.Failed != 1 || report.Summary.Skipped != 1 || report.Summary.Duration != 4 {
		t.Fatalf("summary = %#v", report.Summary)
	}
	if got := report.Cases[1]; got.Status != "failed" || got.Message != "deadline" || got.Type != "TimeoutError" || got.Details != "timed out" || got.DurationMS != 3 {
		t.Fatalf("error case = %#v", got)
	}
}

func TestParseJUnitReportUsesSuiteAttributesWhenCasesAreUnavailable(t *testing.T) {
	report, err := ParseJUnitReport([]byte(`<testsuite name="remote" tests="8" failures="2" errors="1" skipped="1" time="1.25"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Total != 8 || report.Summary.Passed != 4 || report.Summary.Failed != 3 || report.Summary.Skipped != 1 || report.Summary.Duration != 1250 {
		t.Fatalf("summary = %#v", report.Summary)
	}
	if len(report.Cases) != 0 {
		t.Fatalf("cases = %#v, want empty", report.Cases)
	}
}

func TestParseJUnitReportRejectsNonJUnitRoot(t *testing.T) {
	if _, err := ParseJUnitReport([]byte(`<report/>`)); err == nil {
		t.Fatal("expected non-JUnit root to fail")
	}
}
