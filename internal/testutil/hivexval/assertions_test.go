package hivexval

import (
	"testing"
)

// TestAssertions tests all assertion methods.
func TestAssertions(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	t.Run("AssertKeyExists", func(t *testing.T) {
		// Should pass
		v.AssertKeyExists(t, []string{"Software"})
		v.AssertKeyExists(t, []string{"Software", "Company1"})
		v.AssertKeyExists(t, []string{"Software", "Company1", "App1"})

		// Should fail but we can't test that easily without a mock *testing.T
	})

	t.Run("AssertValueExists", func(t *testing.T) {
		v.AssertValueExists(t, []string{"Software", "Company1", "App1"}, "Name")
		v.AssertValueExists(t, []string{"Software", "Company1", "App1"}, "Version")
		v.AssertValueExists(t, []string{"Software", "Company1", "App1"}, "Timeout")
	})

	t.Run("AssertKeyCount", func(t *testing.T) {
		v.AssertKeyCount(t, 9)
	})

	t.Run("AssertValueCount", func(t *testing.T) {
		v.AssertValueCount(t, 8)
	})

	t.Run("AssertSubkeyCount", func(t *testing.T) {
		v.AssertSubkeyCount(t, []string{"Software"}, 2)             // Company1, Company2
		v.AssertSubkeyCount(t, []string{"Software", "Company1"}, 2) // App1, App2
	})

	t.Run("AssertValueType", func(t *testing.T) {
		v.AssertValueType(t, []string{"Software", "Company1", "App1"}, "Name", "REG_SZ")
		v.AssertValueType(t, []string{"Software", "Company1", "App1"}, "Timeout", "REG_DWORD")
		v.AssertValueType(t, []string{"Software", "Company1", "App1"}, "Counter", "REG_QWORD")
		v.AssertValueType(t, []string{"Software", "Company2", "App1"}, "Features", "REG_MULTI_SZ")
		v.AssertValueType(t, []string{"System", "Config"}, "Data", "REG_BINARY")
	})

	t.Run("AssertValueString", func(t *testing.T) {
		v.AssertValueString(t, []string{"Software", "Company1", "App1"}, "Name", "TestApp1")
		v.AssertValueString(t, []string{"Software", "Company1", "App1"}, "Version", "1.0.0")
		v.AssertValueString(t, []string{"Software", "Company1", "App2"}, "Name", "TestApp2")
	})

	t.Run("AssertValueDWORD", func(t *testing.T) {
		v.AssertValueDWORD(t, []string{"Software", "Company1", "App1"}, "Timeout", 30)
		v.AssertValueDWORD(t, []string{"Software", "Company1", "App2"}, "Enabled", 1)
	})

	t.Run("AssertValueQWORD", func(t *testing.T) {
		v.AssertValueQWORD(t, []string{"Software", "Company1", "App1"}, "Counter", 9876543210)
	})

	t.Run("AssertValueStrings", func(t *testing.T) {
		v.AssertValueStrings(t, []string{"Software", "Company2", "App1"}, "Features", []string{"A", "B", "C"})
	})

	t.Run("AssertValueData", func(t *testing.T) {
		v.AssertValueData(t, []string{"System", "Config"}, "Data", []byte{0x01, 0x02, 0x03})
	})

	t.Run("AssertStructureValid", func(t *testing.T) {
		v.AssertStructureValid(t)
	})

	t.Run("AssertTreeStructure", func(t *testing.T) {
		v.AssertTreeStructure(t, 9, 8)
	})

	t.Run("AssertValidationPasses", func(t *testing.T) {
		v.AssertValidationPasses(t)
	})
}

// TestAssertHivexshValid tests hivexsh validation assertion.
func TestAssertHivexshValid(t *testing.T) {
	if !IsHivexshAvailable() {
		t.Skip("hivexsh not available")
	}

	hivePath := createTestHive(t)
	v := Must(New(hivePath, &Options{UseHivexsh: true}))
	defer v.Close()

	v.AssertHivexshValid(t)
}

// TestRequireKeyExists tests the Require variant.
func TestRequireKeyExists(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	v.RequireKeyExists(t, []string{"Software"})
	v.RequireKeyExists(t, []string{"Software", "Company1", "App1"})
}

// TestRequireValueExists tests the Require variant for values.
func TestRequireValueExists(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	v.RequireValueExists(t, []string{"Software", "Company1", "App1"}, "Name")
	v.RequireValueExists(t, []string{"Software", "Company1", "App1"}, "Version")
}
