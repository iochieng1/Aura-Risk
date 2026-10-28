package services

// County response statuses (#62), separate from moderation. Responders move
// a verified report forward: open -> acknowledged -> resolved.
const (
	ResponseOpen         = "open"
	ResponseAcknowledged = "acknowledged"
	ResponseResolved     = "resolved"
)

func IsResponseStatus(s string) bool {
	return s == ResponseOpen || s == ResponseAcknowledged || s == ResponseResolved
}

// CanChangeResponse reports whether a responder may move a report from one
// status to another. Moves only go forward; resolving an open report is
// allowed and counts as acknowledging it at the same time.
func CanChangeResponse(from, to string) bool {
	switch from {
	case ResponseOpen:
		return to == ResponseAcknowledged || to == ResponseResolved
	case ResponseAcknowledged:
		return to == ResponseResolved
	}
	return false
}
