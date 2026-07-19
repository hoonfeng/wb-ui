// Translation of: Source/JavaScriptCore/runtime/Microtask.h
//
// InternalMicrotask enumerates all internal microtask types used by
// Promise reaction scheduling and async/await operations.

package runtime

// InternalMicrotask corresponds to JSC::InternalMicrotask.
// Enumerates all internal microtask types used by Promise reactions.
type InternalMicrotask uint8

const (
	InternalMicrotaskNone InternalMicrotask = iota
	InternalMicrotaskPromiseResolveThenableJobFast
	InternalMicrotaskPromiseResolveThenableJobWithInternalMicrotaskFast
	InternalMicrotaskPromiseResolveThenableJob
	InternalMicrotaskPromiseResolveThenableJobWithInternalMicrotask
	InternalMicrotaskPromiseResolveWithoutHandlerJob
	InternalMicrotaskPromiseFulfillWithoutHandlerJob
	InternalMicrotaskPromiseRaceResolveJob
	InternalMicrotaskPromiseAllResolveJob
	InternalMicrotaskPromiseAllSettledResolveJob
	InternalMicrotaskPromiseAnyResolveJob
	InternalMicrotaskPromiseFinallyReactionJob
	InternalMicrotaskPromiseFinallyAwaitJob
	InternalMicrotaskPromiseReactionJob
	InternalMicrotaskAsyncFunctionResume
	InternalMicrotaskAsyncFromSyncIteratorContinue
	InternalMicrotaskAsyncFromSyncIteratorDone
	InternalMicrotaskAsyncGeneratorYieldAwaited
	InternalMicrotaskAsyncGeneratorBodyCallNormal
	InternalMicrotaskAsyncGeneratorBodyCallReturn
	InternalMicrotaskAsyncGeneratorAwaitReturn
	InternalMicrotaskInvokeFunctionJob
	InternalMicrotaskAsyncModuleExecutionResume
	InternalMicrotaskAsyncModuleExecutionDone
	InternalMicrotaskModuleRegistryFetchSettled
	InternalMicrotaskModuleRegistryModuleSettled
	InternalMicrotaskModuleGraphLoadingError
	InternalMicrotaskModuleLoadStep
	InternalMicrotaskModuleLoadTopSettled
	InternalMicrotaskModuleLoadTopRejected
	InternalMicrotaskModuleLoadSpecifierTransform
	InternalMicrotaskModuleLoadCombinedLoadSettled
	InternalMicrotaskModuleLoadCombinedStateSettled
	InternalMicrotaskModuleLoadLinkEvaluateSettled
	InternalMicrotaskModuleLoadReturnRecord
	InternalMicrotaskModuleLoadReturnModuleKey
	InternalMicrotaskModuleLoadStoreError
	InternalMicrotaskDynamicImportLoadSettled
	InternalMicrotaskDynamicImportEvaluateSettled
	InternalMicrotaskDynamicImportDeferLoadSettled
	InternalMicrotaskDynamicImportDeferDependencySettled
	InternalMicrotaskImportModuleNamespace
	// WebAssemblyMicrotaskCompileStreaming
	// WebAssemblyMicrotaskInstantiateStreaming
	InternalMicrotaskOpaque
)

// MaxMicrotaskArguments is the maximum number of arguments a microtask can receive.
const MaxMicrotaskArguments = 3

// PromiseReactionPacksGlobalContextAndIndex returns true for Promise.all/allSettled/any element jobs
// whose reaction packs (globalContext cell, element index) instead of a single context cell.
func promiseReactionPacksGlobalContextAndIndex(task InternalMicrotask) bool {
	return task >= InternalMicrotaskPromiseAllResolveJob && task <= InternalMicrotaskPromiseAnyResolveJob
}

// QueuedTaskResult corresponds to JSC::QueuedTaskResult.
type QueuedTaskResult uint8

const (
	QueuedTaskResultExecuted  QueuedTaskResult = 0
	QueuedTaskResultDiscard                      = 1
	QueuedTaskResultSuspended                    = 2
)
