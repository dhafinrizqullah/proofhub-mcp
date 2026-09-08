package proofhub

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		projectFlag string
		listFlag    string
		targetFlag  string
		wantProject string
		wantList    string
	}{
		{name: "plain ids", projectFlag: "123", listFlag: "456", wantProject: "123", wantList: "456"},
		{name: "project prefix", projectFlag: "project-123", listFlag: "list-456", wantProject: "123", wantList: "456"},
		{name: "target shorthand", projectFlag: "", listFlag: "", targetFlag: "project-123/list-456", wantProject: "123", wantList: "456"},
		{name: "target overrides", projectFlag: "999", listFlag: "888", targetFlag: "project-123/list-456", wantProject: "123", wantList: "456"},
		{name: "full url", projectFlag: "https://example.proofhub.com/project-123/list-456", wantProject: "123", wantList: "456"},
		{name: "only project", projectFlag: "project-123", wantProject: "123", wantList: ""},
		{name: "empty", wantProject: "", wantList: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotP, gotL := ParseTarget(tt.projectFlag, tt.listFlag, tt.targetFlag)
			assert.Equal(t, tt.wantProject, gotP)
			assert.Equal(t, tt.wantList, gotL)
		})
	}
}

func TestDigitsOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "all digits", input: "12345", expected: "12345"},
		{name: "digits with letters", input: "123abc", expected: "123"},
		{name: "empty", input: "", expected: ""},
		{name: "no digits", input: "abc", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, digitsOnly(tt.input))
		})
	}
}

func TestStripPrefixDigits(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "123", stripPrefixDigits("project-123", "project-"))
	assert.Equal(t, "123", stripPrefixDigits("PROJECT-123", "project-"))
	assert.Equal(t, "456", stripPrefixDigits("456", "project-"))
	assert.Equal(t, "", stripPrefixDigits("", "project-"))
}
