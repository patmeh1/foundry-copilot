// budget.go is the daily-budget alerter. Tracks state on disk under
// ~/.foundry-copilot/budget.json. Idempotent per-day: at most one alert
// fires per UTC day.
package billing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BudgetAlerter is a tiny state machine. Zero-valued is safe (Threshold == 0
// disables alerting entirely).
type BudgetAlerter struct {
	StatePath string
	Threshold float64 // USD

	mu sync.Mutex
}

// budgetState is the on-disk record. Keyed by YYYY-MM-DD strings.
type budgetState struct {
	Alerted map[string]bool `json:"alerted"`
}

// MaybeAlert returns true the FIRST time spendUSD >= Threshold for the given
// day. Subsequent calls on the same day return false even if spend continues
// to grow. Threshold == 0 always returns false.
func (b *BudgetAlerter) MaybeAlert(today time.Time, spendUSD float64) (bool, error) {
	if b.Threshold <= 0 {
		return false, nil
	}
	if spendUSD < b.Threshold {
		return false, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st, err := b.read()
	if err != nil {
		return false, err
	}
	key := today.UTC().Format("2006-01-02")
	if st.Alerted == nil {
		st.Alerted = map[string]bool{}
	}
	if st.Alerted[key] {
		return false, nil
	}
	st.Alerted[key] = true
	if err := b.write(st); err != nil {
		return false, err
	}
	return true, nil
}

func (b *BudgetAlerter) read() (budgetState, error) {
	if b.StatePath == "" {
		return budgetState{}, errors.New("billing.BudgetAlerter: StatePath is required")
	}
	raw, err := os.ReadFile(b.StatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return budgetState{}, nil
		}
		return budgetState{}, err
	}
	var st budgetState
	if err := json.Unmarshal(raw, &st); err != nil {
		return budgetState{}, err
	}
	return st, nil
}

func (b *BudgetAlerter) write(st budgetState) error {
	if err := os.MkdirAll(filepath.Dir(b.StatePath), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(b.StatePath, raw, 0o644)
}
