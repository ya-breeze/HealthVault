package net.ikoro.healthvault.store

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

class SecureStoreTest {

    @Test
    fun `a failed synchronous write is reported instead of claimed durable`() {
        val prefs = FakeSharedPreferences().apply { commitSucceeds = false }
        val store = SecureStore(prefs)

        assertThrows(IllegalStateException::class.java) {
            store.username = "alice"
        }
    }

    @Test
    fun `a newer active retry deadline survives an older overlapping result`() {
        val prefs = FakeSharedPreferences()
        val store = SecureStore(prefs)
        val firstRead = AtomicBoolean(true)
        val oldResultReadState = CountDownLatch(1)
        val releaseOldResult = CountDownLatch(1)
        val newerResultStarted = CountDownLatch(1)
        val newerResultFinished = CountDownLatch(1)
        prefs.beforeGetLong = {
            if (firstRead.compareAndSet(true, false)) {
                oldResultReadState.countDown()
                assertTrue(releaseOldResult.await(5, TimeUnit.SECONDS))
            }
        }

        val oldResult = Thread {
            store.saveRefreshState(failed = false, nextAttemptAtMillis = 0L, nowMillis = 1_500L)
        }
        val newerResult = Thread {
            newerResultStarted.countDown()
            store.saveRefreshState(failed = true, nextAttemptAtMillis = 2_000L, nowMillis = 1_000L)
            newerResultFinished.countDown()
        }
        oldResult.start()
        assertTrue(oldResultReadState.await(5, TimeUnit.SECONDS))
        newerResult.start()
        assertTrue(newerResultStarted.await(5, TimeUnit.SECONDS))

        // With the lock, the newer write cannot finish while the older write
        // is paused between reading and committing. Without it, the newer
        // deadline lands first and the older result then erases it.
        val newerFinishedBeforeRelease = newerResultFinished.await(1, TimeUnit.SECONDS)
        releaseOldResult.countDown()
        oldResult.join(5_000)
        newerResult.join(5_000)
        assertFalse(oldResult.isAlive)
        assertFalse(newerResult.isAlive)
        assertFalse(newerFinishedBeforeRelease)

        assertTrue(store.refreshFailed)
        assertEquals(2_000L, store.nextRefreshAtMillis)

        store.saveRefreshState(failed = false, nextAttemptAtMillis = 0L, nowMillis = 2_001L)
        assertFalse(store.refreshFailed)
        assertEquals(0L, store.nextRefreshAtMillis)
    }

    @Test
    fun `a result from a signed-out session cannot restore refresh state`() {
        val store = SecureStore(FakeSharedPreferences())
        store.saveSession("https://health.example", "alice", "secret")
        val oldGeneration = store.currentSessionGeneration

        store.clearSession()
        store.saveSession("https://health.example", "bob", "new-secret")

        assertFalse(
            store.saveRefreshState(
                failed = true,
                nextAttemptAtMillis = 2_000L,
                nowMillis = 1_000L,
                expectedSessionGeneration = oldGeneration,
            ),
        )
        assertFalse(store.refreshFailed)
        assertEquals(0L, store.nextRefreshAtMillis)
    }
}
