package proofhub

import (
	"strings"
)

// helpers.go provides utility helpers for parsing targets and IDs.

// ParseTarget extracts project and list IDs from forms like:
//
//	"8213786200" + "271478716253"
//	"project-8213786200/list-271478716253"
//	"project-8213786200" (list empty)
//	full ProofHub URL containing "project-.../list-..."
func ParseTarget(projectFlag, listFlag, targetFlag string) (projectID, listID string) {
	projectID = stripPrefixDigits(projectFlag, "project-")
	listID = stripPrefixDigits(listFlag, "list-")
	if targetFlag != "" {
		p, l := parseTargetString(targetFlag)
		if p != "" {
			projectID = p
		}
		if l != "" {
			listID = l
		}
	}
	// Also allow projectFlag itself to contain "project-x/list-y"
	hasSlash := strings.Contains(projectFlag, "/")
	hasBoth := strings.Contains(projectFlag, "project-") && strings.Contains(projectFlag, "list-")
	if hasSlash || hasBoth {
		p, l := parseTargetString(projectFlag)
		if p != "" {
			projectID = p
		}
		if l != "" {
			listID = l
		}
	}
	return projectID, listID
}

func stripPrefixDigits(s, prefix string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// If value is like "project-123", return "123".
	if strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
		return s[len(prefix):]
	}
	return s
}

func parseTargetString(s string) (projectID, listID string) {
	s = strings.TrimSpace(s)
	// Normalize separators: allow full URLs, e.g. https://xxx.proofhub.com/.../project-1/list-2
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == '?' || r == '&' || r == ' ' || r == ','
	})
	for _, p := range parts {
		lp := strings.ToLower(p)
		if strings.HasPrefix(lp, "project-") {
			projectID = p[len("project-"):]
		} else if strings.HasPrefix(lp, "list-") {
			listID = p[len("list-"):]
		}
	}
	return digitsOnly(projectID), digitsOnly(listID)
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	return b.String()
}
