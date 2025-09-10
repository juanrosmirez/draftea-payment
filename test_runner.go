package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	fmt.Println("Running tests...")
	
	// Run integration tests
	cmd := exec.Command("go", "test", "-v", "./test")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Test execution failed: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println("Tests completed successfully!")
}
