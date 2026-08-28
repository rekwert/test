package store

import "errors"

var ErrInvalidTicketStatus = errors.New("invalid ticket status")

func validTicketStatus(status string) bool {
	switch status {
	case "open", "in_progress", "waiting_client", "answered", "return_pending", "resolved", "closed":
		return true
	default:
		return false
	}
}

func validStaffTicketFilter(filter string) bool {
	switch filter {
	case "", "queue", "mine":
		return true
	default:
		return false
	}
}
