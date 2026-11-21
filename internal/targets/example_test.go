package targets

import (
	"fmt"
	"log"
	"sort"
)

func ExampleResolver_Resolve() {
	resolver := NewDefaultResolver()

	// Resolve a specific target
	config, err := resolver.Resolve("wasi")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Target: %s\n", config.Name)
	fmt.Printf("LLVM Target: %s\n", config.LLVMTarget)
	fmt.Printf("CPU: %s\n", config.CPU)
	fmt.Printf("GOOS: %s\n", config.GOOS)
	fmt.Printf("GOARCH: %s\n", config.GOARCH)
	if len(config.BuildTags) > 0 {
		fmt.Printf("Build Tags: %v\n", config.BuildTags)
	}
	if len(config.CFlags) > 0 {
		fmt.Printf("C Flags: %v\n", config.CFlags)
	}
	if len(config.LDFlags) > 0 {
		fmt.Printf("LD Flags: %v\n", config.LDFlags)
	}
}

func ExampleResolver_ListAvailableTargets() {
	resolver := NewDefaultResolver()

	targets, err := resolver.ListAvailableTargets()
	if err != nil {
		log.Fatal(err)
	}

	sort.Strings(targets)
	fmt.Printf("Available targets (%d):\n", len(targets))
	for i, target := range targets {
		fmt.Printf("%d. %s\n", i+1, target)
	}
}

func ExampleResolver_ResolveAll() {
	resolver := NewDefaultResolver()

	configs, err := resolver.ResolveAll()
	if err != nil {
		log.Fatal(err)
	}

	// Count targets by GOOS
	goosCounts := make(map[string]int)
	for _, config := range configs {
		if config.GOOS != "" {
			goosCounts[config.GOOS]++
		}
	}

	fmt.Printf("Loaded %d target configurations\n", len(configs))
	fmt.Printf("GOOS distribution:\n")
	for goos, count := range goosCounts {
		fmt.Printf("  %s: %d targets\n", goos, count)
	}
}
