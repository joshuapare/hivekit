package hivexval

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// AssertKeyExists checks a key exists at the given path.
//
// Example:
//
//	v.AssertKeyExists(t, []string{"Software", "MyApp"})
func (v *Validator) AssertKeyExists(t *testing.T, path []string) {
	t.Helper()
	_, err := v.GetKey(path)
	if err != nil {
		t.Errorf("Key '%s' should exist but doesn't: %v", strings.Join(path, "\\"), err)
	}
}

// AssertKeyNotExists checks a key does NOT exist at the given path.
func (v *Validator) AssertKeyNotExists(t *testing.T, path []string) {
	t.Helper()
	_, err := v.GetKey(path)
	if err == nil {
		t.Errorf("Key '%s' should not exist but does", strings.Join(path, "\\"))
	}
}

// AssertValueExists checks a value exists in a key.
//
// Example:
//
//	v.AssertValueExists(t, []string{"Software", "MyApp"}, "Version")
func (v *Validator) AssertValueExists(t *testing.T, keyPath []string, valueName string) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	_, err = v.GetValue(key, valueName)
	if err != nil {
		t.Errorf("Value '%s' should exist in key '%s' but doesn't: %v",
			valueName, strings.Join(keyPath, "\\"), err)
	}
}

// AssertValueNotExists checks a value does NOT exist.
func (v *Validator) AssertValueNotExists(t *testing.T, keyPath []string, valueName string) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		// Key doesn't exist, so value doesn't exist either
		return
	}

	_, err = v.GetValue(key, valueName)
	if err == nil {
		t.Errorf("Value '%s' should not exist in key '%s' but does",
			valueName, strings.Join(keyPath, "\\"))
	}
}

// AssertKeyCount checks total key count matches expected.
//
// Example:
//
//	v.AssertKeyCount(t, 42)
func (v *Validator) AssertKeyCount(t *testing.T, expected int) {
	t.Helper()
	count, err := v.CountKeys()
	if err != nil {
		t.Fatalf("Cannot count keys: %v", err)
	}

	if count != expected {
		t.Errorf("Expected %d keys, found %d", expected, count)
	}
}

// AssertValueCount checks total value count matches expected.
//
// Example:
//
//	v.AssertValueCount(t, 100)
func (v *Validator) AssertValueCount(t *testing.T, expected int) {
	t.Helper()
	count, err := v.CountValues()
	if err != nil {
		t.Fatalf("Cannot count values: %v", err)
	}

	if count != expected {
		t.Errorf("Expected %d values, found %d", expected, count)
	}
}

// AssertSubkeyCount checks a key has expected number of children.
//
// Example:
//
//	v.AssertSubkeyCount(t, []string{"Software"}, 5)
func (v *Validator) AssertSubkeyCount(t *testing.T, keyPath []string, expected int) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	count, err := v.GetSubkeyCount(key)
	if err != nil {
		t.Fatalf("Cannot get subkey count: %v", err)
	}

	if count != expected {
		t.Errorf("Key '%s' should have %d children, found %d",
			strings.Join(keyPath, "\\"), expected, count)
	}
}

// AssertValueType checks value has expected type.
//
// Example:
//
//	v.AssertValueType(t, []string{"Software", "MyApp"}, "Timeout", "REG_DWORD")
func (v *Validator) AssertValueType(t *testing.T, keyPath []string, valueName string, expectedType string) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	val, err := v.GetValue(key, valueName)
	if err != nil {
		t.Fatalf("Cannot get value '%s': %v", valueName, err)
	}

	typ, err := v.GetValueType(val)
	if err != nil {
		t.Fatalf("Cannot get value type: %v", err)
	}

	// Normalize type strings for comparison
	expected := strings.ToUpper(expectedType)
	actual := strings.ToUpper(typ)

	if actual != expected {
		t.Errorf("Value '%s' in key '%s' should have type %s, found %s",
			valueName, strings.Join(keyPath, "\\"), expectedType, typ)
	}
}

// AssertValueData checks value has expected raw bytes.
//
// Example:
//
//	v.AssertValueData(t, []string{"Software", "MyApp"}, "Binary", []byte{0x01, 0x02, 0x03})
func (v *Validator) AssertValueData(t *testing.T, keyPath []string, valueName string, expected []byte) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	val, err := v.GetValue(key, valueName)
	if err != nil {
		t.Fatalf("Cannot get value '%s': %v", valueName, err)
	}

	data, err := v.GetValueData(val)
	if err != nil {
		t.Fatalf("Cannot get value data: %v", err)
	}

	if !reflect.DeepEqual(data, expected) {
		t.Errorf("Value '%s' in key '%s' has wrong data\nExpected: %v\nActual: %v",
			valueName, strings.Join(keyPath, "\\"), expected, data)
	}
}

// AssertValueString checks string value matches expected.
//
// Example:
//
//	v.AssertValueString(t, []string{"Software", "MyApp"}, "Version", "1.0.0")
func (v *Validator) AssertValueString(t *testing.T, keyPath []string, valueName string, expected string) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	val, err := v.GetValue(key, valueName)
	if err != nil {
		t.Fatalf("Cannot get value '%s': %v", valueName, err)
	}

	str, err := v.GetValueString(val)
	if err != nil {
		t.Fatalf("Cannot get value string: %v", err)
	}

	if str != expected {
		t.Errorf("Value '%s' in key '%s' should be '%s', found '%s'",
			valueName, strings.Join(keyPath, "\\"), expected, str)
	}
}

// AssertValueDWORD checks DWORD value matches expected.
//
// Example:
//
//	v.AssertValueDWORD(t, []string{"Software", "MyApp"}, "Timeout", 30)
func (v *Validator) AssertValueDWORD(t *testing.T, keyPath []string, valueName string, expected uint32) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	val, err := v.GetValue(key, valueName)
	if err != nil {
		t.Fatalf("Cannot get value '%s': %v", valueName, err)
	}

	dw, err := v.GetValueDWORD(val)
	if err != nil {
		t.Fatalf("Cannot get value DWORD: %v", err)
	}

	if dw != expected {
		t.Errorf("Value '%s' in key '%s' should be %d, found %d",
			valueName, strings.Join(keyPath, "\\"), expected, dw)
	}
}

// AssertValueQWORD checks QWORD value matches expected.
//
// Example:
//
//	v.AssertValueQWORD(t, []string{"Software", "MyApp"}, "Counter", 9876543210)
func (v *Validator) AssertValueQWORD(t *testing.T, keyPath []string, valueName string, expected uint64) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	val, err := v.GetValue(key, valueName)
	if err != nil {
		t.Fatalf("Cannot get value '%s': %v", valueName, err)
	}

	qw, err := v.GetValueQWORD(val)
	if err != nil {
		t.Fatalf("Cannot get value QWORD: %v", err)
	}

	if qw != expected {
		t.Errorf("Value '%s' in key '%s' should be %d, found %d",
			valueName, strings.Join(keyPath, "\\"), expected, qw)
	}
}

// AssertValueStrings checks MULTI_SZ value matches expected.
//
// Example:
//
//	v.AssertValueStrings(t, []string{"Software", "MyApp"}, "Features", []string{"A", "B", "C"})
func (v *Validator) AssertValueStrings(t *testing.T, keyPath []string, valueName string, expected []string) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	val, err := v.GetValue(key, valueName)
	if err != nil {
		t.Fatalf("Cannot get value '%s': %v", valueName, err)
	}

	strs, err := v.GetValueStrings(val)
	if err != nil {
		t.Fatalf("Cannot get value strings: %v", err)
	}

	if !reflect.DeepEqual(strs, expected) {
		t.Errorf("Value '%s' in key '%s' has wrong strings\nExpected: %v\nActual: %v",
			valueName, strings.Join(keyPath, "\\"), expected, strs)
	}
}

// AssertStructureValid checks hive structure is valid.
//
// Example:
//
//	v.AssertStructureValid(t)
func (v *Validator) AssertStructureValid(t *testing.T) {
	t.Helper()
	if err := v.ValidateStructure(); err != nil {
		t.Errorf("Hive structure is invalid: %v", err)
	}
}

// AssertHivexshValid checks hivexsh validation passes.
//
// Example:
//
//	v.AssertHivexshValid(t)  // Fails test if hivexsh -d fails
func (v *Validator) AssertHivexshValid(t *testing.T) {
	t.Helper()

	// Check if hivexsh is available
	if !IsHivexshAvailable() {
		if v.opts.SkipIfHivexshUnavailable {
			t.Skip("hivexsh not available")
		} else {
			t.Fatal("hivexsh command not found")
		}
		return
	}

	if err := v.ValidateWithHivexsh(); err != nil {
		// Extract output from HivexshError if available
		var hivexErr *HivexshError
		if errors.As(err, &hivexErr) {
			t.Errorf("Hivexsh validation failed:\n%s", hivexErr.Output())
		} else {
			t.Errorf("Hivexsh validation failed: %v", err)
		}
	}
}

// AssertMatchesValidator asserts this validator matches another.
//
// This performs a deep comparison of the entire tree structure
// and all values between two validators.
//
// Example:
//
//	v1 := hivexval.Must(hivexval.New(path, &hivexval.Options{UseBindings: true}))
//	v2 := hivexval.Must(hivexval.New(path, &hivexval.Options{UseReader: true}))
//	v1.AssertMatchesValidator(t, v2)
func (v *Validator) AssertMatchesValidator(t *testing.T, other *Validator) {
	t.Helper()

	result, err := v.Compare(other)
	if err != nil {
		t.Fatalf("Comparison failed: %v", err)
	}

	if !result.Match {
		t.Errorf("Validators do not match (%d mismatches found):", len(result.Mismatches))
		for i, m := range result.Mismatches {
			if i >= 10 {
				t.Errorf("... and %d more mismatches", len(result.Mismatches)-10)
				break
			}
			t.Errorf("  [%s] %s: %s", m.Category, m.Path, m.Message)
			if m.Expected != nil || m.Actual != nil {
				t.Errorf("    Expected: %v", m.Expected)
				t.Errorf("    Actual:   %v", m.Actual)
			}
		}
	}
}

// AssertTreeStructure checks the tree has expected structure.
//
// This is a convenience method that checks both key and value counts.
//
// Example:
//
//	v.AssertTreeStructure(t, 42, 100)  // 42 keys, 100 values
func (v *Validator) AssertTreeStructure(t *testing.T, expectedKeys int, expectedValues int) {
	t.Helper()
	v.AssertKeyCount(t, expectedKeys)
	v.AssertValueCount(t, expectedValues)
}

// AssertValidationPasses performs comprehensive validation and fails on any error.
//
// This is equivalent to calling Validate() and checking all results.
//
// Example:
//
//	v.AssertValidationPasses(t)
func (v *Validator) AssertValidationPasses(t *testing.T) {
	t.Helper()

	result, err := v.Validate()
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if !result.StructureValid {
		t.Errorf("Structure validation failed:")
		for _, err := range result.Errors {
			t.Errorf("  - %s", err)
		}
	}

	if v.opts.UseHivexsh && !result.HivexshPassed {
		t.Errorf("Hivexsh validation failed:")
		for _, err := range result.Errors {
			if strings.Contains(err, "hivexsh") {
				t.Errorf("  - %s", err)
			}
		}
	}

	if len(result.Warnings) > 0 {
		t.Logf("Validation warnings:")
		for _, warn := range result.Warnings {
			t.Logf("  - %s", warn)
		}
	}
}

// RequireKeyExists is like AssertKeyExists but calls t.Fatal on failure.
func (v *Validator) RequireKeyExists(t *testing.T, path []string) {
	t.Helper()
	_, err := v.GetKey(path)
	if err != nil {
		t.Fatalf("Key '%s' must exist: %v", strings.Join(path, "\\"), err)
	}
}

// RequireValueExists is like AssertValueExists but calls t.Fatal on failure.
func (v *Validator) RequireValueExists(t *testing.T, keyPath []string, valueName string) {
	t.Helper()
	key, err := v.GetKey(keyPath)
	if err != nil {
		t.Fatalf("Cannot get key '%s': %v", strings.Join(keyPath, "\\"), err)
	}

	_, err = v.GetValue(key, valueName)
	if err != nil {
		t.Fatalf("Value '%s' must exist in key '%s': %v",
			valueName, strings.Join(keyPath, "\\"), err)
	}
}
