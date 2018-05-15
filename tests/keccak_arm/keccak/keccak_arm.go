package keccak

// go:noescape
// KeccakF1600 permutes the state using the provided permutation constants
// This function is implemented in keccak_arm.s
func KeccakF1600(state *[25]uint64, constants *[24]uint64)
