package e2e

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive/edit"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_Integration_RealWorld_SoftwareInstall simulates a typical software installation scenario
// Creates: Software\Vendor\Product with typical registry values.
func Test_Integration_RealWorld_SoftwareInstall(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Simulate installing software: Software\TestVendor\TestProduct
	vendorPath := []string{"Software", "TestVendor"}
	vendorRef, keysCreated, err := keyEditor.EnsureKeyPath(rootRef, vendorPath)
	if err != nil {
		t.Fatalf("Failed to create vendor key: %v", err)
	}
	if keysCreated == 0 {
		t.Log("Vendor key already exists (OK for repeated runs)")
	}

	productPath := []string{"Software", "TestVendor", "TestProduct", "1.0"}
	productRef, _, err := keyEditor.EnsureKeyPath(rootRef, productPath)
	if err != nil {
		t.Fatalf("Failed to create product key: %v", err)
	}

	// Add typical installation values
	testCases := []struct {
		name      string
		valueType edit.ValueType
		data      []byte
	}{
		{"InstallPath", format.REGSZ, []byte("C:\\Program Files\\TestProduct\x00")},
		{"Version", format.REGSZ, []byte("1.0.0.0\x00")},
		{"InstallDate", format.REGDWORD, []byte{0x01, 0x02, 0x03, 0x04}},
		{"Publisher", format.REGSZ, []byte("Test Vendor Inc.\x00")},
		{"UninstallString", format.REGSZ, []byte("C:\\Program Files\\TestProduct\\uninstall.exe\x00")},
		{"DisplayIcon", format.REGSZ, []byte("C:\\Program Files\\TestProduct\\app.exe,0\x00")},
		{"DisplayName", format.REGSZ, []byte("Test Product\x00")},
		{"NoModify", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00}},
		{"NoRepair", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00}},
		{"EstimatedSize", format.REGDWORD, []byte{0x00, 0x10, 0x00, 0x00}}, // 4MB
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			upsertErr := valueEditor.UpsertValue(productRef, tc.name, tc.valueType, tc.data)
			if upsertErr != nil {
				t.Errorf("Failed to set %s: %v", tc.name, upsertErr)
			}

			// Verify it exists in index
			_, ok := idx.GetVK(productRef, strings.ToLower(tc.name))
			if !ok {
				t.Errorf("Value %s not found in index after creation", tc.name)
			}
		})
	}

	// Verify vendor key has the product as a subkey
	_, ok := idx.GetNK(vendorRef, "testproduct")
	if !ok {
		t.Error("Product key not found as subkey of vendor")
	}

	t.Logf("Successfully simulated software installation with %d registry values", len(testCases))
}

// Test_Integration_RealWorld_ServiceConfiguration simulates Windows service configuration.
func Test_Integration_RealWorld_ServiceConfiguration(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create: System\CurrentControlSet\Services\TestService
	servicePath := []string{"System", "CurrentControlSet", "Services", "TestService"}
	serviceRef, _, err := keyEditor.EnsureKeyPath(rootRef, servicePath)
	if err != nil {
		t.Fatalf("Failed to create service key: %v", err)
	}

	// Service configuration values
	valueEditor.UpsertValue(
		serviceRef,
		"Type",
		format.REGDWORD,
		[]byte{0x10, 0x00, 0x00, 0x00},
	) // SERVICE_WIN32_OWN_PROCESS
	valueEditor.UpsertValue(
		serviceRef,
		"Start",
		format.REGDWORD,
		[]byte{0x02, 0x00, 0x00, 0x00},
	) // SERVICE_AUTO_START
	valueEditor.UpsertValue(
		serviceRef,
		"ErrorControl",
		format.REGDWORD,
		[]byte{0x01, 0x00, 0x00, 0x00},
	) // SERVICE_ERROR_NORMAL
	valueEditor.UpsertValue(serviceRef, "ImagePath", format.REGSZ, []byte("C:\\Windows\\System32\\test.exe\x00"))
	valueEditor.UpsertValue(serviceRef, "DisplayName", format.REGSZ, []byte("Test Service\x00"))
	valueEditor.UpsertValue(serviceRef, "ObjectName", format.REGSZ, []byte("LocalSystem\x00"))
	valueEditor.UpsertValue(serviceRef, "Description", format.REGSZ, []byte("Test service for integration testing\x00"))

	// Create Parameters subkey (common pattern)
	paramsPath := append([]string(nil), servicePath...)
	paramsPath = append(paramsPath, "Parameters")
	paramsRef, _, err := keyEditor.EnsureKeyPath(rootRef, paramsPath)
	if err != nil {
		t.Fatalf("Failed to create Parameters key: %v", err)
	}

	valueEditor.UpsertValue(paramsRef, "Port", format.REGDWORD, []byte{0x50, 0x00, 0x00, 0x00})           // Port 80
	valueEditor.UpsertValue(paramsRef, "MaxConnections", format.REGDWORD, []byte{0x64, 0x00, 0x00, 0x00}) // 100

	// Verify structure
	_, ok := idx.GetNK(serviceRef, "parameters")
	if !ok {
		t.Error("Parameters subkey not found")
	}

	t.Log("Successfully simulated Windows service configuration")
}

// Test_Integration_RealWorld_UserPreferences simulates user preferences storage.
func Test_Integration_RealWorld_UserPreferences(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create: Software\TestApp\UserPreferences
	prefsPath := []string{"Software", "TestApp", "UserPreferences"}
	prefsRef, _, err := keyEditor.EnsureKeyPath(rootRef, prefsPath)
	if err != nil {
		t.Fatalf("Failed to create preferences key: %v", err)
	}

	// Various preference types
	valueEditor.UpsertValue(prefsRef, "Theme", format.REGSZ, []byte("Dark\x00"))
	valueEditor.UpsertValue(prefsRef, "FontSize", format.REGDWORD, []byte{0x0C, 0x00, 0x00, 0x00}) // 12
	valueEditor.UpsertValue(prefsRef, "AutoSave", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00}) // True
	valueEditor.UpsertValue(
		prefsRef,
		"RecentFiles",
		format.REGMultiSZ,
		[]byte("file1.txt\x00file2.txt\x00file3.txt\x00\x00"),
	)

	// Binary data (e.g., window position)
	windowPos := make([]byte, 16)
	windowPos[0] = 0x64 // X
	windowPos[4] = 0x64 // Y
	windowPos[8] = 0x00
	windowPos[9] = 0x03 // Width 768
	windowPos[12] = 0x00
	windowPos[13] = 0x04 // Height 1024
	valueEditor.UpsertValue(prefsRef, "WindowPosition", format.REGBinary, windowPos)

	// Create subkeys for different sections
	for _, section := range []string{"UI", "Network", "Advanced"} {
		sectionPath := append([]string(nil), prefsPath...)
		sectionPath = append(sectionPath, section)
		sectionRef, _, ensureErr := keyEditor.EnsureKeyPath(rootRef, sectionPath)
		if ensureErr != nil {
			t.Errorf("Failed to create %s section: %v", section, ensureErr)
			continue
		}

		// Add a few values to each section
		valueEditor.UpsertValue(sectionRef, "Enabled", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})
		valueEditor.UpsertValue(sectionRef, "Setting1", format.REGSZ, []byte("Value1\x00"))
	}

	t.Log("Successfully simulated user preferences storage")
}

// Test_Integration_RealWorld_MultipleUpdates tests realistic update patterns.
func Test_Integration_RealWorld_MultipleUpdates(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create key
	path := []string{"Software", "TestApp", "Config"}
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	// Initial values
	valueEditor.UpsertValue(keyRef, "Version", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})
	valueEditor.UpsertValue(keyRef, "Status", format.REGSZ, []byte("Initial\x00"))

	// Simulate 10 updates (like application saving state periodically)
	for i := 1; i <= 10; i++ {
		// Update version counter
		version := []byte{byte(i), 0x00, 0x00, 0x00}
		upsertErr := valueEditor.UpsertValue(keyRef, "Version", format.REGDWORD, version)
		if upsertErr != nil {
			t.Errorf("Update %d failed: %v", i, upsertErr)
		}

		// Update status with growing data
		status := fmt.Sprintf("Update_%d\x00", i)
		upsertErr = valueEditor.UpsertValue(keyRef, "Status", format.REGSZ, []byte(status))
		if upsertErr != nil {
			t.Errorf("Status update %d failed: %v", i, upsertErr)
		}

		// Add a new value each iteration (simulating log entries)
		logEntry := fmt.Sprintf("LogEntry_%d", i)
		logData := []byte(fmt.Sprintf("Event at iteration %d\x00", i))
		err = valueEditor.UpsertValue(keyRef, logEntry, format.REGSZ, logData)
		if err != nil {
			t.Errorf("Log entry %d failed: %v", i, err)
		}
	}

	// Verify final state
	versionRef, ok := idx.GetVK(keyRef, "version")
	if !ok {
		t.Error("Version value not found after updates")
	} else {
		t.Logf("Version value ref: 0x%X", versionRef)
	}

	t.Log("Successfully completed multiple update simulation")
}

// Test_Integration_RealWorld_LargeValueSet tests creating many values under a single key.
func Test_Integration_RealWorld_LargeValueSet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large value set test in short mode")
	}

	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create key
	path := []string{"Software", "TestApp", "LargeValueSet"}
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	// Create 50 values (realistic for some registry keys like environment variables)
	numValues := 50
	data := []byte{0x01, 0x02, 0x03, 0x04}

	for i := range numValues {
		name := fmt.Sprintf("Value_%03d", i)
		upsertErr := valueEditor.UpsertValue(keyRef, name, format.REGDWORD, data)
		if upsertErr != nil {
			t.Errorf("Failed to create value %d: %v", i, upsertErr)
		}
	}

	// Verify all values exist
	successCount := 0
	for i := range numValues {
		name := strings.ToLower(fmt.Sprintf("Value_%03d", i))
		if _, ok := idx.GetVK(keyRef, name); ok {
			successCount++
		}
	}

	if successCount != numValues {
		t.Errorf("Expected %d values in index, found %d", numValues, successCount)
	}

	t.Logf("Successfully created %d values under a single key", numValues)
}

// Test_Integration_RealWorld_DeepHierarchy tests creating deep registry hierarchies.
func Test_Integration_RealWorld_DeepHierarchy(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create a 10-level deep hierarchy (realistic for some registry structures)
	depth := 10
	path := []string{"Software", "TestApp"}
	for i := range depth {
		path = append(path, fmt.Sprintf("Level%d", i))
	}

	leafRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		t.Fatalf("Failed to create deep hierarchy: %v", err)
	}

	// Add value at the deepest level
	err = valueEditor.UpsertValue(leafRef, "DeepValue", format.REGSZ, []byte("At maximum depth\x00"))
	if err != nil {
		t.Errorf("Failed to add value at deep level: %v", err)
	}

	// Verify we can navigate back up
	currentRef := rootRef
	for i, segment := range path {
		nextRef, ok := idx.GetNK(currentRef, strings.ToLower(segment))
		if !ok {
			t.Errorf("Failed to find key '%s' at level %d", segment, i)
			break
		}
		currentRef = nextRef
	}

	if currentRef != leafRef {
		t.Errorf("Navigation mismatch: expected 0x%X, got 0x%X", leafRef, currentRef)
	}

	t.Logf("Successfully created and navigated %d-level deep hierarchy", depth)
}

// Test_Integration_RealWorld_MixedOperations tests realistic mixed operations.
func Test_Integration_RealWorld_MixedOperations(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create base structure
	basePath := []string{"Software", "TestApp", "Mixed"}
	baseRef, _, err := keyEditor.EnsureKeyPath(rootRef, basePath)
	if err != nil {
		t.Fatalf("Failed to create base key: %v", err)
	}

	// Mixed operations simulating real-world usage
	operations := []struct {
		name string
		op   func() error
	}{
		{"Create value A", func() error {
			return valueEditor.UpsertValue(baseRef, "ValueA", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}},
		{"Create value B", func() error {
			return valueEditor.UpsertValue(baseRef, "ValueB", format.REGSZ, []byte("Test\x00"))
		}},
		{"Update value A", func() error {
			return valueEditor.UpsertValue(baseRef, "ValueA", format.REGDWORD, []byte{0x02, 0x00, 0x00, 0x00})
		}},
		{"Create subkey", func() error {
			subPath := append([]string(nil), basePath...)
			subPath = append(subPath, "SubKey1")
			_, _, ensureErr := keyEditor.EnsureKeyPath(rootRef, subPath)
			return ensureErr
		}},
		{"Create value C with larger data", func() error {
			data := bytes.Repeat([]byte{0xAB}, 512)
			return valueEditor.UpsertValue(baseRef, "ValueC", format.REGBinary, data)
		}},
		{"Update value B to larger data", func() error {
			return valueEditor.UpsertValue(baseRef, "ValueB", format.REGSZ, []byte("LongerTestString\x00"))
		}},
		{"Create value D", func() error {
			return valueEditor.UpsertValue(baseRef, "ValueD", format.REGBinary, []byte{0xFF, 0xFE})
		}},
		{"Create another subkey", func() error {
			subPath := append([]string(nil), basePath...)
			subPath = append(subPath, "SubKey2")
			subRef, _, ensureErr := keyEditor.EnsureKeyPath(rootRef, subPath)
			if ensureErr != nil {
				return ensureErr
			}
			// Add value to subkey
			return valueEditor.UpsertValue(subRef, "SubValue", format.REGDWORD, []byte{0x99, 0x00, 0x00, 0x00})
		}},
		{"Update value C to big-data", func() error {
			bigData := bytes.Repeat([]byte{0xCD}, 20*1024) // 20KB
			return valueEditor.UpsertValue(baseRef, "ValueC", format.REGBinary, bigData)
		}},
		{"Create value E with QWORD data", func() error {
			return valueEditor.UpsertValue(
				baseRef,
				"ValueE",
				format.REGQWORD,
				[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			)
		}},
	}

	for i, op := range operations {
		t.Run(fmt.Sprintf("Op%d_%s", i+1, op.name), func(t *testing.T) {
			opErr := op.op()
			if opErr != nil {
				t.Errorf("Operation '%s' failed: %v", op.name, opErr)
			}
		})
	}

	// Final verification
	_, okA := idx.GetVK(baseRef, "valuea")
	_, okB := idx.GetVK(baseRef, "valueb")
	_, okC := idx.GetVK(baseRef, "valuec")
	_, okD := idx.GetVK(baseRef, "valued")
	_, okE := idx.GetVK(baseRef, "valuee")

	if !okA {
		t.Error("ValueA should exist after operations")
	}
	if !okB {
		t.Error("ValueB should exist after updates")
	}
	if !okC {
		t.Error("ValueC should exist after big-data update")
	}
	if !okD {
		t.Error("ValueD should exist after creation")
	}
	if !okE {
		t.Error("ValueE should exist after creation")
	}

	t.Log("Successfully completed mixed operations test")
}

// Test_Integration_RealWorld_DefaultValues tests the default value (empty name) pattern.
func Test_Integration_RealWorld_DefaultValues(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create keys and set their default values
	testKeys := []struct {
		path         []string
		defaultValue string
	}{
		{[]string{"Software", "Test1"}, "Default for Test1"},
		{[]string{"Software", "Test2"}, "Default for Test2"},
		{[]string{"Software", "Test1", "SubKey"}, "Default for SubKey"},
	}

	for _, tk := range testKeys {
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, tk.path)
		if err != nil {
			t.Errorf("Failed to create key %v: %v", tk.path, err)
			continue
		}

		// Set default value (empty name)
		data := []byte(tk.defaultValue + "\x00")
		err = valueEditor.UpsertValue(keyRef, "", format.REGSZ, data)
		if err != nil {
			t.Errorf("Failed to set default value for %v: %v", tk.path, err)
		}

		// Verify it exists with empty name
		_, ok := idx.GetVK(keyRef, "")
		if !ok {
			t.Errorf("Default value not found for %v", tk.path)
		}
	}

	t.Log("Successfully tested default value pattern")
}
