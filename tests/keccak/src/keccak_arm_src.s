@
@ Implementation by Ronny Van Keer, hereby denoted as "the implementer".
@
@ For more information, feedback or questions, please refer to our website:
@ https://keccak.team/
@
@ To the extent possible under law, the implementer has waived all copyright
@ and related or neighboring rights to the source code in this file.
@ http://creativecommons.org/publicdomain/zero/1.0/
@
@ ---
@
@ This file implements Keccak-p[1600] in a SnP-compatible way.
@ Please refer to SnP-documentation.h for more details.
@
@ This implementation comes with KeccakP-1600-SnP.h in the same folder.
@ Please refer to LowLevel.build for the exact list of other files it must be combined with.
@

@ WARNING: These functions work only on little endian CPU with@ ARMv7A + NEON architecture
@ WARNING: State must be 256 bit (32 bytes) aligned, best is 64-byte (cache alignment).
@ INFO: Tested on Cortex-A8 (BeagleBone Black), using gcc.


.text

@ macros

.macro    RhoPi4      dst1, src1, rot1, dst2, src2, rot2, dst3, src3, rot3, dst4, src4, rot4
    .if (\rot1  &  7) != 0
    vshl.u64    \dst1, \src1, #\rot1
    .else
    vext.8      \dst1, \src1, \src1, #8-\rot1/8
    .endif
    .if (\rot2  &  7) != 0
    vshl.u64    \dst2, \src2, #\rot2
    .else
    vext.8      \dst2, \src2, \src2, #8-\rot2/8
    .endif
    .if (\rot3  &  7) != 0
    vshl.u64    \dst3, \src3, #\rot3
    .else
    vext.8      \dst3, \src3, \src3, #8-\rot3/8
    .endif
    .if (\rot4  &  7) != 0
    vshl.u64    \dst4, \src4, #\rot4
    .else
    vext.8      \dst4, \src4, \src4, #8-\rot4/8
    .endif
    .if (\rot1  &  7) != 0
    vsri.u64    \dst1, \src1, #64-\rot1
    .endif
    .if (\rot2  &  7) != 0
    vsri.u64    \dst2, \src2, #64-\rot2
    .endif
    .if (\rot3  &  7) != 0
    vsri.u64    \dst3, \src3, #64-\rot3
    .endif
    .if (\rot4  &  7) != 0
    vsri.u64    \dst4, \src4, #64-\rot4
    .endif
    .endm

.macro    KeccakRound

    @Prepare Theta
    veor.64     q13, q0, q5
    vst1.64     {q12}, [r0:128]!
    veor.64     q14, q1, q6
    vst1.64     {q4}, [r0:128]!
    veor.64     d26,  d26,  d27
    vst1.64     {q9}, [r0:128]
    veor.64     d28,  d28,  d29
    veor.64     d26,  d26,  d20
    veor.64     d27,  d28,  d21

    veor.64     q14, q2, q7
    veor.64     q15, q3, q8
    veor.64     q4, q4, q9
    veor.64     d28,  d28,  d29
    veor.64     d30,  d30,  d31
    veor.64     d25,  d8,  d9
    veor.64     d28,  d28,  d22
    veor.64     d29,  d30,  d23
    veor.64     d25,  d25,  d24
    sub         r0, r0, #32

    @Apply Theta
    vadd.u64    d30,  d27,  d27
    vadd.u64    d24,  d28,  d28
    vadd.u64    d8,  d29,  d29
    vadd.u64    d18,  d25,  d25

    vsri.64     d30,  d27,  #63
    vsri.64     d24,  d28,  #63
    vsri.64     d8,  d29,  #63
    vsri.64     d18,  d25,  #63

    veor.64     d30,  d30,  d25
    veor.64     d24,  d24,  d26
    veor.64     d8,  d8,  d27
    vadd.u64    d27,  d26,  d26   @u
    veor.64     d18,  d18,  d28

    vmov.i64    d31,  d30
    vmov.i64    d25,  d24
    vsri.64     d27,  d26,  #63     @u
    vmov.i64    d9,  d8
    vmov.i64    d19,  d18

    veor.64     d20,  d20,  d30
    veor.64     d21,  d21,  d24
    veor.64     d27,  d27,  d29   @u
    veor.64     d22,  d22,  d8
    veor.64     d23,  d23,  d18
    vmov.i64    d26,  d27           @u

    veor.64     q0, q0, q15
    veor.64     q1, q1, q12
    veor.64     q2, q2, q4
    veor.64     q3, q3, q9

    veor.64     q5, q5, q15
    veor.64     q6, q6, q12
    vld1.64     {q12}, [r0:128]!
    veor.64     q7, q7, q4
    vld1.64     {q4}, [r0:128]!
    veor.64     q8, q8, q9
    vld1.64     {q9}, [r0:128]
    veor.64     d24,  d24,  d26   @u
    sub         r0, r0, #32
    veor.64     q4, q4, q13  @u
    veor.64     q9, q9, q13  @u

    @Rho Pi
    vmov.i64    d27, d2
    vmov.i64    d28, d4
    vmov.i64    d29, d6
    vmov.i64    d25, d8

    RhoPi4      d2, d3, 44, d4, d14, 43, d8, d24, 14, d6, d17, 21  @  1 <  6,  2 < 12,  4 < 24,  3 < 18
    RhoPi4      d3, d9, 20, d14, d16, 25, d24, d21,  2, d17, d15, 15  @  6 <  9, 12 < 13, 24 < 21, 18 < 17
    RhoPi4      d9, d22, 61, d16, d19,  8, d21, d7, 55, d15, d12, 10  @  9 < 22, 13 < 19, 21 <  8, 17 < 11
    RhoPi4      d22, d18, 39, d19, d23, 56, d7, d13, 45, d12, d5,  6  @ 22 < 14, 19 < 23,  8 < 16, 11 < 7
    RhoPi4      d18, d20, 18, d23, d11, 41, d13, d1, 36, d5, d10,  3  @ 14 < 20, 23 < 15, 16 <  5,  7 < 10
    RhoPi4      d20, d28, 62, d11, d25, 27, d1, d29, 28, d10, d27,  1  @ 20 <  2, 15 <  4,  5 <  3, 10 < 1

    @Chi    b+g
    vmov.i64    q13, q0
    vbic.64     q15, q2, q1  @ ba ^= ~be & bi
    veor.64     q0, q15
    vmov.i64    q14, q1
    vbic.64     q15, q3, q2  @ be ^= ~bi & bo
    veor.64     q1, q15
    vbic.64     q15, q4, q3  @ bi ^= ~bo & bu
    veor.64     q2, q15
    vbic.64     q15, q13, q4  @ bo ^= ~bu & ba
    vbic.64     q13, q14, q13  @ bu ^= ~ba & be
    veor.64     q3, q15
    veor.64     q4, q13

    @Chi    k+m
    vmov.i64    q13, q5
    vbic.64     q15, q7, q6  @ ba ^= ~be & bi
    veor.64     q5, q15
    vmov.i64    q14, q6
    vbic.64     q15, q8, q7  @ be ^= ~bi & bo
    veor.64     q6, q15
    vbic.64     q15, q9, q8  @ bi ^= ~bo & bu
    veor.64     q7, q15
    vbic.64     q15, q13, q9  @ bo ^= ~bu & ba
    vbic.64     q13, q14, q13  @ bu ^= ~ba & be
    veor.64     q8, q15
    veor.64     q9, q13

    @Chi    s
    vmov.i64    q13, q10
    vbic.64     d30,  d22,  d21   @ ba ^= ~be & bi
    vbic.64     d31,  d23,  d22   @ be ^= ~bi & bo
    veor.64     q10, q15
    vbic.64     d30,  d24,  d23   @ bi ^= ~bo & bu
    vbic.64     d31,  d26,  d24   @ bo ^= ~bu & ba
    vbic.64     d26,  d27,  d26   @ bu ^= ~ba & be
    veor.64     q11, q15
    vld1.64     d30,   [r1:64]!  @ Iota
    veor.64     d24,  d26
    veor.64     d0, d0, d30     @ Iota
    .endm



@ ----------------------------------------------------------------------------
@
@  void KeccakF1600( void *states, void *constants )
@
.align 8
.global   KeccakF1600
.type   KeccakF1600, %function;
KeccakF1600:
    @ note that we don't store lr, as the plan9 assembler will insert that code for us
    @ sp+4 is taken as the start of the state array
    @ sp+8 is taken as the start of the constants
    ldr     r0, [sp, #4]
    ldr     r1, [sp, #8]
    vpush   {q4-q7}
    @ load state - interleaving loads helps with pipelining
    vld1.64 d0, [r0:64]!
    vld1.64 d2, [r0:64]!
    vld1.64 d4, [r0:64]!
    vld1.64 d6, [r0:64]!
    vld1.64 d8, [r0:64]!
    vld1.64 d1, [r0:64]!
    vld1.64 d3, [r0:64]!
    vld1.64 d5, [r0:64]!
    vld1.64 d7, [r0:64]!
    vld1.64 d9, [r0:64]!
    vld1.64 d10, [r0:64]!
    vld1.64 d12, [r0:64]!
    vld1.64 d14, [r0:64]!
    vld1.64 d16, [r0:64]!
    vld1.64 d18, [r0:64]!
    vld1.64 d11, [r0:64]!
    vld1.64 d13, [r0:64]!
    vld1.64 d15, [r0:64]!
    vld1.64 d17, [r0:64]!
    vld1.64 d19, [r0:64]!
    vld1.64 { d20, d21 }, [r0:128]!
    vld1.64 { d22, d23 }, [r0:128]!
    vld1.64 d24, [r0:64]
    sub     r0, r0, #24*8
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    KeccakRound
    @ store state
    vst1.64 d0, [r0:64]!
    vst1.64 d2, [r0:64]!
    vst1.64 d4, [r0:64]!
    vst1.64 d6, [r0:64]!
    vst1.64 d8, [r0:64]!
    vst1.64 d1, [r0:64]!
    vst1.64 d3, [r0:64]!
    vst1.64 d5, [r0:64]!
    vst1.64 d7, [r0:64]!
    vst1.64 d9, [r0:64]!
    vst1.64 d10, [r0:64]!
    vst1.64 d12, [r0:64]!
    vst1.64 d14, [r0:64]!
    vst1.64 d16, [r0:64]!
    vst1.64 d18, [r0:64]!
    vst1.64 d11, [r0:64]!
    vst1.64 d13, [r0:64]!
    vst1.64 d15, [r0:64]!
    vst1.64 d17, [r0:64]!
    vst1.64 d19, [r0:64]!
    vst1.64 { d20, d21 }, [r0:128]!
    vst1.64 { d22, d23 }, [r0:128]!
    vst1.64 d24, [r0:64]
    vpop    {q4-q7}
    @ note that bx isn't necessary - the plan9 assembler inserts this for us
