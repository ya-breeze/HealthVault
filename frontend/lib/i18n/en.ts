// English strings — the source dictionary. Every other language's dictionary
// (see ru.ts) is typed against this object's keys, so a missing translation
// is a compile error rather than a silent fallback to English at some
// arbitrary spot in the UI. Deliberately not exhaustive on day one — see
// openspec/changes/russian-localization/design.md decision 6 and Non-Goals:
// navigation and the food-logging screens are covered; the rest of the app
// still reads in English regardless of Display Language.
const en = {
  'header.customFoods': 'Custom Foods',
  'header.import': 'Import',
  'header.webhook': 'Webhook',
  'header.webhookUrl': 'Webhook URL',
  'header.copied': 'Copied!',
  'header.copyToClipboard': 'Copy to clipboard',
  'header.logout': 'Logout',
  'header.language': 'Language',

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

  'customFood.title': 'Custom Foods',
  'customFood.addNew': '+ Add Custom Food',
  'customFood.noneYet': 'No custom foods yet.',
  'customFood.edit': 'Edit',
  'customFood.delete': 'Delete',
};

export default en;
