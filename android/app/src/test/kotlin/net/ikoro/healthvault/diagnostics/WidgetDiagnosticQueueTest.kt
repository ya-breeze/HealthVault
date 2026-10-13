package net.ikoro.healthvault.diagnostics

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.runCurrent
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class WidgetDiagnosticQueueTest {
    @Test fun `queue preserves terminal record before upload and bounds pending actions`() = runTest {
        val queue = WidgetDiagnosticQueue(backgroundScope, capacity = 2)
        val stages = mutableListOf<String>()
        assertTrue(queue.offer { stages += "terminal record" })
        assertTrue(queue.offer { stages += "upload" })
        assertFalse(queue.offer { stages += "overflow" })
        assertTrue(stages.isEmpty())
        runCurrent()
        assertEquals(listOf("terminal record", "upload"), stages)
    }

    @Test fun `failed persistence does not prevent later action`() = runTest {
        val queue = WidgetDiagnosticQueue(backgroundScope)
        var uploaded = false
        queue.offer { error("store unavailable") }
        queue.offer { uploaded = true }
        runCurrent()
        assertTrue(uploaded)
    }

    @Test fun `cancellation closes queue rather than silently retaining more actions`() = runTest {
        val queue = WidgetDiagnosticQueue(backgroundScope)
        var later = false
        queue.offer { throw CancellationException("stopped") }
        queue.offer { later = true }
        runCurrent()
        assertFalse(later)
        assertFalse(queue.offer { later = true })
    }
}
