package vision

import "errors"

// ErrInvalidNutritionChatToolCall marks a model-generated tool name or
// argument set that the server refuses. The OpenAI adapter returns a generic
// invalid-request tool result so the model can recover without learning any
// server or database detail.
var ErrInvalidNutritionChatToolCall = errors.New("invalid nutrition chat tool call")
