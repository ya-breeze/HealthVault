import type { Dictionary } from './index';

// Russian strings. Typed as Dictionary (keyed against en.ts) so a key added
// to en.ts without a Russian counterpart is a compile error, not a silent
// English fallback mid-sentence.
const ru: Dictionary = {
  'header.customFoods': 'Свои продукты',
  'header.import': 'Импорт',
  'header.webhook': 'Вебхук',
  'header.webhookUrl': 'URL вебхука',
  'header.copied': 'Скопировано!',
  'header.copyToClipboard': 'Скопировать',
  'header.logout': 'Выйти',
  'header.language': 'Язык',

  'status.processing': 'Анализ…',
  'status.pending_clarification': 'Требуется уточнение',
  'status.pending_review': 'Требуется проверка',
  'status.confirmed': 'Подтверждено',
  'status.failed': 'Ошибка анализа',

  'review.loading': 'Загрузка…',
  'review.mealFallbackName': 'Приём пищи',
  'review.mealNotFound': 'Приём пищи не найден',
  'review.stillAnalyzing': 'Анализ ещё идёт…',
  'review.refresh': 'Обновить',
  'review.analysisFailed': 'Анализ не удался.',
  'review.retry': 'Повторить',
  'review.retrying': 'Повтор…',
  'review.noItems': 'Нет блюд.',
  'review.confirmMeal': 'Подтвердить приём пищи',
  'review.confirming': 'Подтверждение…',

  'item.resolve': 'Определить блюдо',
  'item.verifyEstimate': 'Проверить оценку',
  'item.changeMatch': 'Изменить соответствие',
  'item.deleteTitle': 'Удалить блюдо',
  'item.confirm': 'Подтвердить',
  'item.cancel': 'Отмена',
  'item.sourceReference': 'Найдено',
  'item.sourceManual': 'Вручную',
  'item.sourceEstimated': 'Оценка ИИ',
  'item.sourceNone': 'Не определено',

  'expertMode.toggle': 'Режим эксперта (показать перевод на английский)',
  'expertMode.canonicalNamePrefix': 'По-английски:',

  'history.title': 'История приёмов пищи',
  'history.noMeals': 'Пока нет записей.',
  'history.loadOlder': 'Загрузить ещё',
  'history.loadingMore': 'Загрузка…',

  'customFood.title': 'Свои продукты',
  'customFood.addNew': '+ Добавить продукт',
  'customFood.noneYet': 'Пока нет своих продуктов.',
  'customFood.edit': 'Изменить',
  'customFood.delete': 'Удалить',
};

export default ru;
