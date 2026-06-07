// Command ensemble is the single self-organizing multiroom audio binary.
// Every node runs this. Real wiring (flag parsing, port binds, capability
// probing, lifecycle) is added by piece K; this stub prints the version and
// exits 0 so the tree builds and runs from wave 0.
package main

import "fmt"

func main() {
	fmt.Println("ensemble v2")
}
