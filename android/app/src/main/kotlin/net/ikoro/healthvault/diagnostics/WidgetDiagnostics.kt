package net.ikoro.healthvault.diagnostics

import kotlinx.coroutines.CancellationException
import net.ikoro.healthvault.api.ApiResult
import java.time.Instant
import java.util.UUID

/** Every stage carries the original session generation, including after browser recreation. */
data class WidgetFlow(val id: String = UUID.randomUUID().toString(), val generation: Long, val trigger: String)

/** Platform-free seam: diagnostics failures never change refresh outcomes or cancellation. */
class WidgetDiagnostics(
    val flow: WidgetFlow,
    private val environment: () -> Pair<String, Int>,
    private val record: (DiagnosticEvent, Long) -> Unit,
    val workId: String = "",
    val attempt: Int = 1,
) {
    fun event(operation: String, category: String, provider: String = "", durationMillis: Long = 0,
              snapshotAgeMillis: Long? = null, snapshotChanged: Boolean? = null, httpCode: Int = 0) {
        diagnosticBestEffort {
            val (version, sdk) = environment()
            record(DiagnosticEvent(
                id = UUID.randomUUID().toString(), occurredAt = Instant.now().toString(),
                operation = operation, category = category, requestId = flow.id,
                appVersion = version.replace(Regex("[^a-zA-Z0-9.+_-]"), "_").take(80).ifEmpty { "unknown" },
                androidApi = sdk, workId = workId, trigger = flow.trigger, provider = provider,
                durationMillis = durationMillis.coerceAtLeast(0), attempt = attempt.coerceIn(1, 1000000),
                snapshotAgeMillis = snapshotAgeMillis?.coerceAtLeast(0), snapshotChanged = snapshotChanged,
                httpCode = httpCode,
            ), flow.generation)
        }
    }

    fun fetch(result: ApiResult<*>, durationMillis: Long) {
        val category = when (result) {
            is ApiResult.Success -> "success"
            is ApiResult.Unauthenticated -> "unauthenticated"
            is ApiResult.RateLimited -> "rate_limited"
            is ApiResult.AccessChallenge -> "access_challenge"
            is ApiResult.NetworkFailure -> networkCategory(result.cause)
            is ApiResult.ServerError -> if (result.code in 200..299) "invalid_response" else "server"
        }
        event("widget_fetch", category, durationMillis = durationMillis,
            httpCode = when (result) {
                is ApiResult.Success -> 200
                is ApiResult.Unauthenticated -> 401
                is ApiResult.RateLimited -> 429
                is ApiResult.ServerError -> result.code
                else -> 0
            })
    }
}

inline fun diagnosticBestEffort(action: () -> Unit) {
    try { action() } catch (failure: Exception) {
        if (failure is CancellationException) throw failure
    }
}

/** HTTP recovery counters remain compatible; independent widget stages preserve their identity. */
fun recordedDiagnostic(event: DiagnosticEvent, history: List<DiagnosticEvent>): DiagnosticEvent {
    if (event.operation.startsWith("widget_")) return event
    val previous = history.lastOrNull { it.operation == event.operation }
    val previousFailed = previous != null && previous.category !in setOf("success", "recovered")
    return event.copy(
        category = if (event.category == "success" && previousFailed) "recovered" else event.category,
        attempt = if (previousFailed) (previous!!.attempt + 1).coerceAtMost(1000000) else 1,
    )
}

fun snapshotAge(now: Long, fetchedAt: Long?): Long? = fetchedAt?.let {
    if (now <= it) 0 else if (it < 0 && now > Long.MAX_VALUE + it) Long.MAX_VALUE else now - it
}

/** A periodic retry reuses its previous execution flow; a new interval gets a new UUID. */
fun periodicWidgetFlow(generation: Long, workId: String, retry: Boolean,
    history: List<DiagnosticEvent>): WidgetFlow {
    val previous = if (retry) history.lastOrNull { it.workId == workId && it.operation == "widget_work" } else null
    return WidgetFlow(id = previous?.requestId ?: UUID.randomUUID().toString(), generation = generation,
        trigger = if (retry) "retry" else "periodic")
}

/** Latest pending one-off intent in the unique queue; replacement is a request, not an OS receipt. */
fun pendingWidgetReplacement(history: List<DiagnosticEvent>, newWorkId: String): DiagnosticEvent? {
    val previous = history.lastOrNull { it.operation == "widget_work" && it.category == "enqueue_requested" }
        ?: return null
    if (previous.workId == newWorkId) return null
    val last = history.lastOrNull { it.workId == previous.workId && it.operation == "widget_work" }
        ?: return null
    return previous.takeIf { last.category in setOf("enqueue_requested", "started", "deferred") }
}
