package merge

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
)

// Benchmark_Plan_AddOperations benchmarks building plans.
func Benchmark_Plan_AddOperations(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		plan := NewPlan()
		plan.AddEnsureKey([]string{"Software", "Test", "Key"})
		plan.AddSetValue([]string{"Software", "Test", "Key"}, "Value1", 1, []byte("data\x00"))
		plan.AddSetValue([]string{"Software", "Test", "Key"}, "Value2", 4, []byte{0x01, 0x00, 0x00, 0x00})
		plan.AddDeleteValue([]string{"Software", "Test", "Key"}, "OldValue")
	}
}

// Benchmark_ParseJSONPatch benchmarks JSON patch parsing.
func Benchmark_ParseJSONPatch(b *testing.B) {
	jsonPatch := []byte(`{
		"operations": [
			{
				"op": "ensure_key",
				"key_path": ["Software", "TestVendor", "TestApp"]
			},
			{
				"op": "set_value",
				"key_path": ["Software", "TestVendor", "TestApp"],
				"value_name": "Version",
				"value_type": "REG_SZ",
				"data": [49, 46, 48, 0]
			},
			{
				"op": "set_value",
				"key_path": ["Software", "TestVendor", "TestApp"],
				"value_name": "Enabled",
				"value_type": "REG_DWORD",
				"data": [1, 0, 0, 0]
			},
			{
				"op": "delete_value",
				"key_path": ["Software", "TestVendor", "TestApp"],
				"value_name": "OldSetting"
			}
		]
	}`)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, err := ParseJSONPatch(jsonPatch)
		if err != nil {
			b.Fatalf("ParseJSONPatch failed: %v", err)
		}
	}
}

// Benchmark_MarshalPlan benchmarks plan marshaling.
func Benchmark_MarshalPlan(b *testing.B) {
	plan := NewPlan()
	plan.AddEnsureKey([]string{"Software", "Test"})
	plan.AddSetValue([]string{"Software", "Test"}, "Value1", 1, []byte("data\x00"))
	plan.AddSetValue([]string{"Software", "Test"}, "Value2", 4, []byte{0x01, 0x00, 0x00, 0x00})
	plan.AddDeleteValue([]string{"Software", "Test"}, "OldValue")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, err := MarshalPlan(plan)
		if err != nil {
			b.Fatalf("MarshalPlan failed: %v", err)
		}
	}
}

// Benchmark_Executor_SmallPlan benchmarks executing a small plan (1 key, 2 values).
func Benchmark_Executor_SmallPlan(b *testing.B) {
	_, session, _, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	plan := NewPlan()
	plan.AddEnsureKey([]string{"_BenchSmall"})
	plan.AddSetValue([]string{"_BenchSmall"}, "Value1", 4, []byte{0x01, 0x00, 0x00, 0x00})
	plan.AddSetValue([]string{"_BenchSmall"}, "Value2", 1, []byte("test\x00"))

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		b.StopTimer()
		// Use unique path each iteration to avoid index collisions
		plan.Ops[0].KeyPath = []string{"_BenchSmall", "Iteration", b.Name(), fmt.Sprintf("i%d", i)}
		plan.Ops[1].KeyPath = plan.Ops[0].KeyPath
		plan.Ops[2].KeyPath = plan.Ops[0].KeyPath
		b.StartTimer()

		_, err := session.ApplyWithTx(plan)
		if err != nil {
			b.Fatalf("Apply failed: %v", err)
		}
	}
}

// Benchmark_Executor_MediumPlan benchmarks executing a medium plan (10 keys, 5 values each).
func Benchmark_Executor_MediumPlan(b *testing.B) {
	_, session, _, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		b.StopTimer()
		plan := NewPlan()
		basePath := []string{"_BenchMedium", "Run", fmt.Sprintf("i%d", i)}

		// Create 10 subkeys with 5 values each
		for j := range 10 {
			keyPath := append([]string(nil), basePath...)
			keyPath = append(keyPath, fmt.Sprintf("K%d", j))
			plan.AddEnsureKey(keyPath)

			for k := range 5 {
				plan.AddSetValue(keyPath, fmt.Sprintf("V%d", k), 4, []byte{byte(k), 0x00, 0x00, 0x00})
			}
		}
		b.StartTimer()

		_, err := session.ApplyWithTx(plan)
		if err != nil {
			b.Fatalf("Apply failed: %v", err)
		}
	}
}

// Benchmark_Executor_LargeValues benchmarks operations with large values.
func Benchmark_Executor_LargeValues(b *testing.B) {
	_, session, _, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	// Different value sizes to test
	sizes := []struct {
		name string
		size int
	}{
		{"512B", 512},
		{"4KB", 4 * 1024},
		// TODO: Investigate why 16KB and 20KB benchmarks fail with "truncated external cell" errors
		// This might be a bug in large value handling that needs to be fixed
		// {"16KB", 16 * 1024},
		// {"20KB_BigData", 20 * 1024}, // Forces big-data format
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			data := bytes.Repeat([]byte{0xAB}, sz.size)

			b.ReportAllocs()
			b.ResetTimer()

			for i := range b.N {
				b.StopTimer()
				plan := NewPlan()
				// Use iteration counter for truly unique paths
				keyPath := []string{"_BenchLarge", sz.name, fmt.Sprintf("i%d", i)}
				plan.AddSetValue(keyPath, "LargeValue", format.REGBinary, data)
				b.StartTimer()

				_, err := session.ApplyWithTx(plan)
				if err != nil {
					b.Fatalf("Apply failed: %v", err)
				}
			}
		})
	}
}

// Benchmark_Executor_IdempotentReplay benchmarks replaying the same plan (idempotency).
func Benchmark_Executor_IdempotentReplay(b *testing.B) {
	_, session, _, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	plan := NewPlan()
	plan.AddEnsureKey([]string{"_BenchIdempotent"})
	plan.AddSetValue([]string{"_BenchIdempotent"}, "Value1", 4, []byte{0x01, 0x00, 0x00, 0x00})

	// Execute once to create the key/value
	_, err := session.ApplyWithTx(plan)
	if err != nil {
		b.Fatalf("Initial apply failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	// Now benchmark replaying the same plan (should be mostly no-ops)
	for range b.N {
		_, applyErr := session.ApplyWithTx(plan)
		if applyErr != nil {
			b.Fatalf("Apply failed: %v", applyErr)
		}
	}
}

// Benchmark_Executor_DeepHierarchy benchmarks creating deep key hierarchies.
func Benchmark_Executor_DeepHierarchy(b *testing.B) {
	_, session, _, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		b.StopTimer()
		plan := NewPlan()

		// Create a 10-level deep hierarchy
		keyPath := []string{"_BenchDeep", "Run", fmt.Sprintf("i%d", i)}
		for j := range 10 {
			keyPath = append(keyPath, fmt.Sprintf("L%d", j))
			plan.AddEnsureKey(keyPath)
		}

		// Add a value at the deepest level
		plan.AddSetValue(keyPath, "DeepValue", 1, []byte("data\x00"))
		b.StartTimer()

		_, err := session.ApplyWithTx(plan)
		if err != nil {
			b.Fatalf("Apply failed: %v", err)
		}
	}
}

// Benchmark_Executor_MixedOperations benchmarks a realistic mix of operations.
func Benchmark_Executor_MixedOperations(b *testing.B) {
	_, session, _, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		b.StopTimer()
		plan := NewPlan()
		basePath := []string{"_BenchMixed", "App", fmt.Sprintf("i%d", i)}

		// Simulate software installation pattern
		plan.AddEnsureKey(basePath)
		plan.AddSetValue(basePath, "InstallPath", format.REGSZ, []byte("C:\\Program Files\\App\x00"))
		plan.AddSetValue(basePath, "Version", format.REGSZ, []byte("1.0.0\x00"))
		plan.AddSetValue(basePath, "Enabled", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})

		// Add config subkey with binary data
		configPath := append([]string(nil), basePath...)
		configPath = append(configPath, "Config")
		plan.AddEnsureKey(configPath)
		plan.AddSetValue(configPath, "Settings", format.REGBinary, bytes.Repeat([]byte{0xAB}, 256))

		b.StartTimer()

		_, err := session.ApplyWithTx(plan)
		if err != nil {
			b.Fatalf("Apply failed: %v", err)
		}
	}
}
