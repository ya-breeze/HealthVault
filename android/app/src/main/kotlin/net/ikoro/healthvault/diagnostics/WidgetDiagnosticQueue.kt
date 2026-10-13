package net.ikoro.healthvault.diagnostics

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.launch

/** Ordered, bounded, nonblocking persistence and upload of widget diagnostics. */
class WidgetDiagnosticQueue(scope: CoroutineScope, capacity: Int = 256) {
    private val actions = Channel<() -> Unit>(capacity)

    init {
        scope.launch {
            try {
                for (action in actions) diagnosticBestEffort(action)
            } finally {
                actions.close()
            }
        }
    }

    fun offer(action: () -> Unit): Boolean = actions.trySend(action).isSuccess
}
