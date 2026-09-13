// Synthetic control: literal trip count, closed loop, unsigned packed source,
// four distinct field observations, and dead shifted source at return.
#include "textflag.h"
TEXT ·fields(SB), NOSPLIT, $0-16
 MOVD ptr+0(FP), R6
 MOVD $0, R9
 MOVD $8, R7
loop:
 MOVBU (R6), R8
 AND $3, R8, R11
 LSR $2, R8, R8
 ADD R11, R9, R9
 AND $3, R8, R11
 LSR $2, R8, R8
 ADD R11, R9, R9
 AND $3, R8, R11
 LSR $2, R8, R8
 ADD R11, R9, R9
 AND $3, R8, R11
 LSR $2, R8, R8
 ADD R11, R9, R9
 SUBS $1, R7, R7
 BNE loop
 MOVD R9, ret+8(FP)
 RET
