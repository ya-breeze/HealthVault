// English strings — the source dictionary. Every other language's dictionary
// (see ru.ts) is typed against this object's keys, so a missing translation
// is a compile error rather than a silent fallback to English at some
// arbitrary spot in the UI. Deliberately not exhaustive on day one — see
// openspec/changes/russian-localization/design.md decision 6 and Non-Goals.
//
// What is covered, precisely, so this doesn't quietly overstate itself:
// the header/navigation — including the mobile bottom navigation bar and
// the More sheet it opens — the dashboard (its vitals grid, meal-attention
// line, log-food actions and secondary-metric links), the meal review screen
// (ReviewClient, MealItemRow and its ItemResolver panel), meal history, the
// custom-food catalog list, and the Expert Mode toggle.
//
// Still English regardless of Display Language: the per-type data detail
// pages, the import and login screens, the two food-entry page shells the
// dashboard's own log-food actions lead to — app/food/manual/page.tsx and
// app/food/upload/page.tsx — and the food components not on the review
// path: AddItemForm, CameraCapture, ClarifyModal, CustomFoodModal,
// DeleteMealControl, MacroSummary, ManualItemEditor, MealMetaEditor and
// ReanalyzeControl. (AddItemForm's own chrome is English, but the
// ItemResolver panel it embeds is covered, so that form is partly translated
// rather than wholly English. The steps detail page is the same shape: its
// chrome is English, but the diagnostic disclosure the check-the-health-data
// spec added — DataTypeClient.tsx's `stepsDiagnostics.*` keys — is covered.)
//
// This list is not just a comment: the display-language spec's out-of-scope
// paragraph defers to it by name ("the food entry/editing chrome enumerated
// in the frontend dictionary's scope comment"), so it is the normative
// enumeration and has to be updated in the same commit as any coverage
// change. Three rounds of code review have now corrected it rather than the
// code: it claimed the food-logging screens wholesale while the resolver
// panel was uncovered, it went stale when the dashboard was scoped in, and it
// listed only components — so the two page shells a Russian user reaches by
// tapping "Фото" or "Вручную" were English with nothing disclosing it.
//
// Extending coverage is adding keys here plus their ru.ts counterparts, and
// updating this list; nothing else has to change.
const en = {
  'header.customFoods': 'Custom Foods',
  'header.import': 'Import',
  'header.webhook': 'Webhook',
  'header.webhookUrl': 'Webhook URL',
  'header.copied': 'Copied!',
  'header.copyToClipboard': 'Copy to clipboard',
  'header.logout': 'Logout',
  'header.logoutFailed': 'Could not log you out. Try again.',
  'header.language': 'Language',
  'header.languageChangeFailed': 'Could not save your language preference. Try again.',
  'header.settings': 'Settings',

  // Mobile bottom navigation. Deliberately short: five destinations divide a
  // 320px viewport into 64px columns, and the Russian labels below are the
  // binding constraint on what fits there — not the tap targets, which clear
  // 48px on both axes at that width.
  'nav.label': 'Main',
  'nav.home': 'Home',
  'nav.photo': 'Photo',
  'nav.manual': 'Manual',
  'nav.history': 'History',
  'nav.more': 'More',
  // The More sheet reuses the header.* strings for the controls it inherits
  // (webhook, custom foods, import, settings, logout) rather than
  // duplicating them under nav.* — one control, one label, in both places.
  'nav.moreTitle': 'More',
  'nav.close': 'Close',

  'status.processing': 'Analyzing…',
  'status.pending_clarification': 'Needs clarification',
  'status.pending_review': 'Review needed',
  'status.confirmed': 'Confirmed',
  'status.failed': 'Analysis failed',

  'review.loading': 'Loading…',
  'review.mealFallbackName': 'Meal',
  'review.mealNotFound': 'Meal not found',
  'review.stillAnalyzing': 'Still analyzing…',
  'review.refresh': 'Refresh',
  'review.analysisFailed': 'Analysis failed.',
  'review.retry': 'Retry',
  'review.retrying': 'Retrying…',
  'review.noItems': 'No items.',
  'review.confirmMeal': 'Confirm Meal',
  'review.confirming': 'Confirming…',
  // Toast labels and error fallbacks raised by ReviewClient's own mutations.
  // Every per-item toast label already arrived translated from MealItemRow,
  // but they all funnel through applyMealUpdate, whose default label and
  // failure toast were hardcoded — so a Russian user's failed weight edit
  // reported itself in English on an otherwise fully Russian screen, which is
  // exactly the moment the message most needs to be readable. Found in code
  // review.
  'review.saved': 'Saved',
  'review.updateFailed': 'Update failed',
  'review.loadFailed': 'Failed to load meal',
  'review.analysisRetried': 'Analysis retried',
  'review.retryFailed': 'Retry failed',
  'review.clarificationSubmitted': 'Clarification submitted',
  'review.mealConfirmed': 'Meal confirmed',
  'review.confirmFailed': 'Confirm failed',
  // The Open Food Facts attribution is one sentence wrapping two links, so it
  // is split at the link boundaries: t() takes a key and returns a string,
  // with no placeholder substitution to interpolate elements into. The link
  // texts themselves ("Open Food Facts", "Open Database License (ODbL)") stay
  // literal in the JSX — they are a product name and a licence name, the same
  // reason item.originOff carries identical text in both dictionaries — and
  // the sentence-final "." is literal too, since both languages end it the
  // same way. Only the prose between the links is translated. This paragraph
  // was hardcoded English on a screen the scope comment above declares
  // covered, so a Russian user with any Open Food Facts-matched item met one
  // English sentence mid-page. Found in code review.
  'review.offAttributionPrefix': 'Product data for some items from ',
  'review.offAttributionMiddle': ', available under the ',

  'item.resolve': 'Resolve this item',
  'item.verifyEstimate': 'Verify this estimate',
  'item.changeMatch': 'Change match',
  'item.deleteTitle': 'Delete item',
  'item.confirm': 'Confirm',
  'item.cancel': 'Cancel',
  'item.sourceReference': 'Matched',
  'item.sourceManual': 'Manual',
  'item.sourceEstimated': 'AI estimate',
  'item.sourceNone': 'Unresolved',
  // Where a matched item's macros came from. The two reference-database
  // names are products, not prose, so ru.ts leaves them as-is.
  'item.originOff': 'Open Food Facts',
  'item.originUsda': 'USDA',
  'item.originCustomFood': 'Your custom food',
  'item.updated': 'Item updated',
  'item.removed': 'Item removed',
  'item.refreshed': 'Refreshed with latest change',
  'item.staleRefreshed': 'This item was just changed by another edit — showing its current value.',
  'item.staleRefreshFailed':
    'This item was just changed by another edit, and refreshing failed — this view may be stale. Reload the page to see what changed.',
  'item.updateWeightFailed': 'Failed to update weight',
  'item.weightMustBePositive': 'Weight must be positive to match a food',
  'item.weightPositiveEstimated': 'Weight must be positive for an estimated item',
  'item.weightPositiveMatched': 'Weight must be positive for a matched item',
  'item.deleteFailed': 'Failed to delete item',

  // Units and macro shorthand, shared by every localized screen that renders
  // a macro line (MealItemRow, meal history, the custom-food catalog, the
  // resolver's candidate list) instead of being hardcoded per call site.
  // These are not language-neutral symbols the way "kg" is — Russian writes
  // them "ккал", "г" and "Б/У/Ж" — so inlining them left a stray English
  // fragment mid-row on otherwise translated screens, while the resolver
  // panel immediately below already said "ккал/100 г". Found in code review.
  // Vitals-grid units. 'kg'/'km'/'%' are international symbols, but 'bpm' and
  // 'h'/'ms' are English abbreviations, and Russian writes the whole set in
  // Cyrillic — so all six go through the dictionary rather than only the ones
  // that obviously must.
  'unit.bpm': 'bpm',
  'unit.km': 'km',
  'unit.h': 'h',
  'unit.ms': 'ms',
  'unit.kg': 'kg',
  'unit.percent': '%',
  'unit.kcal': 'kcal',
  'unit.kcalPer100g': 'kcal/100g',
  'unit.grams': 'g',
  'unit.proteinShort': 'P',
  'unit.carbsShort': 'C',
  'unit.fatShort': 'F',

  // The item-resolution panel (ItemResolver) — the review screen's most-used
  // control, reachable for every item until the meal is confirmed.
  'resolver.tabSearch': 'Search food',
  'resolver.tabManual': 'Enter macros',
  // Prefixes the item's own name to form the tablist's accessible name; kept
  // as a prefix rather than a formatted string because `t` takes a key and
  // returns a plain string, with no interpolation.
  'resolver.modeLabel': 'Item resolution mode for',
  'resolver.search': 'Search',
  'resolver.searchFailed': 'Search failed',
  'resolver.translationNotice': 'New search terms may be sent to an external model provider for translation.',
  'resolver.searchedAs': 'Searched as:',
  'resolver.refreshTranslation': 'Refresh translation',
  'resolver.refreshTranslationEmpty': 'Could not refresh the translation — showing the previous term.',
  'resolver.refreshFailed': 'Refresh failed',
  'resolver.noMatches': 'No matches found.',
  'resolver.yourCustomFood': 'your custom food',
  'resolver.bindFailed': 'Failed to bind food',
  'resolver.name': 'Name',
  'resolver.nameRequired': 'Name is required',
  'resolver.calories': 'Calories',
  'resolver.protein': 'Protein (g)',
  'resolver.carbs': 'Carbs (g)',
  'resolver.fat': 'Fat (g)',
  'resolver.sugar': 'Sugar (g)',
  'resolver.sodium': 'Sodium (g)',
  'resolver.fiber': 'Fiber (g)',
  'resolver.saveAsCustomFood': 'Save as a reusable food, so a future photo of this dish can match it automatically',
  'resolver.save': 'Save',
  'resolver.saving': 'Saving…',
  'resolver.saveMacrosFailed': 'Failed to save macros',

  // Distinct wording from ReanalyzeControl's unrelated "Expert" input tab
  // (same page, further down) — both are legitimately called "expert"
  // something, so the visible label spells out what this one does. Must not
  // contain "name": the existing e2e suite locates the item/custom-food Name
  // field via getByLabel('Name') / label:has-text("Name"), and this toggle's
  // own accessible name sits on the same page — a substring match against
  // "name" here makes both locators strict-mode-ambiguous. Found in e2e
  // regression (food.spec.ts) after first shipping "(show original name)".
  'expertMode.toggle': 'Expert Mode (show English translation)',
  'expertMode.canonicalNamePrefix': 'English:',

  'history.title': 'Meal History',
  'history.noMeals': 'No meals logged yet.',
  'history.loadOlder': 'Load older',
  'history.loadingMore': 'Loading…',
  // Error fallbacks, used when a failure carries no message of its own. Found
  // by loading the Russian pages and reading them, after the identical class
  // of bug had been fixed in ReviewClient one round earlier and not looked for
  // anywhere else — these two screens are on the same localized list.
  'history.loadFailed': 'Failed to load meals',
  'history.loadMoreFailed': 'Failed to load more meals',

  // Day Completeness badge/controls on the history page — one below-threshold
  // day at a time (food-day-completeness capability, design.md §6 "Frontend
  // surface"). No badge/control renders at all for a `complete` day or the
  // caller's current Logged Day; see DayCompletenessControl.
  'completeness.completeBadge': 'Complete',
  'completeness.unconfirm': 'Unconfirm',
  'completeness.markComplete': 'Mark day complete',
  'completeness.updating': 'Updating…',
  'completeness.confirmFailed': 'Failed to mark day complete',
  'completeness.unconfirmFailed': 'Failed to unconfirm day',

  // Collapsed-by-default settings panel at the top of /food/history for the
  // two food-day-completeness settings (design.md §2, tasks.md 9.1-9.3):
  // `timezone` (the Logged Day boundary) and `usual_meals_per_day` (the
  // completeness threshold). See HistorySettingsPanel.
  'historySettings.title': 'Day grouping settings',
  'historySettings.timezoneLabel': 'Timezone',
  'historySettings.mealsPerDayLabel': 'Usual meals per day',
  'historySettings.save': 'Save',
  'historySettings.saving': 'Saving…',
  'historySettings.invalidInput': 'Enter a whole number of at least 1 meal per day',
  'historySettings.saveFailed': 'Failed to save settings',

  'customFood.title': 'Custom Foods',
  'customFood.addNew': '+ Add Custom Food',
  'customFood.noneYet': 'No custom foods yet.',
  'customFood.edit': 'Edit',
  'customFood.delete': 'Delete',
  'customFood.loadFailed': 'Failed to load custom foods',
  'customFood.deleteFailed': 'Delete failed',

  'dashboard.vitalsHeading': 'Vitals · last 7 days',
  // The edit mode covers both order and per-card visibility, so the control
  // that enters it is named for the mode, not for reordering alone. The
  // *.order* keys below keep their names but speak of "layout" for the same
  // reason.
  'dashboard.customize': 'Customize',
  'dashboard.done': 'Done',
  'dashboard.saving': 'Saving…',
  'dashboard.loadingOrder': 'Loading your saved layout…',
  'dashboard.orderLoadFailed': 'Could not load your saved dashboard layout. Customizing is unavailable right now.',
  'dashboard.orderSaveFailed': 'Could not save your dashboard layout. Try again.',
  'dashboard.hideCard': 'Hide {metric}',
  'dashboard.showCard': 'Show {metric}',
  'dashboard.retryLoad': 'Try again',
  'dashboard.allCardsHidden': 'All cards are hidden. Tap Customize to show some again.',
  // Four plural forms because the dictionary type is shared with ru.ts, which
  // needs all four. English only ever selects 'one' and 'other', so the two
  // middle entries are unreachable here and simply mirror 'other' — see
  // pluralForm in lib/i18n/index.ts.
  'dashboard.needsAttention.one': '{count} meal needs attention',
  'dashboard.needsAttention.few': '{count} meals need attention',
  'dashboard.needsAttention.many': '{count} meals need attention',
  'dashboard.needsAttention.other': '{count} meals need attention',
  'dashboard.logFood': 'Log food',
  'dashboard.photo': 'Photo',
  'dashboard.manual': 'Manual',
  'dashboard.history': 'History',
  'dashboard.moreData': 'More data',
  'dashboard.hideMoreData': 'Hide More data section',
  'dashboard.showMoreData': 'Show More data section',

  'vitals.noData': 'No data',
  'vitals.trend7d': '7d trend',
  'vitals.moveUp': 'Move {metric} up',
  'vitals.moveDown': 'Move {metric} down',
  // Shown on a Vital Card only when its value's source bucket isn't today's
  // (check-the-health-data spec) — e.g. today has no records yet, so the
  // card is showing yesterday's full-day total.
  'vitals.asOf': 'As of {date}',

  // The `loggingGap.` prefix is kept for the whole card, including the keys
  // below that have nothing to do with the gap. Renaming it is deferred to
  // Phase 4, when the Healthiness Label row lands and the card fully becomes a
  // nutrition card — so the churn happens once rather than twice.
  'loggingGap.title': 'Nutrition',
  'loggingGap.loading': 'Loading your nutrition…',
  'loggingGap.todayCalories': '{consumed} / {target} kcal today',
  'loggingGap.protein': 'Protein {consumed}/{target} g',
  'loggingGap.carbs': 'Carbs {consumed}/{target} g',
  'loggingGap.fat': 'Fat {consumed}/{target} g',
  'loggingGap.onTrack': 'Your log matches your weight',
  'loggingGap.hintToggle': 'Show details',
  'loggingGap.onTrackDetail': 'On your fully-logged days, intake and weight trend agree — any gap left is too small to measure.',
  'loggingGap.notEnoughData': 'Not enough data yet',
  'loggingGap.retrievalError': 'Temporarily unavailable',
  'loggingGap.targetUnmet': 'Complete your profile and goal weight to see your nutrition targets.',
  'loggingGap.targetUnmetLink': 'Update now',
  'loggingGap.unlogged': '{range} kcal/day not logged',
  'loggingGap.loggedMore': '{range} kcal/day logged more than the trend implies',
  'loggingGap.outlierNote': 'One or more weigh-ins were excluded as outliers.',
  'loggingGap.caveatPhoto': 'Logged intake is estimated from photo recognition and may carry its own bias.',
  'loggingGap.caveatActivity': "This doesn't separately account for error in your activity multiplier — a misestimate of activity can look like unlogged (or over-logged) intake.",

  // The steps detail page's diagnostic disclosure (check-the-health-data
  // spec) — collapsed by default, same hint-then-detail pattern as the
  // loggingGap keys above. The only translated strings on the per-type data
  // detail pages; see the scope comment at the top of this file.
  'stepsDiagnostics.hintToggle': 'Show diagnostic',
  'stepsDiagnostics.title': 'Step diagnostics',
  'stepsDiagnostics.columnDay': 'Day',
  'stepsDiagnostics.columnRaw': 'Raw total',
  'stepsDiagnostics.columnCollapsed': 'Counted total',
  'stepsDiagnostics.columnDropped': 'Records dropped',
  'stepsDiagnostics.columnPayloads': 'Syncs contributing',
  'stepsDiagnostics.columnLocalDay': 'Local-day total',
  'stepsDiagnostics.readingDuplicates': 'Duplicate step records in the database are inflating the raw total.',
  'stepsDiagnostics.readingMultipleSyncs': 'More than one sync wrote steps for the same day.',
  'stepsDiagnostics.readingDayBoundary': "Your local day boundary differs from this chart's UTC day.",
  'stepsDiagnostics.readingNothing': 'Nothing to report — raw, counted and local-day totals agree.',

  // One per DATA_TYPES entry. Translated rather than derived from the type id:
  // the dashboard used to render a secondary metric's label by replacing
  // underscores in its identifier, which can only ever produce English.
  'metric.steps': 'Steps',
  'metric.heart_rate': 'Heart Rate',
  'metric.heart_rate_variability': 'HRV',
  'metric.sleep': 'Sleep',
  'metric.distance': 'Distance',
  'metric.active_calories': 'Active Calories',
  'metric.total_calories': 'Total Calories',
  'metric.weight': 'Weight',
  'metric.weight_goal': 'Goal Weight',
  'metric.height': 'Height',
  'metric.blood_pressure': 'Blood Pressure',
  'metric.blood_glucose': 'Blood Glucose',
  'metric.oxygen_saturation': 'Oxygen Sat.',
  'metric.body_temperature': 'Body Temperature',
  'metric.skin_temperature': 'Skin Temperature',
  'metric.respiratory_rate': 'Respiratory Rate',
  'metric.resting_heart_rate': 'Resting Heart Rate',
  'metric.exercise': 'Exercise',
  'metric.hydration': 'Hydration',
  'metric.nutrition': 'Nutrition',
  'metric.basal_metabolic_rate': 'Basal Metabolic Rate',
  'metric.body_fat': 'Body Fat',
  'metric.lean_body_mass': 'Lean Body Mass',
  'metric.vo2_max': 'VO2 Max',
  'metric.bone_mass': 'Bone Mass',
  'metric.speed': 'Speed',
  'metric.food_meal': 'Food Meal',
};

export default en;
