//go:generate asm2go -file src/keccak.s -gofile keccak_arm.go -out keccak_arm.s -as-opts -march=armv7-a -as-opts -mfpu=neon-vfpv4
package keccak

// go:noescape
// This function is implemented in keccak.s
func KeccakF1600(state *[25]uint64, constants *[24]uint64)
