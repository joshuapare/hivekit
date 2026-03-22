package benchmark

import (
	"path/filepath"
	"testing"
)

func TestGenerateSmallFlat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small-flat.hive")

	err := GenerateSmallFlat(path)
	if err != nil {
		t.Fatalf("GenerateSmallFlat failed: %v", err)
	}

	size := HiveSize(path)
	if size < 1*1024*1024 {
		t.Errorf("expected small-flat hive to be > 1MB, got %d bytes", size)
	}
	t.Logf("small-flat hive size: %d bytes (%.2f MB)", size, float64(size)/(1024*1024))
}

func TestGenerateSmallDeep(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small-deep.hive")

	err := GenerateSmallDeep(path)
	if err != nil {
		t.Fatalf("GenerateSmallDeep failed: %v", err)
	}

	size := HiveSize(path)
	if size < 512*1024 {
		t.Errorf("expected small-deep hive to be > 512KB, got %d bytes", size)
	}
	t.Logf("small-deep hive size: %d bytes (%.2f MB)", size, float64(size)/(1024*1024))
}

func TestGenerateMediumMixed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping medium fixture generation in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "medium-mixed.hive")

	err := GenerateMediumMixed(path)
	if err != nil {
		t.Fatalf("GenerateMediumMixed failed: %v", err)
	}

	size := HiveSize(path)
	if size < 10*1024*1024 {
		t.Errorf("expected medium-mixed hive to be > 10MB, got %d bytes", size)
	}
	t.Logf("medium-mixed hive size: %d bytes (%.2f MB)", size, float64(size)/(1024*1024))
}

func TestGenerateLargeWide(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large fixture generation in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "large-wide.hive")

	err := GenerateLargeWide(path)
	if err != nil {
		t.Fatalf("GenerateLargeWide failed: %v", err)
	}

	size := HiveSize(path)
	if size < 20*1024*1024 {
		t.Errorf("expected large-wide hive to be > 20MB, got %d bytes", size)
	}
	t.Logf("large-wide hive size: %d bytes (%.2f MB)", size, float64(size)/(1024*1024))
}

func TestGenerateLargeRealistic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large fixture generation in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "large-realistic.hive")

	err := GenerateLargeRealistic(path)
	if err != nil {
		t.Fatalf("GenerateLargeRealistic failed: %v", err)
	}

	size := HiveSize(path)
	if size < 100*1024*1024 {
		t.Errorf("expected large-realistic hive to be > 100MB, got %d bytes", size)
	}
	t.Logf("large-realistic hive size: %d bytes (%.2f MB)", size, float64(size)/(1024*1024))
}
