package net.ikoro.healthvault.ui

import androidx.appcompat.app.AppCompatDelegate
import androidx.core.os.LocaleListCompat

/**
 * Primary subtags this app ships a UI dictionary for — mirrors
 * shippedUILanguages in backend/pkg/server/display_language.go. Adding a
 * third dictionary means adding it here in the same change, exactly as that
 * file's own comment requires of its frontend counterpart.
 */
private val shippedUILanguages = setOf("en", "ru")

internal fun shippedDisplayLanguage(displayLanguage: String): String? {
    val primary = displayLanguage.substringBefore('-').substringBefore('_').lowercase()
    return primary.takeIf { it in shippedUILanguages }
}

/**
 * Applies a TodaySummary.displayLanguage value as the app's per-app locale
 * when it names a language this app ships strings for, judged by primary
 * subtag alone (matching isShippedUILanguage server-side). Falls back to the
 * device locale — via an empty override — for anything else.
 */
fun applyDisplayLanguage(displayLanguage: String) {
    val language = shippedDisplayLanguage(displayLanguage)
    val locales = if (language != null) {
        LocaleListCompat.forLanguageTags(language)
    } else {
        LocaleListCompat.getEmptyLocaleList()
    }
    AppCompatDelegate.setApplicationLocales(locales)
}
