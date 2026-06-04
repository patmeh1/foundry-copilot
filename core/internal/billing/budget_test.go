package billing

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBudget_FiresOnceWhenCrossed(t *testing.T) {
	b := &BudgetAlerter{StatePath: filepath.Join(t.TempDir(), "budget.json"), Threshold: 50}
	day := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	got, err := b.MaybeAlert(day, 75)
	if err != nil || !got {
		t.Fatalf("want true,nil; got %v,%v", got, err)
	}
}

func TestBudget_DoesNotDoubleFire(t *testing.T) {
	b := &BudgetAlerter{StatePath: filepath.Join(t.TempDir(), "budget.json"), Threshold: 50}
	day := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	b.MaybeAlert(day, 75)
	got, _ := b.MaybeAlert(day, 100)
	if got {
		t.Fatal("second call on same day must return false")
	}
}

func TestBudget_DifferentDayFiresAgain(t *testing.T) {
	b := &BudgetAlerter{StatePath: filepath.Join(t.TempDir(), "budget.json"), Threshold: 50}
	b.MaybeAlert(time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC), 75)
	got, _ := b.MaybeAlert(time.Date(2025, 6, 2, 12, 0, 0, 0, time.UTC), 75)
	if !got {
		t.Fatal("next day must fire again")
	}
}

func TestBudget_BelowThresholdDoesNotFire(t *testing.T) {
	b := &BudgetAlerter{StatePath: filepath.Join(t.TempDir(), "budget.json"), Threshold: 50}
	got, _ := b.MaybeAlert(time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC), 49)
	if got {
		t.Fatal("must not fire below threshold")
	}
}

func TestBudget_ZeroThresholdNeverFires(t *testing.T) {
	b := &BudgetAlerter{StatePath: filepath.Join(t.TempDir(), "budget.json"), Threshold: 0}
	got, _ := b.MaybeAlert(time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC), 9999999)
	if got {
		t.Fatal("zero threshold disables alerting")
	}
}
