package voucher

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// fakeOwnershipRepo embeds the full VoucherRepository interface so only the
// methods we exercise below need real bodies — calling any other method on
// this value would panic with a nil-interface dispatch, which is exactly what
// we want in a focused unit test.
type fakeOwnershipRepo struct {
	voucher.VoucherRepository
	bid uuid.UUID
	err error
}

func (f *fakeOwnershipRepo) GetVoucherDocumentBusinessID(ctx context.Context, docID uuid.UUID) (uuid.UUID, error) {
	return f.bid, f.err
}

func newTestReviewService(repo voucher.VoucherRepository) *DocumentReviewService {
	return &DocumentReviewService{
		voucherRepo: repo,
		logger:      logger.NewConsole(logger.ErrorLevel, logger.TextFormat),
	}
}

func TestAssertDocumentOwnership_Matches(t *testing.T) {
	bid := uuid.New()
	svc := newTestReviewService(&fakeOwnershipRepo{bid: bid})
	if err := svc.AssertDocumentOwnership(context.Background(), uuid.New(), bid); err != nil {
		t.Errorf("matching business should not error; got %v", err)
	}
}

func TestAssertDocumentOwnership_Mismatch(t *testing.T) {
	svc := newTestReviewService(&fakeOwnershipRepo{bid: uuid.New()})
	err := svc.AssertDocumentOwnership(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrDocumentNotOwned) {
		t.Errorf("different business should return ErrDocumentNotOwned; got %v", err)
	}
}

func TestAssertDocumentOwnership_LegacyNilBidFailsClosed(t *testing.T) {
	// Legacy rows predating migration 000061 have a NULL bid → returned as
	// uuid.Nil. Must be rejected rather than silently granting access.
	svc := newTestReviewService(&fakeOwnershipRepo{bid: uuid.Nil})
	err := svc.AssertDocumentOwnership(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrDocumentNotOwned) {
		t.Errorf("legacy nil-bid should fail closed; got %v", err)
	}
}

func TestAssertDocumentOwnership_RepoErrorPropagates(t *testing.T) {
	boom := errors.New("db down")
	svc := newTestReviewService(&fakeOwnershipRepo{err: boom})
	err := svc.AssertDocumentOwnership(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, boom) {
		t.Errorf("repo error should propagate; got %v", err)
	}
}

func TestDraftHash_StableAcrossKeyOrder(t *testing.T) {
	// Go maps iterate in randomized order, but json.Marshal on a map sorts
	// keys, so hashing two maps with the same content must be identical.
	a := map[string]interface{}{
		"vendor_name": "Acme",
		"total":       12345,
		"items":       []interface{}{"a", "b"},
	}
	b := map[string]interface{}{
		"items":       []interface{}{"a", "b"},
		"total":       12345,
		"vendor_name": "Acme",
	}
	if draftHash(a) != draftHash(b) {
		t.Errorf("expected same hash across key orderings")
	}
}

func TestDraftHash_DetectsHumanEdit(t *testing.T) {
	before := map[string]interface{}{"vendor_name": "Acme", "total": 12345}
	after := map[string]interface{}{"vendor_name": "Acme Corp", "total": 12345}
	if draftHash(before) == draftHash(after) {
		t.Errorf("hash should differ when a field is edited")
	}
}

func TestDraftHash_NilEmptyString(t *testing.T) {
	if draftHash(nil) != "" {
		t.Errorf("nil map should hash to empty string")
	}
}

func TestTemplateDegraded_RequiresMinimumSampleSize(t *testing.T) {
	// Even a 100% correction rate must not trigger regen when only 1-2 docs
	// have been validated — the sample is too small to be meaningful.
	if templateDegraded(2, 2) {
		t.Errorf("must not trigger with only 2 validated docs even at 100%% rate")
	}
	if templateDegraded(4, 4) {
		t.Errorf("must not trigger below the minimum sample size (5)")
	}
}

func TestTemplateDegraded_TriggersAtThreshold(t *testing.T) {
	// 3 of 5 = 60% → at threshold, should trigger.
	if !templateDegraded(3, 5) {
		t.Errorf("3/5 (60%%) should trigger regeneration")
	}
	// 6 of 10 = 60% → should trigger.
	if !templateDegraded(6, 10) {
		t.Errorf("6/10 (60%%) should trigger regeneration")
	}
	// 7 of 10 = 70% → comfortably over.
	if !templateDegraded(7, 10) {
		t.Errorf("7/10 (70%%) should trigger regeneration")
	}
}

func TestTemplateDegraded_DoesNotTriggerBelowThreshold(t *testing.T) {
	// 2 of 5 = 40% → below threshold, must not trigger.
	if templateDegraded(2, 5) {
		t.Errorf("2/5 (40%%) should not trigger regeneration")
	}
	// 5 of 10 = 50% → still under, must not trigger.
	if templateDegraded(5, 10) {
		t.Errorf("5/10 (50%%) should not trigger regeneration")
	}
	// 0 corrections → must not trigger.
	if templateDegraded(0, 20) {
		t.Errorf("clean template (0 corrections) should not trigger regeneration")
	}
}

func TestCalculateTotalPages(t *testing.T) {
	cases := []struct {
		name     string
		total    int64
		pageSize int
		want     int
	}{
		{"empty total", 0, 20, 0},
		{"exact multiple", 40, 20, 2},
		{"with remainder", 41, 20, 3},
		{"zero page size defaults to 1", 100, 0, 1},
		{"negative page size defaults to 1", 100, -5, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateTotalPages(tc.total, tc.pageSize); got != tc.want {
				t.Errorf("calculateTotalPages(%d,%d)=%d want %d", tc.total, tc.pageSize, got, tc.want)
			}
		})
	}
}
