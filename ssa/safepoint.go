/*
 * Copyright (c) 2026 The XGo Authors (xgo.dev). All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package ssa

// EnableCooperativeSafepoints controls compiler-inserted scheduling polls.
func (p Program) EnableCooperativeSafepoints(enable bool) {
	p.enableSafepoints = enable
}

// CooperativeSafepointsEnabled reports whether scheduling polls are enabled.
func (p Program) CooperativeSafepointsEnabled() bool {
	return p.enableSafepoints
}

// PreemptRequestedVar names the runtime counter of pending preemption
// requests read by every safepoint when the request poll is enabled.
const PreemptRequestedVar = PkgRuntime + ".PreemptRequested"

// EnablePreemptRequestPoll makes each cooperative safepoint load
// PreemptRequestedVar and call the runtime only when it is non-zero
// (gopacabramn).
func (p Program) EnablePreemptRequestPoll(enable bool) {
	p.preemptRequestPoll = enable
}

// PreemptRequestPollEnabled reports whether safepoints poll the request flag.
func (p Program) PreemptRequestPollEnabled() bool {
	return p.preemptRequestPoll
}

// EnableBlockingCallMarkers controls the EnterSyscall/ExitSyscall markers
// around C calls and CallbackEnter/CallbackExit around exported entries
// (gopacabramn).
func (p Program) EnableBlockingCallMarkers(enable bool) {
	p.blockingCallMarkers = enable
}

// BlockingCallMarkersEnabled reports whether C calls are marked.
func (p Program) BlockingCallMarkersEnabled() bool {
	return p.blockingCallMarkers
}
