//go:build ignore

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	"github.com/joshuapare/hivekit/hive/builder"
)

func main() {
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile := flag.String("memprofile", "", "write memory profile to file")
	strategy := flag.String("strategy", "inplace", "strategy to use: inplace, append, hybrid")
	flag.Parse()

	// Set up CPU profiling
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	regPath := "testdata/suite/windows-8-consumer-preview-software.reg"

	// Check if file exists
	if _, err := os.Stat(regPath); os.IsNotExist(err) {
		log.Fatalf("Test .reg file not found: %s", regPath)
	}

	// Create temp directory for output hive
	dir, err := os.MkdirTemp("", "profile-test-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	hivePath := filepath.Join(dir, "windows-8-consumer-preview-software.hive")

	// Set strategy
	var strategyType builder.StrategyType
	switch *strategy {
	case "inplace":
		strategyType = builder.StrategyInPlace
	case "append":
		strategyType = builder.StrategyAppend
	case "hybrid":
		strategyType = builder.StrategyHybrid
	default:
		log.Fatalf("Unknown strategy: %s", *strategy)
	}

	opts := &builder.Options{
		Strategy:           strategyType,
		CreateIfNotExists:  true,
		AutoFlushThreshold: 1000,
	}

	fmt.Printf("Building hive with strategy: %s\n", *strategy)
	fmt.Printf("Input: %s\n", regPath)
	fmt.Printf("Output: %s\n", hivePath)

	// Build the hive
	err = builder.BuildFromRegFile(hivePath, regPath, opts)
	if err != nil {
		fmt.Printf("Build failed: %v\n", err)

		// Check file size even on failure
		if info, statErr := os.Stat(hivePath); statErr == nil {
			sizeMB := float64(info.Size()) / (1024 * 1024)
			fmt.Printf("Partial hive size: %.2f MB (%d bytes)\n", sizeMB, info.Size())
		}
	} else {
		// Success - print stats
		info, _ := os.Stat(hivePath)
		sizeMB := float64(info.Size()) / (1024 * 1024)
		fmt.Printf("✓ Build succeeded!\n")
		fmt.Printf("Final hive size: %.2f MB (%d bytes)\n", sizeMB, info.Size())
	}

	// Write memory profile
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}

	fmt.Println("Profiling complete.")
}
