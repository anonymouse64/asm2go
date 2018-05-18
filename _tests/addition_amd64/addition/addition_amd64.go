package addition

//go:noescape
// Add2 returns the sum of two numbers
// This function is implemented in addition.s
func Add2(x, y int) int
