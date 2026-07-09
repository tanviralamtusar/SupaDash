package provisioner

import "testing"

func TestValidateBranchName(t *testing.T) {
	valid := []string{"feature-x", "dev", "Test_1", "a"}
	for _, name := range valid {
		if err := ValidateBranchName(name); err != nil {
			t.Errorf("ValidateBranchName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := []string{"", "main", "MAIN", "postgres", "-lead", "has space", "a/b", "'; DROP DATABASE x; --"}
	for _, name := range invalid {
		if err := ValidateBranchName(name); err == nil {
			t.Errorf("ValidateBranchName(%q) expected error, got nil", name)
		}
	}
}

func TestBranchDBName(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"feature-x", "br_feature_x"},
		{"DEV", "br_dev"},
		{"a_b", "br_a_b"},
	}
	for _, tt := range tests {
		got := BranchDBName(tt.branch)
		if got != tt.want {
			t.Errorf("BranchDBName(%q) = %q, want %q", tt.branch, got, tt.want)
		}
		// Every valid branch name must yield a valid, injection-safe DB name
		if err := validateBranchDBName(got); err != nil {
			t.Errorf("derived DB name %q failed validation: %v", got, err)
		}
	}
}

func TestQuoteLiteral(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "'plain'"},
		{"o'brien", "'o''brien'"},
		{"'; DROP TABLE x; --", "'''; DROP TABLE x; --'"},
	}
	for _, tt := range tests {
		if got := quoteLiteral(tt.in); got != tt.want {
			t.Errorf("quoteLiteral(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
