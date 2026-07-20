package main

import (
	"fmt"

	"github.com/crizah/Worm/diff"
)

func main() {
	a := "BMOAL"
	b := "BLOA"

	ans := diff.LCS(a, b)
	fmt.Print(ans)

}
