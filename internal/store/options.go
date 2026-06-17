package store

import sq "github.com/Masterminds/squirrel"

// WithOrderBy appends an ORDER BY clause.
// clause must be a compile-time constant SQL fragment (e.g. "finished_at DESC").
// Do not pass user-controlled strings — there is no escaping.
func WithOrderBy(clause string) ListOption {
	return func(b sq.SelectBuilder) sq.SelectBuilder {
		return b.OrderBy(clause)
	}
}
