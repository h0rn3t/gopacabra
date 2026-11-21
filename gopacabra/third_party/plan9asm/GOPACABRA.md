# Gopacabra patches to github.com/xgo-dev/plan9asm

Copy of `github.com/xgo-dev/plan9asm` v0.6.2, wired in through a `replace`
directive in the root `go.mod`. `.github` is removed.

- `go.mod`: `replace github.com/xgo-dev/llvm => ../llvm`, so the copy builds
  and tests on its own against LLVM 23 (`GOFLAGS=-mod=mod go test .`).
- amd64 ABI0 call frame (P11.5.2). Go assembly calls without
  `<ABIInternal>` use ABI0: the caller stores arguments at `0(SP)…` and reads
  results after them. Upstream only gives 386 a local stack; on amd64 `SP`
  was a zeroed register and such calls passed undefined arguments.
  - `amd64_ctx.go` `emitEntryAllocas`: amd64 also gets `%local_stack`, at
    least TEXT `$frame` bytes above the initial SP. `stackMovementBudget`
    counts `ADDQ`/`SUBQ`/`ANDQ`/`LEAQ` writes to SP like their 386 forms.
  - `amd64_ctx.go` `ptrFromMem`, `localStackPtr`, `writesSP`: when no
    instruction writes SP, `off(SP)` becomes a GEP into `%local_stack`
    instead of `inttoptr`, so LLVM tells frame slots apart and folds the
    stores and loads around a call.
  - `go_translate.go` `goFuncSigForDeclaredFunc`: a declared Go function's
    signature carries its ABI0 frame layout (results unflattened: `Index` is
    the result number, `Field` the part of an aggregate).
  - `amd64_lower_branch.go` `callSym`, `callSymFrame`: on amd64, a `CALL` to
    a function with a frame layout, without `<ABIInternal>` and with a local
    stack, loads the arguments from the frame, calls, and stores the results
    back. `<ABIInternal>` calls keep the register path.

Known limit: a named pseudo-SP reference (`x-8(SP)`) is parsed as hardware
SP.
