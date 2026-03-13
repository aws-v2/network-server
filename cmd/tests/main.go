package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	start := time.Now()
	fmt.Println("====================================================")
	fmt.Println("🚀 Network Service: Unified Test Suite")
	fmt.Println("====================================================")

	// Run Service Unit Tests
	fmt.Println("\n🔍 Running Service Layer Unit Tests...")
	if err := runGoTest("./tests/service/..."); err != nil {
		fmt.Printf("\n❌ Service tests failed: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(start)
	fmt.Println("\n====================================================")
	fmt.Printf("✅ All tests passed! (Duration: %v)\n", duration.Round(time.Millisecond))
	fmt.Println("====================================================")
}

func runGoTest(path string) error {
	cmd := exec.Command("go", "test", "-v", "-count=1", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
