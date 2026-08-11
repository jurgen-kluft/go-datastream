package datastream

import (
	"fmt"
	"os"
)

type IssueType int8

const (
	IssueTypeError IssueType = iota
	IssueTypeWarning
	IssueTypeInformation
)

type Issue struct {
	IssueType   IssueType
	Description string
}

func (issue Issue) String() string {
	label := "Unknown"
	switch issue.IssueType {
	case IssueTypeError:
		label = "Error"
	case IssueTypeWarning:
		label = "Warning"
	case IssueTypeInformation:
		label = "Information"
	}
	return fmt.Sprintf("%s: %s", label, issue.Description)
}

type Context struct {
	issues  []Issue
	verbose bool
}

func NewContext() *Context {
	return &Context{}
}

func (ctx *Context) SetVerbose(verbose bool) {
	ctx.verbose = verbose
}

func (ctx *Context) Issues() []Issue {
	return append([]Issue(nil), ctx.issues...)
}

func (ctx *Context) HasErrors() bool {
	return ctx.hasIssueType(IssueTypeError)
}

func (ctx *Context) HasWarnings() bool {
	return ctx.hasIssueType(IssueTypeWarning)
}

func (ctx *Context) Report() {
	for _, issue := range ctx.issues {
		output := os.Stdout
		if issue.IssueType == IssueTypeError {
			output = os.Stderr
		}
		fmt.Fprintln(output, issue.String())
	}
}

func (ctx *Context) AddError(format string, args ...any) {
	ctx.add(IssueTypeError, fmt.Sprintf(format, args...))
}

func (ctx *Context) AddWarning(format string, args ...any) {
	ctx.add(IssueTypeWarning, fmt.Sprintf(format, args...))
}

func (ctx *Context) AddInformation(format string, args ...any) {
	ctx.add(IssueTypeInformation, fmt.Sprintf(format, args...))
}

func (ctx *Context) hasIssueType(issueType IssueType) bool {
	for _, issue := range ctx.issues {
		if issue.IssueType == issueType {
			return true
		}
	}
	return false
}

func (ctx *Context) add(issueType IssueType, description string) {
	if issueType == IssueTypeInformation && !ctx.verbose {
		return
	}
	ctx.issues = append(ctx.issues, Issue{IssueType: issueType, Description: description})
}
