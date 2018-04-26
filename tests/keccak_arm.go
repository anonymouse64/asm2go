package keccak

// go:noescape
// This function is implemented in keccak.s
func KeccakF1600(state *[25]uint64, constants *[25]uint64)
