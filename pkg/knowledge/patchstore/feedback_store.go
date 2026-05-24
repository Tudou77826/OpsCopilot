package patchstore

import "context"

// FeedbackStore manages rating and issue data for patches
type FeedbackStore interface {
	// GetFeedback retrieves all user feedback for a specific patch
	GetFeedback(ctx context.Context, patchID string) ([]UserFeedback, error)

	// SaveFeedback saves or updates a user's feedback for a patch
	SaveFeedback(ctx context.Context, feedback UserFeedback) error

	// UpdateIssueStatus changes an issue's status
	UpdateIssueStatus(ctx context.Context, patchID, issueID, status string) error

	// ListAllFeedback returns all feedback from all users
	ListAllFeedback(ctx context.Context) ([]UserFeedback, error)
}
