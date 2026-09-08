// Root build file: declares plugin versions once via the version catalogue
// (gradle/libs.versions.toml) and applies them with `apply false` so `:app`
// opts in without re-declaring versions.
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
}
