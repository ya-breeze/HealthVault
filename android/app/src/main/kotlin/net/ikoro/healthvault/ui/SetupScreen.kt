package net.ikoro.healthvault.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.api.HealthVaultApi
import net.ikoro.healthvault.store.SecureStore

/**
 * Normalizes a user-entered server address to an origin: adds `https://` if
 * no scheme was typed, and drops any path/query/fragment/trailing slash. The
 * caller decides what to do with a non-https result (SetupScreen shows a
 * warning) rather than this function silently upgrading it, since a LAN
 * stack like http://192.168.1.54:8892 is a legitimate target.
 */
internal fun normalizeServerUrl(input: String): String? {
    val trimmed = input.trim()
    if (trimmed.isEmpty()) return null
    val withScheme = if (trimmed.contains("://")) trimmed else "https://$trimmed"
    val uri = runCatching { java.net.URI(withScheme) }.getOrNull() ?: return null
    if (uri.host.isNullOrEmpty()) return null
    val port = if (uri.port == -1) "" else ":${uri.port}"
    return "${uri.scheme}://${uri.host}$port"
}

private sealed class SetupError {
    data object InvalidCredentials : SetupError()
    data class LockedOut(val retrySeconds: Long) : SetupError()
    data object AccessChallenge : SetupError()
    data object Unreachable : SetupError()
    data class Other(val code: Int) : SetupError()
}

@Composable
private fun SetupError.message(): String = when (this) {
    is SetupError.InvalidCredentials -> stringResource(R.string.setup_error_invalid_credentials)
    is SetupError.LockedOut -> stringResource(R.string.setup_error_locked_out, retrySeconds)
    is SetupError.AccessChallenge -> stringResource(R.string.setup_error_access_challenge)
    is SetupError.Unreachable -> stringResource(R.string.setup_error_unreachable)
    is SetupError.Other -> stringResource(R.string.setup_error_server, code)
}

@Composable
fun SetupScreen(
    api: HealthVaultApi,
    secureStore: SecureStore,
    onSignedIn: () -> Unit,
) {
    var serverUrlInput by remember { mutableStateOf(secureStore.serverUrl ?: "") }
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var loading by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<SetupError?>(null) }
    val scope = rememberCoroutineScope()

    val normalized = normalizeServerUrl(serverUrlInput)

    Surface(modifier = Modifier.fillMaxSize()) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(text = stringResource(R.string.setup_title), style = MaterialTheme.typography.headlineSmall)

            OutlinedTextField(
                value = serverUrlInput,
                onValueChange = { serverUrlInput = it },
                label = { Text(stringResource(R.string.setup_server_url)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            if (serverUrlInput.isNotBlank() && normalized != null && !normalized.startsWith("https://")) {
                Text(text = stringResource(R.string.setup_http_warning))
            }

            OutlinedTextField(
                value = username,
                onValueChange = { username = it },
                label = { Text(stringResource(R.string.setup_username)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )

            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text(stringResource(R.string.setup_password)) },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                modifier = Modifier.fillMaxWidth(),
            )

            error?.let { Text(text = it.message(), color = MaterialTheme.colorScheme.error) }

            if (loading) {
                CircularProgressIndicator()
            } else {
                Button(
                    enabled = normalized != null && username.isNotBlank() && password.isNotBlank(),
                    onClick = {
                        val target = normalized ?: return@Button
                        loading = true
                        error = null
                        scope.launch {
                            val result = withContext(Dispatchers.IO) { api.login(target, username, password) }
                            loading = false
                            error = when (result) {
                                is ApiResult.Success -> {
                                    secureStore.serverUrl = target
                                    secureStore.username = username
                                    secureStore.password = password
                                    onSignedIn()
                                    null
                                }
                                is ApiResult.Unauthenticated -> SetupError.InvalidCredentials
                                is ApiResult.RateLimited -> SetupError.LockedOut(result.retryAfter.inWholeSeconds)
                                is ApiResult.AccessChallenge -> SetupError.AccessChallenge
                                is ApiResult.NetworkFailure -> SetupError.Unreachable
                                is ApiResult.ServerError -> SetupError.Other(result.code)
                            }
                        }
                    },
                ) {
                    Text(stringResource(R.string.setup_sign_in))
                }
            }
        }
    }
}
