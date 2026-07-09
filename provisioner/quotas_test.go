package provisioner

import "testing"

func TestValidateResourceFloor(t *testing.T) {
	const MB = 1024 * 1024

	tests := []struct {
		name     string
		memory   int64
		storage  int64
		database int64
		wantErr  bool
	}{
		{"all zero (plan defaults)", 0, 0, 0, false},
		{"exactly at minimums", 500 * MB, 1024 * MB, 500 * MB, false},
		{"above minimums", 1024 * MB, 5 * 1024 * MB, 2048 * MB, false},
		{"memory below minimum", 499 * MB, 0, 0, true},
		{"storage below minimum", 0, 1023 * MB, 0, true},
		{"database below minimum", 0, 0, 499 * MB, true},
		{"one below among valid", 500 * MB, 1024 * MB, 100 * MB, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResourceFloor(tt.memory, tt.storage, tt.database)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateResourceFloor(%d, %d, %d) error = %v, wantErr %v",
					tt.memory, tt.storage, tt.database, err, tt.wantErr)
			}
		})
	}
}

func TestPlanForInstanceSize(t *testing.T) {
	tests := []struct {
		size string
		want QuotaPlan
	}{
		{"", PlanFree},
		{"micro", PlanFree},
		{"MICRO", PlanFree},
		{"small", PlanStarter},
		{"medium", PlanPro},
		{"large", PlanEnterprise},
		{"unknown-size", PlanFree},
	}

	for _, tt := range tests {
		if got := PlanForInstanceSize(tt.size); got != tt.want {
			t.Errorf("PlanForInstanceSize(%q) = %v, want %v", tt.size, got, tt.want)
		}
	}
}

// Plan defaults must never violate the platform minimums (except unlimited 0).
func TestPlanDefaultsRespectMinimums(t *testing.T) {
	for _, plan := range []QuotaPlan{PlanFree, PlanStarter, PlanPro, PlanEnterprise} {
		q := GetDefaultQuotas(plan)
		if q.MemoryLimit != 0 && q.MemoryLimit < MinMemoryBytes {
			t.Errorf("plan %s memory default %d below minimum %d", plan, q.MemoryLimit, MinMemoryBytes)
		}
		if q.StorageSize != 0 && q.StorageSize < MinStorageBytes {
			t.Errorf("plan %s storage default %d below minimum %d", plan, q.StorageSize, MinStorageBytes)
		}
		if q.DatabaseSize != 0 && q.DatabaseSize < MinDatabaseBytes {
			t.Errorf("plan %s database default %d below minimum %d", plan, q.DatabaseSize, MinDatabaseBytes)
		}
	}
}
