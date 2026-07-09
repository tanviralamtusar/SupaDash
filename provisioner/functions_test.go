package provisioner

import "testing"

func TestValidateFunctionSlug(t *testing.T) {
	valid := []string{"hello", "hello-world", "fn_2", "A1", "x"}
	for _, slug := range valid {
		if err := ValidateFunctionSlug(slug); err != nil {
			t.Errorf("ValidateFunctionSlug(%q) unexpected error: %v", slug, err)
		}
	}

	invalid := []string{
		"",
		"main",             // reserved
		"../escape",        // traversal
		"has space",        // whitespace
		"has/slash",        // separator
		"has\\backslash",   // separator
		".hidden",          // leading dot
		"-leading-dash",    // must start alphanumeric
		"way-too-long-" + string(make([]byte, 100)), // length + NUL bytes
	}
	for _, slug := range invalid {
		if err := ValidateFunctionSlug(slug); err == nil {
			t.Errorf("ValidateFunctionSlug(%q) expected error, got nil", slug)
		}
	}
}
