package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/kubev2v/assisted-migration-agent/internal/models"
	srvErrors "github.com/kubev2v/assisted-migration-agent/pkg/errors"
)

const (
	collectionTable = "collections"

	colColID                           = "id"
	colColVCenterID                    = "vcenter_id"
	colColVCenter                      = "vcenter"
	colColState                        = "state"
	colColActive                       = "active"
	colColVMCountMigratable            = "vm_count_migratable"
	colColVMCountNonMigratable         = "vm_count_non_migratable"
	colColVMCountTotal                 = "vm_count_total"
	colColClusterCountTotal            = "cluster_count_total"
	colColVMCountNewSincePrevious      = "vm_count_new_since_previous"
	colColVMCountMissingSincePrevious  = "vm_count_missing_since_previous"
	colColVMCountDeltaSincePrevious    = "vm_count_delta_since_previous"
	colColVMCountMigDeltaSincePrevious = "vm_count_migratable_delta_since_previous"
	colColStartedAt                    = "started_at"
	colColFinishedAt                   = "finished_at"
	colColError                        = "error"
	colColCreatedAt                    = "created_at"
	colColUpdatedAt                    = "updated_at"
)

var collectionSelectColumns = []string{
	colColID,
	colColVCenterID,
	colColVCenter,
	colColState,
	colColActive,
	colColVMCountMigratable,
	colColVMCountNonMigratable,
	colColVMCountTotal,
	colColClusterCountTotal,
	colColVMCountNewSincePrevious,
	colColVMCountMissingSincePrevious,
	colColVMCountDeltaSincePrevious,
	colColVMCountMigDeltaSincePrevious,
	colColStartedAt,
	colColFinishedAt,
	colColError,
	colColCreatedAt,
	colColUpdatedAt,
}

// CollectionCounters holds the VM/cluster count fields updated after a collection run.
type CollectionCounters struct {
	VMCountMigratable    int
	VMCountNonMigratable int
	VMCountTotal         int
	ClusterCountTotal    int
}

type CollectionStore struct {
	db QueryInterceptor
}

func NewCollectionStore(db QueryInterceptor) *CollectionStore {
	return &CollectionStore{db: db}
}

// List returns all collections matching filter. Pass nil to return all rows.
// Use ListOption helpers (WithOrderBy, WithOffset, WithLimit) for sorting and pagination.
func (s *CollectionStore) List(ctx context.Context, filter sq.Sqlizer, opts ...ListOption) ([]models.Collection, error) {
	builder := sq.Select(collectionSelectColumns...).From(collectionTable)
	if filter != nil {
		builder = builder.Where(filter)
	}
	for _, opt := range opts {
		builder = opt(builder)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("building list collections query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying collections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cols []models.Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning collection: %w", err)
		}
		cols = append(cols, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating collection rows: %w", err)
	}
	return cols, nil
}

// Create inserts a new collection row and returns the persisted record.
// The id is assigned by the DB sequence; do not set col.ID.
func (s *CollectionStore) Create(ctx context.Context, col models.Collection) (*models.Collection, error) {
	now := time.Now()

	query, args, err := sq.Insert(collectionTable).
		Columns(
			colColVCenterID,
			colColVCenter,
			colColState,
			colColActive,
			colColStartedAt,
			colColError,
			colColCreatedAt,
			colColUpdatedAt,
		).
		Values(
			col.VCenterID,
			sql.NullString{String: col.VCenter, Valid: col.VCenter != ""},
			col.State,
			col.Active,
			col.StartedAt,
			sql.NullString{String: col.Error, Valid: col.Error != ""},
			now,
			now,
		).
		Suffix(fmt.Sprintf("RETURNING %s", strings.Join(collectionSelectColumns, ", "))).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("building create collection query: %w", err)
	}

	row := s.db.QueryRowContext(ctx, query, args...)
	return scanCollection(row)
}

// MarkFailed sets state=failed, records the error message, and sets finished_at=now().
// Only matches collections in running state (WHERE state = 'running').
func (s *CollectionStore) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	query, args, err := sq.Update(collectionTable).
		Set(colColState, string(models.CollectionStateFailed)).
		Set(colColError, errMsg).
		Set(colColFinishedAt, sq.Expr("now()")).
		Set(colColUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{colColID: id, colColState: string(models.CollectionStateRunning)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("building mark failed query: %w", err)
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("marking collection failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return srvErrors.NewResourceNotFoundError("collection", fmt.Sprintf("%d", id))
	}

	return nil
}

// UpdateCounters updates the VM/cluster count fields for the given collection.
func (s *CollectionStore) UpdateCounters(ctx context.Context, id int64, counters CollectionCounters) error {
	query, args, err := sq.Update(collectionTable).
		Set(colColVMCountMigratable, counters.VMCountMigratable).
		Set(colColVMCountNonMigratable, counters.VMCountNonMigratable).
		Set(colColVMCountTotal, counters.VMCountTotal).
		Set(colColClusterCountTotal, counters.ClusterCountTotal).
		Set(colColUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{colColID: id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("building update counters query: %w", err)
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating collection counters: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return srvErrors.NewResourceNotFoundError("collection", fmt.Sprintf("%d", id))
	}

	return nil
}

// MarkDone sets state=done, active=true, finished_at=now().
// Only matches collections in running state (WHERE state = 'running').
// Returns ResourceNotFoundError when the id does not exist OR when the
// collection exists but is not in running state — callers cannot distinguish
// the two cases from the error alone.
func (s *CollectionStore) MarkDone(ctx context.Context, id int64) error {
	query, args, err := sq.Update(collectionTable).
		Set(colColState, string(models.CollectionStateDone)).
		Set(colColActive, true).
		Set(colColFinishedAt, sq.Expr("now()")).
		Set(colColUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{colColID: id, colColState: string(models.CollectionStateRunning)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("building mark done query: %w", err)
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("marking collection done: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return srvErrors.NewResourceNotFoundError("collection", fmt.Sprintf("%d", id))
	}
	return nil
}

// Deactivate sets active=false for the given collection.
// Silent no-op if the collection no longer exists (e.g. already deleted by a retention sweep).
func (s *CollectionStore) Deactivate(ctx context.Context, id int64) error {
	query, args, err := sq.Update(collectionTable).
		Set(colColActive, false).
		Set(colColUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{colColID: id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("building deactivate query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("deactivating collection: %w", err)
	}
	return nil
}

// Delete removes a collection by ID.
func (s *CollectionStore) Delete(ctx context.Context, id int64) error {
	query, args, err := sq.Delete(collectionTable).
		Where(sq.Eq{colColID: id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("building delete collection query: %w", err)
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("deleting collection: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return srvErrors.NewResourceNotFoundError("collection", fmt.Sprintf("%d", id))
	}

	return nil
}

// scanCollection scans one row into a *models.Collection.
// Column order must match collectionSelectColumns exactly.
func scanCollection(row rowScanner) (*models.Collection, error) {
	var (
		c       models.Collection
		vcenter sql.NullString
		errMsg  sql.NullString
	)
	err := row.Scan(
		&c.ID,
		&c.VCenterID,
		&vcenter,
		&c.State,
		&c.Active,
		&c.VMCountMigratable,
		&c.VMCountNonMigratable,
		&c.VMCountTotal,
		&c.ClusterCountTotal,
		&c.VMCountNewSincePrevious,
		&c.VMCountMissingSincePrevious,
		&c.VMCountDeltaSincePrevious,
		&c.VMCountMigratableDeltaSincePrevious,
		&c.StartedAt,
		&c.FinishedAt,
		&errMsg,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.VCenter = vcenter.String
	c.Error = errMsg.String
	return &c, nil
}
