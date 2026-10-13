package net.ikoro.healthvault.work

import androidx.work.Data
import androidx.work.workDataOf
import net.ikoro.healthvault.diagnostics.DiagnosticEvent
import net.ikoro.healthvault.diagnostics.WidgetFlow
import net.ikoro.healthvault.diagnostics.periodicWidgetFlow

internal fun widgetWorkData(flow: WidgetFlow): Data = workDataOf(
    RefreshScheduler.FLOW_ID to flow.id,
    RefreshScheduler.FLOW_GENERATION to flow.generation,
    RefreshScheduler.FLOW_TRIGGER to flow.trigger,
)

/** Decode the same persisted Data that WorkManager supplies to the worker. */
internal fun widgetWorkFlow(input: Data, tags: Set<String>, currentGeneration: Long,
    workId: String, retry: Boolean, history: List<DiagnosticEvent>): WidgetFlow {
    val suppliedId = input.getString(RefreshScheduler.FLOW_ID)
    if (suppliedId != null) return WidgetFlow(
        suppliedId, input.getLong(RefreshScheduler.FLOW_GENERATION, -1),
        if (retry) "retry" else input.getString(RefreshScheduler.FLOW_TRIGGER) ?: "",
    )
    val flow = periodicWidgetFlow(currentGeneration, workId, retry, history)
    return if (!retry && RefreshScheduler.PERIODIC_TAG !in tags) flow.copy(trigger = "") else flow
}
