package mysql

import (
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"math"
	"testing"
)

func TestAllocationCapacityDoesNotRoundUsage(t *testing.T) {
	for _, tc := range []struct {
		ratio   float64
		used    int64
		exceeds bool
	}{
		{math.Exp2(53), 1 << 53, false}, {math.Exp2(53), 1<<53 + 1, true},
		{math.Exp2(63), math.MaxInt64, false}, {4.5, 4, false}, {4.5, 5, true},
	} {
		inv := model.Inventory{Total: 1, AllocationRatio: tc.ratio}
		if got := allocationExceedsCapacity(tc.used, inv); got != tc.exceeds {
			t.Fatal(tc, got)
		}
	}
}
