#include "funcdata.h"
#include "textflag.h"

TEXT ·callAdd(SB),$24-24
	GO_ARGS
	NO_LOCAL_POINTERS
	MOVQ $0, ret+16(FP)
	GO_RESULTS_INITIALIZED
	MOVQ a+0(FP), AX
	MOVQ AX, 0(SP)
	MOVQ b+8(FP), AX
	MOVQ AX, 8(SP)
	CALL ·add(SB)
	MOVQ 16(SP), AX
	MOVQ AX, ret+16(FP)
	RET
