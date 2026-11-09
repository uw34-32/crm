package model

const (
	// DefaultPageSize applies when a list does not ask for a size.
	DefaultPageSize = 50
	// MaxPageSize is the most rows one list response can hold.
	MaxPageSize = 200
	// MaxPageOffset bounds how many rows a list may be asked to skip, because
	// PostgreSQL still has to walk them.
	MaxPageOffset = 100_000
)

// ListParams selects one page of a list. Search is matched literally, and empty
// means no filter.
type ListParams struct {
	Search string
	Limit  int
	Offset int
}

// Bounded returns p with its size and offset forced inside the limits, whatever
// the caller validated.
func (p ListParams) Bounded() ListParams {
	if p.Limit <= 0 {
		p.Limit = DefaultPageSize
	}
	p.Limit = min(p.Limit, MaxPageSize)
	p.Offset = min(max(p.Offset, 0), MaxPageOffset)
	return p
}

// Page is one page of a list. HasMore reports whether rows follow it.
type Page[T any] struct {
	Items   []T  `json:"items"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}
