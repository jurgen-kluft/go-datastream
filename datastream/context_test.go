package datastream

import (
	"io"
	"os"
	"testing"
)

func TestIssueString(t *testing.T) {
	tests := []struct {
		name  string
		issue Issue
		want  string
	}{
		{name: "error", issue: Issue{IssueType: IssueTypeError, Description: "failed"}, want: "Error: failed"},
		{name: "warning", issue: Issue{IssueType: IssueTypeWarning, Description: "questionable"}, want: "Warning: questionable"},
		{name: "information", issue: Issue{IssueType: IssueTypeInformation, Description: "finished"}, want: "Information: finished"},
		{name: "unknown", issue: Issue{IssueType: IssueType(99), Description: "unexpected"}, want: "Unknown: unexpected"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.issue.String(); got != test.want {
				t.Fatalf("Issue.String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestContextQueriesAndVerboseMode(t *testing.T) {
	ctx := &Context{}
	ctx.AddInformation("hidden")
	ctx.AddWarning("warning")
	ctx.AddWarning("duplicate")
	ctx.AddError("error")
	ctx.SetVerbose(true)
	ctx.AddInformation("visible")

	if !ctx.HasErrors() || !ctx.HasWarnings() {
		t.Fatal("Context did not report its errors and warnings")
	}
	issues := ctx.Issues()
	if len(issues) != 4 {
		t.Fatalf("len(Issues()) = %d, want 4", len(issues))
	}
	issues[0].Description = "modified"
	if ctx.Issues()[0].Description == "modified" {
		t.Fatal("Issues returned mutable Context storage")
	}
}

func TestContextReport(t *testing.T) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStdout, originalStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	t.Cleanup(func() {
		os.Stdout, os.Stderr = originalStdout, originalStderr
	})

	ctx := NewContext()
	ctx.SetVerbose(true)
	ctx.AddWarning("warning")
	ctx.AddError("error")
	ctx.AddInformation("information")
	ctx.Report()
	stdoutWriter.Close()
	stderrWriter.Close()

	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(stdout), "Warning: warning\nInformation: information\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := string(stderr), "Error: error\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}
