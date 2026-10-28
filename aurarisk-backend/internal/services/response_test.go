package services

import "testing"

func TestCanChangeResponse(t *testing.T) {
	allowed := map[[2]string]bool{
		{ResponseOpen, ResponseAcknowledged}:     true,
		{ResponseOpen, ResponseResolved}:         true,
		{ResponseAcknowledged, ResponseResolved}: true,
	}
	statuses := []string{ResponseOpen, ResponseAcknowledged, ResponseResolved, "closed"}
	for _, from := range statuses {
		for _, to := range statuses {
			if got := CanChangeResponse(from, to); got != allowed[[2]string{from, to}] {
				t.Errorf("%s -> %s: %v", from, to, got)
			}
		}
	}
	if IsResponseStatus("closed") || !IsResponseStatus(ResponseResolved) {
		t.Fatal("IsResponseStatus")
	}
}
