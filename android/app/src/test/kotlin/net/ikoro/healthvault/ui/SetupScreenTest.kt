package net.ikoro.healthvault.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SetupScreenTest {

    @Test
    fun `normalizes supported server URLs to an origin`() {
        assertEquals("https://hcw.example.com", normalizeServerUrl("hcw.example.com/path?q=1#part"))
        assertEquals("http://192.168.1.54:8892", normalizeServerUrl("http://192.168.1.54:8892/food/"))
        assertEquals("http://[::1]:8892", normalizeServerUrl("http://[::1]:8892/api"))
    }

    @Test
    fun `rejects schemes and ports OkHttp cannot request`() {
        assertNull(normalizeServerUrl("ftp://hcw.example.com"))
        assertNull(normalizeServerUrl("https://hcw.example.com:99999"))
    }
}
