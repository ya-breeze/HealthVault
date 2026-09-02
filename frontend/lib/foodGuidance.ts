export const MAX_HINT_LENGTH = 500;
export const MAX_EXPERT_COMPONENTS = 20;
export const MAX_COMPONENT_NAME_LENGTH = 100;
export const MAX_COMBINED_COMPONENT_NAME_LENGTH = 500;
// Matches backend/pkg/server/food_describe.go's maxDescriptionLength.
export const MAX_DESCRIPTION_LENGTH = 1000;

export const unicodeLength = (value: string) => Array.from(value).length;
export const normalizedUnicodeLength = (value: string) => unicodeLength(value.trim());
