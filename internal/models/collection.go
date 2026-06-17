package models

import "time"

type CollectionState string

const (
	CollectionStateRunning CollectionState = "running"
	CollectionStateDone    CollectionState = "done"
	CollectionStateFailed  CollectionState = "failed"
)

type Collection struct {
	ID                                  int64
	VCenterID                           string
	VCenter                             string
	State                               CollectionState
	Active                              bool
	VMCountMigratable                   int
	VMCountNonMigratable                int
	VMCountTotal                        int
	ClusterCountTotal                   int
	VMCountNewSincePrevious             int
	VMCountMissingSincePrevious         int
	VMCountDeltaSincePrevious           int
	VMCountMigratableDeltaSincePrevious int
	StartedAt                           *time.Time
	FinishedAt                          *time.Time
	Error                               string
	CreatedAt                           time.Time
	UpdatedAt                           time.Time
}
