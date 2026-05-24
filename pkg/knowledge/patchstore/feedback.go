package patchstore

import "time"

// Rating represents a single user's rating of a patch
type Rating struct {
	Score     int       `json:"score"`     // 1-5
	Comment   string    `json:"comment"`
	Timestamp time.Time `json:"timestamp"`
}

// Issue represents a reported problem or suggestion for a patch
type Issue struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`       // "bug", "outdated", "suggestion"
	Priority    string    `json:"priority"`   // "high", "medium", "low"
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Reporter    string    `json:"reporter"`
	Status      string    `json:"status"`     // "open", "resolved", "wontfix"
	Timestamp   time.Time `json:"timestamp"`
}

// UserFeedback holds all feedback from one user for one patch
// Stored as one JSON file per user per patch to avoid concurrent write conflicts
type UserFeedback struct {
	PatchID string  `json:"patchId"`
	Service string  `json:"service"`
	Module  string  `json:"module"`
	User    string  `json:"user"`
	Rating  *Rating `json:"rating,omitempty"`
	Issues  []Issue `json:"issues,omitempty"`
}

// HasRating returns true if this feedback contains a rating
func (f *UserFeedback) HasRating() bool {
	return f.Rating != nil && f.Rating.Score > 0
}

// OpenIssues returns issues with status "open"
func (f *UserFeedback) OpenIssues() []Issue {
	var open []Issue
	for _, issue := range f.Issues {
		if issue.Status == "open" {
			open = append(open, issue)
		}
	}
	return open
}
