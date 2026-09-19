package net.ikoro.healthvault.widget

import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.GlanceId
import androidx.glance.GlanceModifier
import androidx.glance.GlanceTheme
import androidx.glance.ImageProvider
import androidx.glance.action.clickable
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.LinearProgressIndicator
import androidx.glance.appwidget.action.actionRunCallback
import androidx.glance.appwidget.action.actionStartActivity
import androidx.glance.appwidget.appWidgetBackground
import androidx.glance.appwidget.components.CircleIconButton
import androidx.glance.appwidget.components.SquareIconButton
import androidx.glance.appwidget.cornerRadius
import androidx.glance.appwidget.provideContent
import androidx.glance.background
import androidx.glance.layout.Alignment
import androidx.glance.layout.Box
import androidx.glance.layout.Column
import androidx.glance.layout.Row
import androidx.glance.layout.Spacer
import androidx.glance.layout.fillMaxSize
import androidx.glance.layout.fillMaxWidth
import androidx.glance.layout.height
import androidx.glance.layout.padding
import androidx.glance.layout.width
import androidx.glance.semantics.contentDescription
import androidx.glance.semantics.semantics
import androidx.glance.text.FontWeight
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.TodaySummary
import net.ikoro.healthvault.ui.MainActivity

/** Samsung FlexWindow uses a separate, fixed near-square surface from the resizable home widget. */
class FlexWindowSummaryWidget : GlanceAppWidget() {
    override suspend fun provideGlance(context: Context, id: GlanceId) {
        val app = context.applicationContext as HealthVaultApp
        val snapshot = app.secureStore.loadSnapshot()
        val resourceContext = localizedResourceContext(context, snapshot?.summary?.displayLanguage)
        val state = widgetState(
            summary = snapshot?.summary,
            fetchedAtMillis = snapshot?.fetchedAtMillis,
            nowMillis = System.currentTimeMillis(),
            hasSession = app.secureStore.hasSession(),
            refreshFailed = app.secureStore.refreshFailed,
        )

        provideContent {
            GlanceTheme {
                FlexWindowContent(state, resourceContext)
            }
        }
    }
}

@Composable
private fun FlexWindowContent(state: WidgetState, resourceContext: Context) {
    val backgroundModifier = GlanceModifier.fillMaxSize().appWidgetBackground()
    val cardModifier = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
        backgroundModifier
            .background(GlanceTheme.colors.widgetBackground)
            .cornerRadius(R.dimen.widget_corner_radius)
    } else {
        backgroundModifier.background(ImageProvider(R.drawable.widget_background))
    }
    val accessibleCardModifier = cardModifier.semantics {
        contentDescription = widgetContentDescription(resourceContext, state)
    }
    val interactiveCardModifier = when (widgetTapTarget(state)) {
        WidgetTapTarget.LOG_FOOD ->
            accessibleCardModifier.clickable(actionRunCallback<FlexWindowLogFoodAction>())
        WidgetTapTarget.OPEN_APP ->
            accessibleCardModifier.clickable(actionStartActivity(Intent(resourceContext, MainActivity::class.java)))
    }

    Box(modifier = interactiveCardModifier.padding(16.dp)) {
        when (state) {
            is WidgetState.Loaded -> FlexWindowSummary(
                resourceContext,
                state.summary,
                isStale = false,
            )
            is WidgetState.Stale -> FlexWindowSummary(
                resourceContext,
                state.summary,
                isStale = true,
            )
            is WidgetState.SignedOut -> FlexWindowMessage(
                resourceContext.getString(R.string.widget_sign_in),
                resourceContext.getString(R.string.widget_open_to_sign_in),
            )
            is WidgetState.Error -> FlexWindowMessage(
                resourceContext.getString(R.string.app_name),
                resourceContext.getString(R.string.widget_no_data),
            )
        }
    }
}

@Composable
private fun FlexWindowMessage(title: String, message: String) {
    Column(modifier = GlanceModifier.fillMaxSize(), verticalAlignment = Alignment.CenterVertically) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            WidgetBrandMark(size = 28)
            Spacer(modifier = GlanceModifier.width(4.dp))
            WidgetText(text = title, fontSize = 22.sp, fontWeight = FontWeight.Bold, maxLines = 1)
        }
        Spacer(modifier = GlanceModifier.height(8.dp))
        WidgetText(
            text = message,
            color = GlanceTheme.colors.onSurfaceVariant,
            fontSize = 14.sp,
            maxLines = 2,
        )
    }
}

@Composable
private fun FlexWindowSummary(
    resourceContext: Context,
    summary: TodaySummary,
    isStale: Boolean,
) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }

    Column(modifier = GlanceModifier.fillMaxSize()) {
        Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = GlanceModifier.defaultWeight()) {
                WidgetHeader(resourceContext, isStale, markSize = 24, fontSize = 14.sp)
                WidgetText(
                    text = consumed.toString(),
                    fontSize = 48.sp,
                    fontWeight = FontWeight.Bold,
                    maxLines = 1,
                )
                WidgetText(
                    text = target?.let {
                        resourceContext.getString(
                            R.string.widget_calorie_target_progress,
                            it.calories,
                            progressPercent(consumed, it.calories),
                        )
                    } ?: resourceContext.getString(R.string.widget_calories_today),
                    color = GlanceTheme.colors.onSurfaceVariant,
                    fontSize = 14.sp,
                    maxLines = 1,
                )
            }
            SquareIconButton(
                imageProvider = ImageProvider(R.drawable.ic_add_24),
                contentDescription = resourceContext.getString(R.string.widget_log_food),
                onClick = actionRunCallback<FlexWindowLogFoodAction>(),
            )
            CircleIconButton(
                imageProvider = ImageProvider(R.drawable.ic_refresh_24),
                contentDescription = resourceContext.getString(R.string.widget_refresh),
                onClick = actionRunCallback<RefreshAction>(),
                backgroundColor = null,
                contentColor = GlanceTheme.colors.onSurfaceVariant,
            )
        }

        if (target != null) {
            Spacer(modifier = GlanceModifier.height(8.dp))
            LinearProgressIndicator(
                progress = progressFraction(consumed, target.calories),
                modifier = GlanceModifier.fillMaxWidth().height(8.dp),
                color = GlanceTheme.colors.primary,
                backgroundColor = GlanceTheme.colors.surfaceVariant,
            )
        }

        Spacer(modifier = GlanceModifier.defaultWeight())
        FlexWindowMacroRow(
            resourceContext,
            R.string.widget_protein_short,
            summary.proteinGramsConsumed.toInt(),
            summary.target.takeIf { it.available }?.proteinGrams ?: 0,
        )
        Spacer(modifier = GlanceModifier.defaultWeight())
        FlexWindowMacroRow(
            resourceContext,
            R.string.widget_carbs_short,
            summary.carbsGramsConsumed.toInt(),
            summary.target.takeIf { it.available }?.carbsGrams ?: 0,
        )
        Spacer(modifier = GlanceModifier.defaultWeight())
        FlexWindowMacroRow(
            resourceContext,
            R.string.widget_fat_short,
            summary.fatGramsConsumed.toInt(),
            summary.target.takeIf { it.available }?.fatGrams ?: 0,
        )
    }
}

@Composable
private fun FlexWindowMacroRow(
    resourceContext: Context,
    labelRes: Int,
    consumedGrams: Int,
    targetGrams: Int,
) {
    Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        WidgetText(
            text = if (targetGrams > 0) {
                resourceContext.getString(
                    R.string.widget_macro_of_target,
                    resourceContext.getString(labelRes),
                    consumedGrams,
                    targetGrams,
                )
            } else {
                resourceContext.getString(
                    R.string.widget_macro_consumed,
                    resourceContext.getString(labelRes),
                    consumedGrams,
                )
            },
            color = GlanceTheme.colors.onSurfaceVariant,
            fontSize = 14.sp,
            maxLines = 1,
            modifier = GlanceModifier.defaultWeight(),
        )
        if (targetGrams > 0) {
            Spacer(modifier = GlanceModifier.width(12.dp))
            LinearProgressIndicator(
                progress = progressFraction(consumedGrams, targetGrams),
                modifier = GlanceModifier.defaultWeight().height(7.dp),
                color = GlanceTheme.colors.primary,
                backgroundColor = GlanceTheme.colors.surfaceVariant,
            )
        }
    }
}
