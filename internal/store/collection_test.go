package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubev2v/assisted-migration-agent/internal/models"
	"github.com/kubev2v/assisted-migration-agent/internal/store"
	"github.com/kubev2v/assisted-migration-agent/internal/store/migrations"
	srvErrors "github.com/kubev2v/assisted-migration-agent/pkg/errors"
	"github.com/kubev2v/assisted-migration-agent/pkg/filter"
	"github.com/kubev2v/assisted-migration-agent/test"
)

func newTestCollection(vcenterID string) models.Collection {
	now := time.Now()
	return models.Collection{
		VCenterID: vcenterID,
		VCenter:   "vc-01.example.com",
		State:     models.CollectionStateRunning,
		Active:    false,
		StartedAt: &now,
	}
}

var _ = Describe("CollectionStore", func() {
	var (
		ctx context.Context
		s   *store.Store
		db  *sql.DB
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		db, err = store.NewDB(nil, ":memory:")
		Expect(err).NotTo(HaveOccurred())

		err = migrations.Run(ctx, db)
		Expect(err).NotTo(HaveOccurred())

		s = store.NewStore(db, test.NewMockValidator())
	})

	AfterEach(func() {
		if db != nil {
			_ = db.Close()
		}
	})

	Context("Create", func() {
		// Given a new collection
		// When we create it
		// Then it should be persisted with a sequence-assigned ID > 0 and a non-zero CreatedAt
		It("should persist a collection and return it with a sequence-assigned ID", func() {
			vcenterID := uuid.New().String()
			col := newTestCollection(vcenterID)

			created, err := s.Collection().Create(ctx, col)
			Expect(err).NotTo(HaveOccurred())
			Expect(created.ID).To(BeNumerically(">", int64(0)))
			Expect(created.VCenterID).To(Equal(col.VCenterID))
			Expect(created.State).To(Equal(col.State))
			Expect(created.Active).To(Equal(col.Active))
			Expect(created.CreatedAt.IsZero()).To(BeFalse())
		})
	})

	Context("List (was GetActive)", func() {
		// Given no active collection for a vcenter
		// When List is called with active=true filter
		// Then the result is empty (no error)
		It("should return empty when no active collection exists", func() {
			results, err := s.Collection().List(ctx, sq.Eq{"vcenter_id": uuid.New().String(), "active": true})
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(BeEmpty())
		})
	})

	Context("MarkDone and Deactivate", func() {
		// Given two sequential collections for the same vcenter
		// When the second is marked done superseding the first (deactivate prev, mark new done)
		// Then the second should be active+done, the first inactive,
		// and the first should appear in List with done+inactive filter
		It("should make the new collection active and deactivate the previous", func() {
			vcenterID := uuid.New().String()

			prev := newTestCollection(vcenterID)
			prevCreated, err := s.Collection().Create(ctx, prev)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Collection().MarkDone(ctx, prevCreated.ID)).To(Succeed())

			results, err := s.Collection().List(ctx, sq.Eq{"vcenter_id": vcenterID, "active": true})
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results[0].ID).To(Equal(prevCreated.ID))

			newCol := newTestCollection(vcenterID)
			newColCreated, err := s.Collection().Create(ctx, newCol)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Collection().Deactivate(ctx, prevCreated.ID)).To(Succeed())
			Expect(s.Collection().MarkDone(ctx, newColCreated.ID)).To(Succeed())

			results, err = s.Collection().List(ctx, sq.Eq{"vcenter_id": vcenterID, "active": true})
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			activeNew := results[0]
			Expect(activeNew.ID).To(Equal(newColCreated.ID))
			Expect(activeNew.State).To(Equal(models.CollectionStateDone))
			Expect(activeNew.FinishedAt).NotTo(BeNil())

			oldCols, err := s.Collection().List(ctx,
				sq.Eq{"vcenter_id": vcenterID, "state": string(models.CollectionStateDone), "active": false},
				store.WithOrderBy("finished_at DESC"),
				store.WithOffset(0),
			)
			Expect(err).NotTo(HaveOccurred())
			oldIDs := make([]int64, len(oldCols))
			for i, c := range oldCols {
				oldIDs[i] = c.ID
			}
			Expect(oldIDs).To(ContainElement(prevCreated.ID))
		})
	})

	Context("List with retention offset", func() {
		// Given four sequential published collections (col4 active, col1/2/3 done+inactive)
		// When List is called with state=done active=false and WithOffset(1)
		// Then two oldest are returned (col3 retained, col1+col2 returned)
		// And with WithOffset(3), nothing is returned (all 3 inactives retained)
		It("should respect the retain count offset", func() {
			vcenterID := uuid.New().String()

			var prevID int64
			for i := range 3 {
				col := newTestCollection(vcenterID)
				created, err := s.Collection().Create(ctx, col)
				Expect(err).NotTo(HaveOccurred(), "Create iter %d", i)
				if prevID != 0 {
					Expect(s.Collection().Deactivate(ctx, prevID)).To(Succeed())
				}
				Expect(s.Collection().MarkDone(ctx, created.ID)).To(Succeed())
				prevID = created.ID
			}
			col4 := newTestCollection(vcenterID)
			col4Created, err := s.Collection().Create(ctx, col4)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Collection().Deactivate(ctx, prevID)).To(Succeed())
			Expect(s.Collection().MarkDone(ctx, col4Created.ID)).To(Succeed())

			doneInactiveFilter := sq.Eq{"vcenter_id": vcenterID, "state": string(models.CollectionStateDone), "active": false}

			// retainCount=1: keep the most recent inactive (col3), return col1+col2.
			results, err := s.Collection().List(ctx, doneInactiveFilter,
				store.WithOrderBy("finished_at DESC"),
				store.WithOffset(1),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(2))

			// retainCount=3: all 3 inactives within the retain count, return nothing.
			results, err = s.Collection().List(ctx, doneInactiveFilter,
				store.WithOrderBy("finished_at DESC"),
				store.WithOffset(3),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(BeEmpty())
		})
	})

	Context("MarkFailed", func() {
		// Given a running collection
		// When we mark it failed
		// Then List with state=running filter returns empty for that vcenter
		It("should transition state to failed and make it unavailable as running", func() {
			col := newTestCollection(uuid.New().String())
			created, err := s.Collection().Create(ctx, col)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.Collection().MarkFailed(ctx, created.ID, "something went wrong")).To(Succeed())

			running, err := s.Collection().List(ctx, sq.Eq{"vcenter_id": col.VCenterID, "state": string(models.CollectionStateRunning)})
			Expect(err).NotTo(HaveOccurred())
			Expect(running).To(BeEmpty())
		})
	})

	Context("Delete", func() {
		// Given an existing running collection
		// When we delete it
		// Then List with state=running filter returns empty (the row is gone)
		It("should remove the collection from the store", func() {
			vcenterID := uuid.New().String()
			col := newTestCollection(vcenterID)
			created, err := s.Collection().Create(ctx, col)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.Collection().Delete(ctx, created.ID)).To(Succeed())

			running, err := s.Collection().List(ctx, sq.Eq{"vcenter_id": vcenterID, "state": string(models.CollectionStateRunning)})
			Expect(err).NotTo(HaveOccurred())
			Expect(running).To(BeEmpty())
		})
	})

	Context("UpdateCounters", func() {
		// Given a collection with counters updated
		// When we mark it done and retrieve it as active
		// Then the counters should match what was set
		It("should persist VM and cluster count fields", func() {
			col := newTestCollection(uuid.New().String())
			created, err := s.Collection().Create(ctx, col)
			Expect(err).NotTo(HaveOccurred())

			counters := store.CollectionCounters{
				VMCountMigratable:    10,
				VMCountNonMigratable: 3,
				VMCountTotal:         13,
				ClusterCountTotal:    2,
			}
			Expect(s.Collection().UpdateCounters(ctx, created.ID, counters)).To(Succeed())

			Expect(s.Collection().MarkDone(ctx, created.ID)).To(Succeed())
			results, err := s.Collection().List(ctx, sq.Eq{"vcenter_id": col.VCenterID, "active": true})
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			active := results[0]
			Expect(active.VMCountTotal).To(Equal(13))
			Expect(active.VMCountMigratable).To(Equal(10))
		})
	})

	Context("List", func() {
		// Given two collections for the same vcenter (one active, one running)
		// When List is called with active=true filter
		// Then only the active collection is returned
		It("should return only collections matching the filter", func() {
			vcenterID := uuid.New().String()

			running := newTestCollection(vcenterID)
			_, err := s.Collection().Create(ctx, running)
			Expect(err).NotTo(HaveOccurred())

			active := newTestCollection(vcenterID)
			activeCreated, err := s.Collection().Create(ctx, active)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Collection().MarkDone(ctx, activeCreated.ID)).To(Succeed())

			results, err := s.Collection().List(ctx, sq.Eq{"active": true, "vcenter_id": vcenterID})
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results[0].ID).To(Equal(activeCreated.ID))
		})

		// Given no filter (nil)
		// When List is called with nil filter
		// Then all collections for any vcenter are returned
		It("should return all collections when filter is nil", func() {
			vcenterID := uuid.New().String()
			col := newTestCollection(vcenterID)
			_, err := s.Collection().Create(ctx, col)
			Expect(err).NotTo(HaveOccurred())

			results, err := s.Collection().List(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).NotTo(BeEmpty())
		})

		// Given three inactive done collections for a vcenter
		// When List is called with WithOrderBy("finished_at DESC") and WithOffset(1)
		// Then the two oldest are returned (one retained by offset)
		It("should support WithOrderBy and WithOffset for retention queries", func() {
			vcenterID := uuid.New().String()
			var prevID int64
			for i := range 3 {
				col := newTestCollection(vcenterID)
				created, err := s.Collection().Create(ctx, col)
				Expect(err).NotTo(HaveOccurred(), "Create iter %d", i)
				if prevID != 0 {
					Expect(s.Collection().Deactivate(ctx, prevID)).To(Succeed())
				}
				Expect(s.Collection().MarkDone(ctx, created.ID)).To(Succeed())
				prevID = created.ID
			}
			// Mark a 4th done to make the first 3 inactive.
			col4 := newTestCollection(vcenterID)
			col4Created, err := s.Collection().Create(ctx, col4)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Collection().Deactivate(ctx, prevID)).To(Succeed())
			Expect(s.Collection().MarkDone(ctx, col4Created.ID)).To(Succeed())

			// Offset=1 skips the most recent inactive (col3), returns col1+col2.
			results, err := s.Collection().List(ctx,
				sq.Eq{"vcenter_id": vcenterID, "state": string(models.CollectionStateDone), "active": false},
				store.WithOrderBy("finished_at DESC"),
				store.WithOffset(1),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(2))

			// Offset=3 skips all 3 inactives, returns nothing.
			results, err = s.Collection().List(ctx,
				sq.Eq{"vcenter_id": vcenterID, "state": string(models.CollectionStateDone), "active": false},
				store.WithOrderBy("finished_at DESC"),
				store.WithOffset(3),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(BeEmpty())
		})
	})

	Context("not-found error paths", func() {
		// Given a non-existent collection ID
		// When MarkFailed is called
		// Then it returns ResourceNotFoundError
		It("MarkFailed should return ResourceNotFoundError for missing id", func() {
			err := s.Collection().MarkFailed(ctx, 999999, "err")
			Expect(err).To(HaveOccurred())
			Expect(srvErrors.IsResourceNotFoundError(err)).To(BeTrue())
		})

		// Given a non-existent collection ID
		// When Delete is called
		// Then it returns ResourceNotFoundError
		It("Delete should return ResourceNotFoundError for missing id", func() {
			err := s.Collection().Delete(ctx, 999999)
			Expect(err).To(HaveOccurred())
			Expect(srvErrors.IsResourceNotFoundError(err)).To(BeTrue())
		})

		// Given a non-existent collection ID
		// When UpdateCounters is called
		// Then it returns ResourceNotFoundError
		It("UpdateCounters should return ResourceNotFoundError for missing id", func() {
			err := s.Collection().UpdateCounters(ctx, 999999, store.CollectionCounters{})
			Expect(err).To(HaveOccurred())
			Expect(srvErrors.IsResourceNotFoundError(err)).To(BeTrue())
		})

		// Given a collection that is already done
		// When MarkDone is called again
		// Then it returns ResourceNotFoundError (state guard: WHERE state = 'running')
		It("MarkDone should return ResourceNotFoundError when collection is not in running state", func() {
			col := newTestCollection(uuid.New().String())
			created, err := s.Collection().Create(ctx, col)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Collection().MarkDone(ctx, created.ID)).To(Succeed())

			// Second call: collection is now 'done', state guard rejects it.
			err = s.Collection().MarkDone(ctx, created.ID)
			Expect(err).To(HaveOccurred())
			Expect(srvErrors.IsResourceNotFoundError(err)).To(BeTrue())
		})
	})

	Context("ParseWithCollectionMap", func() {
		// Given a valid DSL expression
		// When filter.ParseWithCollectionMap is called
		// Then it returns a Sqlizer that filters collections correctly
		It("should parse a DSL expression and filter collections", func() {
			vcenterID := uuid.New().String()

			col := newTestCollection(vcenterID)
			created, err := s.Collection().Create(ctx, col)
			Expect(err).NotTo(HaveOccurred())

			expr := fmt.Sprintf("vcenter_id = '%s' and state = 'running'", vcenterID)
			f, err := filter.ParseWithCollectionMap([]byte(expr))
			Expect(err).NotTo(HaveOccurred())

			results, err := s.Collection().List(ctx, f)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results[0].ID).To(Equal(created.ID))
		})
	})
})
