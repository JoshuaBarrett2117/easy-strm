package main

import (
	"fmt"
	"os"
)

func runParser() {
	b, err := os.ReadFile("debug_directory_tree.txt")
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}
	entries, err := Parse115DirTreeFile(b)
	if err != nil {
		fmt.Println("Error parsing:", err)
		return
	}
	fmt.Printf("Parsed %d entries\n", len(entries))
	
	if len(entries) > 0 {
		fmt.Printf("First entry: %+v\n", entries[0])
		fmt.Printf("Last entry: %+v\n", entries[len(entries)-1])
	}
}

func init() {
	if len(os.Args) > 1 && os.Args[1] == "testparse" {
		runParser()
		os.Exit(0)
	}
}
