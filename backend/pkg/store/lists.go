package store

import (
	"strings"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
)

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// containsPattern turns search input into an ILIKE pattern that matches it as a
// literal substring. Nil means no search. Queries must say ESCAPE '\'.
func containsPattern(search string) *string {
	if search == "" {
		return nil
	}
	pattern := "%" + likeEscaper.Replace(search) + "%"
	return &pattern
}

// collectPage reads rows queried with a limit of one more than the page size, and
// uses the extra row only to report that more follow.
func collectPage[T any](rows pgx.Rows, p model.ListParams, scan pgx.RowToFunc[T]) (model.Page[T], error) {
	items, err := pgx.CollectRows(rows, scan)
	if err != nil {
		return model.Page[T]{}, err
	}
	hasMore := len(items) > p.Limit
	if hasMore {
		items = items[:p.Limit]
	}
	return model.Page[T]{Items: items, Limit: p.Limit, Offset: p.Offset, HasMore: hasMore}, nil
}
