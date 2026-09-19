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
                CalorieValue(
                    resourceContext,
                    consumed,
                    isStale,
                    useReducedContent = false,
                    valueSize = 48.sp,
                    unitSize = 16.sp,
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
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
                    if (target != null) {
                        Spacer(modifier = GlanceModifier.width(6.dp))
                        PaceGlyph(summary, summary.caloriesConsumed, target.calories, fontSize = 14.sp)
                    }
                }
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
            PaceProgress(summary, summary.caloriesConsumed, target.calories, height = 8)
        }

        Spacer(modifier = GlanceModifier.defaultWeight())
        FlexWindowMacroRow(
            resourceContext,
            summary,
            R.string.widget_protein_short,
            summary.proteinGramsConsumed,
            summary.target.takeIf { it.available }?.proteinGrams ?: 0,
        )
        Spacer(modifier = GlanceModifier.defaultWeight())
        FlexWindowMacroRow(
            resourceContext,
            summary,
            R.string.widget_carbs_short,
            summary.carbsGramsConsumed,
            summary.target.takeIf { it.available }?.carbsGrams ?: 0,
        )
        Spacer(modifier = GlanceModifier.defaultWeight())
        FlexWindowMacroRow(
            resourceContext,
            summary,
            R.string.widget_fat_short,
            summary.fatGramsConsumed,
            summary.target.takeIf { it.available }?.fatGrams ?: 0,
        )
    }
}

@Composable
private fun FlexWindowMacroRow(
    resourceContext: Context,
    summary: TodaySummary,
    labelRes: Int,
    consumedGrams: Double,
    targetGrams: Int,
) {
    val consumed = consumedGrams.toInt()
    val signal = paceFor(summary, consumedGrams, targetGrams)
    Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        WidgetText(
            text = if (targetGrams > 0) {
                resourceContext.getString(
                    R.string.widget_macro_of_target,
                    resourceContext.getString(labelRes),
                    consumed,
                    targetGrams,
                )
            } else {
                resourceContext.getString(
                    R.string.widget_macro_consumed,
                    resourceContext.getString(labelRes),
                    consumed,
                )
            },
            color = GlanceTheme.colors.onSurfaceVariant,
            fontSize = 14.sp,
            maxLines = 1,
            modifier = GlanceModifier.defaultWeight(),
        )
        if (targetGrams > 0) {
            Spacer(modifier = GlanceModifier.width(12.dp))
            PaceProgress(
                summary,
                consumedGrams,
                targetGrams,
                height = 7,
                modifier = GlanceModifier.defaultWeight(),
            )
            if (signal != null) {
                Spacer(modifier = GlanceModifier.width(8.dp))
                WidgetText(
                    text = signal.direction.glyph,
                    color = paceColor(signal.level),
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Bold,
                    maxLines = 1,
                )
            }
        }
    }
}
