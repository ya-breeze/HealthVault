package net.ikoro.healthvault.diagnostics

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.io.IOException
import java.io.InterruptedIOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import javax.net.ssl.SSLException

@OptIn(kotlinx.serialization.ExperimentalSerializationApi::class)
@Serializable
data class DiagnosticEvent(
    val id: String,
    @SerialName("occurred_at") val occurredAt: String,
    val operation: String,
    val category: String,
    @SerialName("request_id") val requestId: String,
    @SerialName("http_code") val httpCode: Int = 0,
    @SerialName("duration_millis") val durationMillis: Long = 0,
    @kotlinx.serialization.EncodeDefault val attempt: Int = 1,
    @SerialName("app_version") val appVersion: String,
    @SerialName("android_api") val androidApi: Int,
)

@Serializable
data class DiagnosticJournal(
    val events: List<DiagnosticEvent> = emptyList(),
    val pendingIds: List<String> = emptyList(),
    val lastSummarySuccess: String? = null,
) {
    companion object {
        const val MAX_EVENTS = 200
        const val MAX_AGE_MILLIS = 14L * 24 * 60 * 60 * 1000
    }
}

fun networkCategory(cause: Throwable): String = when (cause) {
    is UnknownHostException -> "dns"
    is SocketTimeoutException -> "timeout"
    is InterruptedIOException -> "timeout"
    is SSLException -> "tls"
    is IOException -> "network"
    else -> "invalid_response"
}
