//go:build gopacabra && gopacabramn && (linux || darwin)

package runtime

import "unsafe"

// Контексти і стеки G для M:N — _wrap/gopacabra_mn.c (P-NEW-09.5). Планувальника
// ще немає: функції нижче лише створюють, перемикають і звільняють контексти.
const gopacabraMNLLGoFiles = "; _wrap/gopacabra_mn.c"

// mnCtx — непрозорий gopacabra_mn_ctx: збережений sp і межі стека G.
type mnCtx struct{ _ [0]byte }

//go:linkname mnCtxNew C.gopacabra_mn_ctx_new
func mnCtxNew(size uintptr, fn goroutineFunc, arg unsafe.Pointer, goid uint64) *mnCtx

//go:linkname mnCtxFree C.gopacabra_mn_ctx_free
func mnCtxFree(ctx *mnCtx)

//go:linkname mnThreadCtx C.gopacabra_mn_thread_ctx
func mnThreadCtx() *mnCtx

//go:linkname mnSwitch C.gopacabra_mn_switch
func mnSwitch(from, to *mnCtx)

//go:linkname mnExit C.gopacabra_mn_exit
func mnExit(from, to *mnCtx)

// mnSelfCtx — контекст, що виконується: стек G з резерву або, без нього,
// стек потоку. Контекст потоку — це g0 M, і хелпер, що зберіг би в ньому стан
// G, зіпсував би повернення в планувальник.
func mnSelfCtx() *mnCtx {
	if gp := (*g)(unsafe.Pointer(currentG)); gp != nil && gp.context.platform.mn != nil {
		return gp.context.platform.mn
	}
	return mnThreadCtx()
}

// mnPing — дві сторони перемикання для MNSwitchForTesting.
type mnPing struct {
	main, g *mnCtx
	rounds  int
	sum     uint64
}

// mnLCG — послідовність, яку обидві сторони рахують через перемикання.
func mnLCG(x, mul uint64, i int) uint64 { return x*mul + uint64(i) }

const (
	mnMulG    = 6364136223846793005
	mnMulMain = 2862933555777941757
)

func mnPingEntry(arg unsafe.Pointer) unsafe.Pointer {
	p := (*mnPing)(arg)
	x := uint64(1)
	for i := 0; i < p.rounds; i++ {
		x = mnLCG(x, mnMulG, i)
		mnSwitch(p.g, p.main)
	}
	p.sum = x
	mnExit(p.g, p.main)
	return nil
}

// MNSwitchForTesting перемикає потік і G rounds разів туди й назад. Кожна
// сторона веде свою послідовність у локальних змінних, які живуть через
// перемикання; true — обидві суми як без перемикань.
func MNSwitchForTesting(rounds int) bool {
	// Сирі контексти хелпера прив'язані до потоку: G не мігрує, доки він іде.
	preemptDisable()
	defer preemptEnable()
	p := &mnPing{main: mnSelfCtx(), rounds: rounds}
	p.g = mnCtxNew(0, goroutineFunc(mnPingEntry), unsafe.Pointer(p), nextGoid(nil))
	if p.g == nil {
		return false
	}
	y := uint64(7)
	// rounds повернень з перемикань G і одне — з її виходу.
	for i := 0; i <= rounds; i++ {
		mnSwitch(p.main, p.g)
		y = mnLCG(y, mnMulMain, i)
	}
	mnCtxFree(p.g)
	wantG, wantMain := uint64(1), uint64(7)
	for i := 0; i < rounds; i++ {
		wantG = mnLCG(wantG, mnMulG, i)
	}
	for i := 0; i <= rounds; i++ {
		wantMain = mnLCG(wantMain, mnMulMain, i)
	}
	return p.sum == wantG && y == wantMain
}

// mnRun — функція, яку MNRunForTesting виконує на стеку G.
type mnRun struct {
	main, g *mnCtx
	f       func()
}

func mnRunEntry(arg unsafe.Pointer) unsafe.Pointer {
	r := (*mnRun)(arg)
	r.f()
	mnExit(r.g, r.main)
	return nil
}

// MNRunForTesting виконує f до кінця на новому стеку G розміру stack
// (0 — типовий). false — резерв не вдалося взяти.
func MNRunForTesting(stack uintptr, f func()) bool {
	// Сирі контексти хелпера прив'язані до потоку: G не мігрує, доки він іде.
	preemptDisable()
	defer preemptEnable()
	r := &mnRun{main: mnSelfCtx(), f: f}
	r.g = mnCtxNew(stack, goroutineFunc(mnRunEntry), unsafe.Pointer(r), nextGoid(nil))
	if r.g == nil {
		return false
	}
	mnSwitch(r.main, r.g)
	mnCtxFree(r.g)
	return true
}

// mnParked — G, яка стартує, паркується до другого перемикання і виходить.
type mnParked struct {
	main, g *mnCtx
}

func mnParkEntry(arg unsafe.Pointer) unsafe.Pointer {
	p := (*mnParked)(arg)
	mnSwitch(p.g, p.main)
	mnExit(p.g, p.main)
	return nil
}

// MNParkForTesting запускає n G зі стеком stack, кожну до першого
// перемикання назад, кличе whileParked, поки всі припарковані, і завершує
// їх. false — резерв для якоїсь G не вдалося взяти.
func MNParkForTesting(n int, stack uintptr, whileParked func()) bool {
	// Сирі контексти хелпера прив'язані до потоку: G не мігрує, доки він іде.
	preemptDisable()
	defer preemptEnable()
	main := mnSelfCtx()
	ps := make([]mnParked, n)
	started := 0
	for i := range ps {
		ps[i].main = main
		ps[i].g = mnCtxNew(stack, goroutineFunc(mnParkEntry), unsafe.Pointer(&ps[i]), nextGoid(nil))
		if ps[i].g == nil {
			break
		}
		mnSwitch(main, ps[i].g)
		started++
	}
	if started == n {
		whileParked()
	}
	for i := 0; i < started; i++ {
		mnSwitch(main, ps[i].g)
		mnCtxFree(ps[i].g)
	}
	return started == n
}

// MNWaiterResetForTesting кладе в w (пам'ять розміру SchedulerWaiter)
// нового очікувача поточної G, як netpoll перед кожним паркуванням.
func MNWaiterResetForTesting(w unsafe.Pointer) {
	*(*SchedulerWaiter)(w) = CurrentSchedulerWaiter()
}

// MNWaiterParkForTesting — Park очікувача w.
func MNWaiterParkForTesting(w unsafe.Pointer) { (*SchedulerWaiter)(w).Park() }

// MNWaiterReadyForTesting — Ready очікувача w.
func MNWaiterReadyForTesting(w unsafe.Pointer) { (*SchedulerWaiter)(w).Ready() }
