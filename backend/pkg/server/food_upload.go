package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/off"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
	"github.com/ya-breeze/healthvault/pkg/usda"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

// multipartOverheadBytes is added to the request-body cap on top of
// MaxUploadBytes, so a photo legitimately right at the limit isn't rejected
// for the few extra bytes of multipart boundaries and headers around it.
//
// This cap only matters if nginx's client_max_body_size on the /api/
// location (nginx/nginx.conf) stays above MaxUploadBytes+this — raising
// HCW_MAX_UPLOAD_BYTES without also raising nginx's limit silently
// reintroduces a 413-before-the-backend-ever-sees-it bug.
const multipartOverheadBytes = 64 * 1024

// CreateMeal handles POST /api/food/meals: a multipart upload with the photo
// in the "photo" field and optional guidance in "hint". Photo-first: after
// request validation, the file is saved and the FoodMeal row
// committed as processing before the vision call runs, so no outcome of that
// call — success, failure, or timeout — can lose the photo.
func (h *foodHandlers) CreateMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+multipartOverheadBytes)
	file, _, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "photo file is required", http.StatusBadRequest)
		return
	}
	defer file.Close() //nolint:errcheck
	hint, err := normalizeHint(r.FormValue("hint"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	familyID := FamilyIDFromCtx(r)
	mealID := uuid.New()

	// The client-supplied filename (available on the discarded second return
	// value above) plays no part in the stored path — see photostorage's
	// package doc — so a traversal-shaped filename has nothing to reach.
	relPath, err := h.photos.Save(file, h.maxUploadBytes, claims.UserID, photostorage.OwnerMeal, mealID)
	if err != nil {
		writeUploadError(w, err)
		return
	}

	meal := database.FoodMeal{
		UserID:    claims.UserID,
		PhotoPath: relPath,
		Status:    database.MealStatusProcessing,
		LoggedAt:  time.Now().UTC(),
	}
	meal.ID = mealID
	meal.FamilyID = familyID
	if err := h.storage.DB().Create(&meal).Error; err != nil {
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}

	// meal.UpdatedAt, set by GORM's Create above, is this attempt's lease
	// token from the start — see analyzeMeal's doc comment.
	applied, failErr := h.analyzeMeal(r.Context(), &meal, meal.UpdatedAt, hint)
	if failErr != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	result, err := h.reloadIfSuperseded(&meal, applied)
	writeReloadedMeal(w, result, err, http.StatusCreated)
}

func writeUploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, photostorage.ErrTooLarge):
		http.Error(w, "upload exceeds maximum size", http.StatusRequestEntityTooLarge)
	case errors.Is(err, photostorage.ErrHEIC):
		http.Error(w, "HEIC images are not supported; please use JPEG, PNG, or WebP", http.StatusUnsupportedMediaType)
	case errors.Is(err, photostorage.ErrUnsupportedFormat):
		http.Error(w, "unsupported image format", http.StatusUnsupportedMediaType)
	default:
		http.Error(w, "upload error", http.StatusInternalServerError)
	}
}

// analyzeMeal runs the vision recognition call, including an optional upload
// hint (Retry passes an empty value), and persists its outcome via
// processRecognition. Any error, including a timeout, marks the meal failed
// with the photo retained. Used by the upload and retry paths, where there is
// nothing valuable to lose by falling back to failed — see runAnalysis's doc
// comment, and Reanalyze (food_reanalyze.go) for the path that does have
// something to lose and so does not use this wrapper.
//
// lease is this analysis attempt's token — the meal's updated_at at the
// moment it was claimed into processing (by Create, RetryMeal's claim, or
// ClarifyMeal's claim). It's threaded all the way to persistAnalysis and
// failMeal so a *newer* attempt claiming the same meal (RetryMeal's
// stale-processing recovery is exactly what makes this possible: a slow
// call from *this* attempt can cross the same vision-timeout threshold a
// concurrent Retry uses to decide the meal is stale) invalidates this one's
// right to write a result — see food_reanalyze.go's claim doc comment for
// the full scenario this guards against.
//
// Returns (applied, err) — see failMeal's doc comment for the distinction
// between the two: applied is whether this attempt's own outcome (success,
// or the failMeal fallback) is what actually got persisted; a non-nil err
// means failMeal's own write failed outright and must be surfaced as this
// attempt's own failure (e.g. HTTP 500), not silently treated the same as a
// lost lease. applied==false, err==nil means a newer attempt's lease
// pre-empted this one — meal's in-memory fields (still whatever the claim
// set, since neither persistAnalysis nor failMeal touched it) no longer
// reflect the database. Callers that respond to an HTTP caller with meal
// must check applied and reload the real current state rather than
// returning a stale snapshot — see reloadIfSuperseded.
func (h *foodHandlers) analyzeMeal(ctx context.Context, meal *database.FoodMeal, lease time.Time, hint string) (applied bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, h.visionTimeout)
	defer cancel()
	if err := h.runAnalysis(ctx, meal, hint, lease, false); err != nil {
		return h.failMeal(meal, lease)
	}
	return true, nil
}

// reloadIfSuperseded returns meal unchanged if applied is true (this
// attempt's own write is what's current), or a freshly reloaded copy from
// the database if not — a newer analysis attempt has since claimed the row,
// and meal's in-memory fields no longer reflect it. Propagates the reload
// error rather than falling back to the known-stale meal: applied == false
// means the in-memory meal is known not to match the database, so returning
// it as a successful response — even if the reload itself fails — would
// misrepresent the actual state, including presenting a meal that no longer
// exists (e.g. deleted by whatever operation superseded this one) as if it
// still did. Callers must use writeReloadedMeal (or equivalent 404/500
// mapping) rather than assume this always succeeds.
func (h *foodHandlers) reloadIfSuperseded(meal *database.FoodMeal, applied bool) (*database.FoodMeal, error) {
	if applied {
		return meal, nil
	}
	return h.loadOwnedMeal(meal.ID, meal.UserID)
}

// writeReloadedMeal writes meal as JSON with successStatus, or maps a
// reloadIfSuperseded error to the appropriate HTTP error: 404 if the meal no
// longer exists (owner-scoped, so this also covers "someone else's meal now"
// which can't actually happen here), 500 for any other reload failure.
func writeReloadedMeal(w http.ResponseWriter, meal *database.FoodMeal, err error, successStatus int) {
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, successStatus, meal)
}

// runAnalysis reads the stored photo and runs vision recognition with the
// given hint (empty for an unguided upload or retry), persisting the
// outcome via processRecognition on success. On failure it returns the error
// without persisting anything — the meal's status, items, and aggregate are
// left exactly as the caller found them. Callers decide how to handle that:
// analyzeMeal falls back to failMeal (safe for upload/retry, which have
// nothing valuable to lose), while Reanalyze reverts to the meal's prior
// state instead, since it can be called against a confirmed meal with real
// content behind it.
//
// strict is threaded to resolveItems — see its doc comment for what it
// changes and why Reanalyze (unlike upload/retry/clarify) must pass true.
func (h *foodHandlers) runAnalysis(ctx context.Context, meal *database.FoodMeal, hint string, lease time.Time, strict bool) error {
	photoBytes, err := h.photos.Read(meal.PhotoPath)
	if err != nil {
		return err
	}
	displayLanguage := DisplayLanguage(h.storage, meal.UserID)
	recognized, err := h.vision.Recognize(ctx, photoBytes, mimeTypeForExt(extOf(meal.PhotoPath)), hint, displayLanguage)
	if err != nil {
		return err
	}
	return h.processRecognition(ctx, meal, recognized, lease, strict, displayLanguage)
}

// processRecognition persists the outcome of a Recognize or Clarify call.
// Shared by both, since a clarify round returns the same RecognizeResult
// shape and needs the same branching:
//   - pending_clarification, with the next round's questions appended to
//     clarify_log, when the model is still unsure and the round cap
//     (database.MaxClarifyRounds) has not been reached.
//   - pending_review otherwise: every item is offered its candidate
//     shortlist and either bound or left unresolved.
//
// The meal's own aggregate is left at zero; it is computed only on confirm.
// Returns any persistence error rather than swallowing it: runAnalysis's
// callers, specifically Reanalyze, need to know a persistence failure
// happened even though the vision call itself succeeded, so they can revert
// rather than report success with a mutated meal. strict is passed straight
// through to resolveItems.
func (h *foodHandlers) processRecognition(
	ctx context.Context, meal *database.FoodMeal, recognized *vision.RecognizeResult, lease time.Time, strict bool,
	displayLanguage string,
) error {
	nextRound := meal.ClarifyRound + 1
	if len(recognized.ClarificationQuestions) > 0 && nextRound <= database.MaxClarifyRounds {
		items := unresolvedItemsFrom(recognized.Items, meal.ID, meal.UserID, meal.FamilyID)
		clarifyLog, err := buildPendingQuestionsLog(meal.ClarifyLog, nextRound, recognized.ClarificationQuestions)
		if err != nil {
			return err
		}
		return h.persistAnalysis(meal, database.MealStatusPendingClarification, recognized.Raw, items, &clarifyLog, lease)
	}

	items, err := h.resolveItems(ctx, meal, recognized.Items, strict, displayLanguage)
	if err != nil {
		return err
	}
	return h.persistAnalysis(meal, database.MealStatusPendingReview, recognized.Raw, items, nil, lease)
}

// buildPendingQuestionsLog computes the new clarify_log value: round's
// questions (each with an empty Answer, marking them unanswered) appended to
// the existing log. Pure computation, no I/O — persistAnalysis writes the
// result as part of its own transaction, so the item replacement, status
// transition, and clarify_log update commit or roll back together. A
// malformed existing log is tolerated (started fresh) rather than treated as
// fatal — recovering forward is preferable to blocking on corrupt history.
func buildPendingQuestionsLog(existingLog string, round int, questions []string) (string, error) {
	var entries []database.ClarifyEntry
	if existingLog != "" {
		json.Unmarshal([]byte(existingLog), &entries) //nolint:errcheck
	}
	for _, q := range questions {
		entries = append(entries, database.ClarifyEntry{Round: round, Question: q, Answer: ""})
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// resolveItems retrieves a candidate shortlist per recognized item, offers
// every non-empty shortlist to the model in a single Select call, and binds
// whatever it chooses — except that a chosen candidate only binds
// unconditionally when its shortlist was the exact-name custom-food
// short-circuit (a deterministic identity match). For every other selected
// candidate (ranked custom food, Open Food Facts, or USDA — all fuzzy Select
// guesses), the item's own persisted Recognize estimate now takes precedence
// whenever it is present and passes PlausibleEstimatedProfile: the candidate
// is discarded rather than bound, and the item's FdcID/CustomFoodID/OffCode
// are left nil so a discarded ranked-custom-food pick can't inflate that
// food's future rankedCustomFoodCandidates usage score once the meal is
// confirmed. An item left unresolved by every path below — no candidates,
// not selected, selected as "none of these", an out-of-range index, a
// selected candidate whose profile lookup fails, or a fuzzy candidate
// discarded in favor of the estimate — falls back to its own persisted
// Recognize estimate (MacroSource estimated) when one is present and usable,
// and only keeps MacroSource none when it is not. See
// openspec/specs/food-photo-recognition "Macro Estimate Fallback for
// Unmatched Items" and design.md decisions 1-3 (llm-first-macro-estimate).
//
// strict controls what happens when the Select call itself errors (e.g. it
// shares runAnalysis's overall timeout with Recognize, so a slow Recognize
// call can leave Select to fail with a context deadline): false (upload,
// retry, clarify — the analyzeMeal/failMeal family, which has nothing
// valuable to lose) degrades gracefully, leaving every candidate item to the
// estimate fallback (or none) rather than failing the whole analysis over a
// Select hiccup. true (Reanalyze only) returns the error instead, because
// Reanalyze's own contract is that a failure leaves the meal completely
// unchanged — silently swallowing a Select failure there would let this
// function return a "successful" but effectively unresolved item set, and
// Reanalyze would then replace a confirmed meal's real, reviewed items (and
// zero its aggregate) with that unresolved set and report 200, when the
// correct outcome for a vision-provider failure is 502 with nothing changed.
func (h *foodHandlers) resolveItems(
	ctx context.Context, meal *database.FoodMeal, recognizedItems []vision.Item, strict bool, displayLanguage string,
) ([]database.FoodItem, error) {
	items := make([]database.FoodItem, len(recognizedItems))
	candidateSets := make([][]vision.Candidate, len(recognizedItems))
	exactMatch := make([]bool, len(recognizedItems))
	itemCandidates := make([]vision.ItemCandidates, 0, len(recognizedItems))

	// Computed once per meal, not per item: usageByID, rankedCustomFoodCandidates
	// and the user's custom-food catalog (for fuzzyCustomFoodMatch) each depend
	// only on userID, so re-fetching them per item would otherwise rerun the
	// same queries identically for every recognized item in the meal (multiple
	// extra DB round-trips for a full plate instead of one).
	usageByID := h.customFoodUsageByID(meal.UserID)
	ranked := h.rankedCustomFoodCandidates(meal.UserID, usageByID)
	customFoods, err := h.customFoodsForUser(meal.UserID)
	if err != nil {
		// strict (Reanalyze only) fails the whole call on this error, the same
		// as it does for a Select-call failure below — see this function's doc
		// comment on strict for why Reanalyze can't tolerate a silent partial
		// degradation. Non-strict (upload/retry/clarify) degrades instead to
		// "no custom-food matching for this meal" rather than failing the
		// whole analysis, matching this function's existing posture toward a
		// Select-call failure there — but unlike that case, this one has no
		// other signal an operator could notice, so it's logged explicitly.
		if strict {
			return nil, err
		}
		slog.Error("resolveItems: fetch custom foods", "err", err, "user_id", meal.UserID)
		customFoods = nil
	}

	for i, ri := range recognizedItems {
		items[i] = newUnresolvedItem(ri, meal.ID, meal.UserID, meal.FamilyID)

		candidates, exact := h.retrieveCandidates(ranked, customFoods, usageByID, ri, displayLanguage)
		candidateSets[i] = candidates
		exactMatch[i] = exact
		if len(candidates) > 0 {
			itemCandidates = append(itemCandidates, vision.ItemCandidates{
				ItemIndex: i, ItemName: ri.Name, ItemBrand: ri.Brand, Candidates: candidates,
			})
		}
	}

	if len(itemCandidates) > 0 {
		sel, err := h.vision.Select(ctx, itemCandidates)
		switch {
		case err != nil && strict:
			return nil, err
		case err == nil:
			for _, s := range sel.Selections {
				if s.ItemIndex < 0 || s.ItemIndex >= len(items) {
					continue
				}
				candidates := candidateSets[s.ItemIndex]
				if s.CandidateIndex < 0 || s.CandidateIndex >= len(candidates) {
					continue // "none of these" — the estimate fallback below still applies
				}

				// A fuzzy (non-exact-name) match is only a fallback: the item's
				// own usable estimate wins instead, and the candidate is
				// discarded entirely — no identity field is stamped onto the
				// item — leaving it for the estimate fallback loop below. See
				// design.md decision 2 (llm-first-macro-estimate).
				if !exactMatch[s.ItemIndex] {
					if _, usable := items[s.ItemIndex].PlausibleEstimatedProfile(); usable {
						continue
					}
				}

				chosen := candidates[s.CandidateIndex]
				profile, ok := h.profileForCandidate(meal.UserID, chosen)
				if !ok {
					continue // profile lookup failed — the estimate fallback below still applies
				}
				items[s.ItemIndex].FdcID = chosen.FdcID
				items[s.ItemIndex].CustomFoodID = chosen.CustomFoodID
				items[s.ItemIndex].OffCode = chosen.OffCode
				items[s.ItemIndex].ApplyProfile(profile)
			}
			// err != nil && !strict falls straight through to the loop below with
			// every item still MacroSource none, same as today plus the fallback.
		}
	}

	for i := range items {
		if items[i].MacroSource == database.MacroSourceNone {
			items[i].ApplyEstimatedProfile()
		}
	}
	return items, nil
}

// rankedCustomFoodLimit bounds how many frequency/recency-ranked custom
// foods are offered as candidates alongside whatever Open Food Facts/USDA
// candidates the existing brand-based routing produces — see design.md
// decision 2.
const rankedCustomFoodLimit = 5

// customFoodUsageRow is one confirmed-meal item bound to a custom food,
// fetched unaggregated (see rankedCustomFoodCandidates for why: SQLite's
// driver returns a MAX(logged_at) aggregate as a plain string with no
// column-type metadata attached, which the standard library's default
// scanner cannot convert into time.Time — a plain per-row select of the
// column, by contrast, still carries that metadata and scans cleanly. usage
// and recency are aggregated in Go instead, over what is expected to be a
// small number of rows for a single family's data).
type customFoodUsageRow struct {
	CustomFoodID uuid.UUID
	LoggedAt     time.Time
}

// customFoodUsage is one custom food's aggregated usage, computed from
// customFoodUsageRow rows.
type customFoodUsage struct {
	CustomFoodID uuid.UUID
	UsageCount   int
	LastUsed     time.Time
}

// usageScore weights usage frequency higher than recency, so a dish
// confirmed only monthly still outranks a food used once recently — see
// design.md decision 2. Recency contributes a bounded (0,1] term that decays
// toward 0 as the last use gets older, never able to outweigh even a single
// additional use. now is passed in rather than read internally via
// time.Since so that every score in a single ranking pass is computed
// against the same instant — see rankedCustomFoodCandidates.
func usageScore(u customFoodUsage, now time.Time) float64 {
	const frequencyWeight = 10.0
	daysSinceUse := now.Sub(u.LastUsed).Hours() / 24
	if daysSinceUse < 0 {
		daysSinceUse = 0
	}
	recencyScore := 1.0 / (1.0 + daysSinceUse)
	return float64(u.UsageCount)*frequencyWeight + recencyScore
}

// customFoodUsageByID computes every one of the caller's custom foods' usage
// (confirmed-meal count + most-recent use), keyed by CustomFoodID, from their
// confirmed meal history only — a still-pending_review meal's item already
// carries a custom_food_id from this very matching process, so counting it
// here would let an unverified, possibly-wrong automatic match reinforce its
// own future ranking before the user ever confirmed it was correct. See
// design.md decision 2 for the exact query and why the join key is meal_id,
// not user_id. Computed once per meal by resolveItems and shared by
// rankedCustomFoodCandidates (ranking) and fuzzyCustomFoodMatch's tie-break
// (design.md decision 5's "most-recently-used") rather than queried
// separately by each.
func (h *foodHandlers) customFoodUsageByID(userID uuid.UUID) map[uuid.UUID]customFoodUsage {
	var rows []customFoodUsageRow
	// .Table() bypasses GORM's model-level deleted_at scope (that scoping is
	// attached to the FoodItem/FoodMeal models, not to a raw table query), so
	// the deleted_at IS NULL predicates below are explicit: without them, a
	// match the user removed via DeleteMealItem — or an entire meal removed
	// via the generic DELETE /api/data/food_meal endpoint — would still count
	// toward and keep reinforcing that food's future ranking.
	err := h.storage.DB().
		Table("food_items").
		Select("food_items.custom_food_id AS custom_food_id, food_meals.logged_at AS logged_at").
		Joins("JOIN food_meals ON food_items.meal_id = food_meals.id").
		Where("food_items.user_id = ? AND food_items.custom_food_id IS NOT NULL AND food_meals.status = ? "+
			"AND food_items.deleted_at IS NULL AND food_meals.deleted_at IS NULL",
			userID, database.MealStatusConfirmed).
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil
	}

	byFood := make(map[uuid.UUID]customFoodUsage, len(rows))
	for _, r := range rows {
		u := byFood[r.CustomFoodID]
		u.CustomFoodID = r.CustomFoodID
		u.UsageCount++
		if r.LoggedAt.After(u.LastUsed) {
			u.LastUsed = r.LoggedAt
		}
		byFood[r.CustomFoodID] = u
	}
	return byFood
}

// rankedCustomFoodCandidates returns userID's own custom foods ranked by
// usageScore, computed from usageByID (see customFoodUsageByID).
func (h *foodHandlers) rankedCustomFoodCandidates(
	userID uuid.UUID, usageByID map[uuid.UUID]customFoodUsage,
) []vision.Candidate {
	if len(usageByID) == 0 {
		return nil
	}
	usage := make([]customFoodUsage, 0, len(usageByID))
	for _, u := range usageByID {
		usage = append(usage, u)
	}

	// usage is built from a Go map above, so its starting order is already
	// randomized on every call — sort.SliceStable alone would not make this
	// deterministic, since "stable" only preserves whatever (random) input
	// order two tied elements arrived in. Breaking ties on CustomFoodID
	// instead makes the result deterministic outright: two foods tied on
	// usageScore (equal count, near-equal recency) get a consistent relative
	// order across repeated, otherwise-identical requests, so which one gets
	// cut by rankedCustomFoodLimit below can't flip from one upload to the next.
	//
	// Scores are precomputed once against a single `now` rather than calling
	// usageScore(u) — which reads time.Since internally — from inside the
	// comparator: sort's contract requires less(i,j) and less(j,i) to be
	// consistent for the lifetime of the sort, but two calls to time.Since
	// made at different instants can each return a (however slightly)
	// different score for the same tied pair, so both could come back true
	// depending on evaluation order — violating that contract and silently
	// bypassing the CustomFoodID tie-breaker below. Sorted by index, not by
	// usage directly, since sort.Slice only permutes the slice it's given —
	// sorting usage directly while indexing into a same-length scores slice
	// from the comparator would desync the two after the first swap.
	now := time.Now()
	scores := make([]float64, len(usage))
	for i, u := range usage {
		scores[i] = usageScore(u, now)
	}
	order := make([]int, len(usage))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if scores[a] != scores[b] {
			return scores[a] > scores[b]
		}
		return usage[a].CustomFoodID.String() < usage[b].CustomFoodID.String()
	})
	sorted := make([]customFoodUsage, len(usage))
	for i, idx := range order {
		sorted[i] = usage[idx]
	}
	usage = sorted
	if len(usage) > rankedCustomFoodLimit {
		usage = usage[:rankedCustomFoodLimit]
	}

	ids := make([]uuid.UUID, len(usage))
	for i, u := range usage {
		ids[i] = u.CustomFoodID
	}
	var foods []database.CustomFood
	if err := h.storage.DB().Where("id IN ? AND user_id = ?", ids, userID).Find(&foods).Error; err != nil {
		return nil
	}
	byID := make(map[uuid.UUID]database.CustomFood, len(foods))
	for _, f := range foods {
		byID[f.ID] = f
	}

	out := make([]vision.Candidate, 0, len(usage))
	for _, u := range usage {
		f, ok := byID[u.CustomFoodID]
		if !ok {
			continue
		}
		id := f.ID
		out = append(out, vision.Candidate{CustomFoodID: &id, Description: f.Name})
	}
	return out
}

// retrieveCandidates mirrors Search's *precedence* rule — a custom-food name
// match wins outright over Open Food Facts/USDA, not additionally queried
// for it — not its matching algorithm: Search (food.go) still does an exact,
// case-insensitive match, per food-search-translation's "Custom Food
// Matching Is Unaffected By Translation" (a separate, already-shipped
// requirement this change does not touch), while here a fuzzy-name custom
// food match (see fuzzyMatchThreshold, design.md decision 5) wins outright,
// offered as the sole candidate. Otherwise, ranked (the caller's
// frequency/recency-ranked custom foods, see rankedCustomFoodCandidates —
// computed once per meal by the caller, not per item, since it depends only
// on userID) is combined with whatever Open Food Facts/USDA candidates the
// existing brand-based routing produces: a non-empty brand routes to Open
// Food Facts first — its candidates are used if it returns any, falling back
// to USDA only when OFF returns none (including when no OFF database is open
// at all). An empty brand skips OFF entirely and queries USDA directly:
// there is no signal to safely select among differently-branded OFF products
// for a brandless query. See design.md "OFF queried only when a brand was
// extracted, USDA as the fallback".
//
// When displayLanguage is not English, Open Food Facts and USDA are not
// queried at all, regardless of brand — both are English-vocabulary
// reference databases and no attempt is made to translate the Display Name
// back to English for matching. See design.md decision 4. The fuzzy
// custom-food match above is unaffected by language: it matches against the
// user's own catalog, not an English-vocabulary reference database.
//
// The bool return is true only for the fuzzy-name custom-food short-circuit
// — see fuzzyMatchThreshold and design.md decision 5 for why a fuzzy hit is
// still trusted with unconditional-bind weight — and false for every other
// shortlist (ranked custom food, Open Food Facts, or USDA), including when
// the shortlist happens to contain exactly one candidate: shortlist length
// is not a safe proxy for exactness, since a fuzzy search can also
// legitimately return a single result. The caller uses this to decide
// whether a selected candidate binds unconditionally or is only a fallback
// to the item's own estimate — see resolveItems and design.md decision 1.
//
// ranked is only ever read here, never appended to in place: it is shared
// across every recognized item in the meal, so growing it in place via a
// plain append could, when it has spare capacity, overwrite data written by
// a sibling call for a different item in the same meal.
//
// The recognized item is passed whole rather than as its Name/Preparation/
// State/Brand fields unpacked into four strings: they are read here only to
// describe one item, they always travel together, and taking them apart at
// the call site made this a positional eight-argument call in which four
// adjacent strings had nothing but their order to tell them apart.
func (h *foodHandlers) retrieveCandidates(
	ranked []vision.Candidate, customFoods []database.CustomFood, usageByID map[uuid.UUID]customFoodUsage,
	ri vision.Item, displayLanguage string,
) ([]vision.Candidate, bool) {
	if best, ok := fuzzyCustomFoodMatch(customFoods, ri.Name, usageByID); ok {
		id := best.ID
		return []vision.Candidate{{CustomFoodID: &id, Description: best.Name}}, true
	}

	if !vision.IsEnglishDisplayLanguage(displayLanguage) {
		return ranked, false
	}

	if ri.Brand != "" && h.off != nil {
		foods, offErr := h.off.Search(ri.Name, ri.Brand, off.DefaultCandidates)
		if offErr == nil && len(foods) > 0 {
			out := make([]vision.Candidate, 0, len(ranked)+len(foods))
			out = append(out, ranked...)
			for _, f := range foods {
				code := f.Code
				out = append(out, vision.Candidate{OffCode: &code, Brands: f.Brands, Description: f.ProductName})
			}
			return out, false
		}
	}

	if h.usda != nil {
		term := usda.QueryFor(ri.Name, ri.Preparation, ri.State)
		foods, usdaErr := h.usda.Search(term, usda.DefaultCandidates)
		if usdaErr == nil {
			out := make([]vision.Candidate, 0, len(ranked)+len(foods))
			out = append(out, ranked...)
			for _, f := range foods {
				fdcID := f.FdcID
				out = append(out, vision.Candidate{FdcID: &fdcID, Description: f.Description})
			}
			return out, false
		}
	}
	return ranked, false
}

// fuzzyCustomFoodMatch scores every one of foods (the caller's own custom
// foods, fetched once per meal — see resolveItems) against name (the
// recognized item's Display Name) and returns the highest-similarity one
// that clears fuzzyMatchThreshold, the sameDigitsIn veto, and the
// fuzzyMinNearMatchLen length gate (all fuzzy.go), or
// ok=false when none does (including when foods is empty). Ties are broken
// by most-recently-used — see design.md decision 5 — using usageByID
// (customFoodUsageByID's confirmed-meal LastUsed, not the row's own
// UpdatedAt: a food edited today but never actually logged must not outrank
// one used almost daily whose row simply hasn't been touched since
// creation). A food absent from usageByID (never
// confirmed in a meal) has the zero LastUsed value, so it only wins a tie
// against another equally-unused food. Matching runs against
// CustomFood.Name (the Display Name), the language the user actually
// recognizes their own catalog entries in.
func fuzzyCustomFoodMatch(
	foods []database.CustomFood, name string, usageByID map[uuid.UUID]customFoodUsage,
) (database.CustomFood, bool) {
	var best database.CustomFood
	bestScore := -1.0
	found := false
	for _, f := range foods {
		// Names differing only in a number are vetoed outright regardless of
		// score — see sameDigitsIn (fuzzy.go).
		if !sameDigitsIn(f.Name, name) {
			continue
		}
		score := fuzzySimilarity(f.Name, name)
		if score < fuzzyMatchThreshold {
			continue
		}
		// Short names must match exactly (after normalization); only longer
		// ones get the threshold's tolerance — see fuzzyMinNearMatchLen
		// (fuzzy.go) for why the length-normalized score alone is too
		// permissive down there.
		if score < 1 && !nearMatchAllowed(f.Name, name) {
			continue
		}
		if !found || fuzzyMatchBetter(f, score, best, bestScore, usageByID) {
			best, bestScore, found = f, score, true
		}
	}
	return best, found
}

// fuzzyMatchBetter reports whether candidate f (scoring score) should
// replace best (scoring bestScore) as the current-best fuzzy match: a higher
// score wins outright; a tied score is broken by most-recently-used (design.md
// decision 5); and a tie on that too — e.g. two never-logged custom foods,
// both with a zero usageByID LastUsed — falls back to CustomFoodID, so the
// result is deterministic across otherwise-identical requests instead of
// depending on customFoodsForUser's unordered row return order. Mirrors
// rankedCustomFoodCandidates's identical CustomFoodID tie-breaker.
func fuzzyMatchBetter(
	f database.CustomFood, score float64, best database.CustomFood, bestScore float64,
	usageByID map[uuid.UUID]customFoodUsage,
) bool {
	if score != bestScore {
		return score > bestScore
	}
	fLast, bLast := usageByID[f.ID].LastUsed, usageByID[best.ID].LastUsed
	if !fLast.Equal(bLast) {
		return fLast.After(bLast)
	}
	return f.ID.String() < best.ID.String()
}

func (h *foodHandlers) profileForCandidate(userID uuid.UUID, c vision.Candidate) (database.NutrientProfile, bool) {
	if c.CustomFoodID != nil {
		cf, err := h.findOwnedCustomFood(*c.CustomFoodID, userID)
		if err != nil {
			return database.NutrientProfile{}, false
		}
		return cf.Profile(), true
	}
	if c.FdcID != nil && h.usda != nil {
		food, err := h.usda.ByFdcID(*c.FdcID)
		if err != nil || food == nil {
			return database.NutrientProfile{}, false
		}
		return food.Profile, true
	}
	if c.OffCode != nil && h.off != nil {
		food, err := h.off.ByCode(*c.OffCode)
		if err != nil || food == nil {
			return database.NutrientProfile{}, false
		}
		return food.Profile, true
	}
	return database.NutrientProfile{}, false
}

// newUnresolvedItem builds a FoodItem from one recognized vision.Item,
// carrying over every recognition-time field before any candidate binding is
// attempted. Shared by resolveItems (whose copy may be bound further right
// after this) and unresolvedItemsFrom (the pending_clarification degraded
// path, which never binds at all) so the two field lists can't drift apart —
// each was previously hand-rolled independently, and this change already had
// to add CanonicalName to both by hand.
func newUnresolvedItem(ri vision.Item, mealID, userID, familyID uuid.UUID) database.FoodItem {
	item := database.FoodItem{
		UserID: userID, MealID: mealID, Name: ri.Name, CanonicalName: ri.CanonicalName,
		Preparation: ri.Preparation, State: ri.State, Brand: ri.Brand,
		WeightGrams: ri.WeightGrams, Confidence: ri.Confidence,
		MacroSource: database.MacroSourceNone,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	item.SetEstimatedProfile(ri.EstimatedProfile)
	return item
}

func unresolvedItemsFrom(recognizedItems []vision.Item, mealID, userID, familyID uuid.UUID) []database.FoodItem {
	items := make([]database.FoodItem, len(recognizedItems))
	for i, ri := range recognizedItems {
		items[i] = newUnresolvedItem(ri, mealID, userID, familyID)
	}
	return items
}

// persistAnalysis replaces the meal's FoodItem rows and writes its status in
// one transaction, so a re-analysis (retry, reanalyze, or a clarify round
// moving to pending_review) can never append a second set alongside the
// first. It also unconditionally zeros the meal's seven stored aggregate
// columns: for upload/clarify/retry that is a no-op (the aggregate is
// already zero, since none of those paths can be reached from confirmed),
// but Reanalyze can run from confirmed, and without this a meal leaving
// confirmed would carry its old totals forward against a brand-new,
// unreviewed item set.
//
// clarifyLog, when non-nil, is written in the same transaction as the item
// replacement — see processRecognition's pending_clarification branch. It
// must not be a separate statement after this transaction commits: Reanalyze
// relies on "persistAnalysis either fully applies or changes nothing" to
// decide whether to revert, and a clarify_log write that could fail on its
// own, after the items/status/aggregate were already committed, would leave
// exactly the inconsistent state revertReanalyze can't fix — status reverted
// to what it was, but items and aggregate already replaced.
//
// errLeaseLost is returned by persistAnalysis when lease no longer matches
// the meal's current updated_at — a newer analysis attempt (see
// analyzeMeal's doc comment) has since claimed the row, and this attempt's
// result must not be written over it.
var errLeaseLost = errors.New("meal was claimed by a newer analysis attempt")

// Returns any transaction error instead of handling it — callers decide what
// "failed to persist" means for them. analyzeMeal's callers (upload, retry)
// fall back to failMeal, which is safe for them (nothing valuable to lose).
// Reanalyze does not: a persistence failure there must revert to the meal's
// prior state, not fall back to marking it failed and returning 200, which
// is exactly what happened before this function reported its own errors —
// it silently called failMeal internally and Reanalyze had no way to see
// that the "success" it thought it had wasn't one.
//
// lease conditions the final meal write on updated_at still matching what
// this attempt's own claim wrote — see analyzeMeal's doc comment for why
// that can legitimately not be true by the time this runs. Item
// delete+create happen inside the same transaction as that conditional
// write, so a lost lease rolls back the item replacement too, not just
// skips the meal-row update.
func (h *foodHandlers) persistAnalysis(
	meal *database.FoodMeal, status, rawResponse string, items []database.FoodItem, clarifyLog *string, lease time.Time,
) error {
	updates := map[string]any{
		"status": status, "raw_response": rawResponse,
		"calories": 0, "protein_grams": 0, "carbs_grams": 0, "fat_grams": 0,
		"sugar_grams": 0, "sodium_grams": 0, "dietary_fiber_grams": 0,
	}
	if clarifyLog != nil {
		updates["clarify_log"] = *clarifyLog
	}
	err := h.storage.DB().Transaction(func(tx *gorm.DB) error {
		// Unscoped: FoodItem embeds TenantModel, so a plain Delete soft-deletes
		// (sets deleted_at) rather than removing the row. GORM's own reads
		// (Preload("Items"), Find) filter deleted_at automatically, so the app
		// never shows the stale rows — but they'd never actually go away
		// either, growing without bound across retries and clarify rounds, and
		// contradicting "replaces" above.
		if err := tx.Unscoped().Where("meal_id = ?", meal.ID).Delete(&database.FoodItem{}).Error; err != nil {
			return err
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&database.FoodMeal{}).Where("id = ? AND updated_at = ?", meal.ID, lease).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errLeaseLost
		}
		return nil
	})
	if err != nil {
		return err
	}
	meal.Status = status
	meal.RawResponse = rawResponse
	meal.Items = items
	meal.Calories, meal.ProteinGrams, meal.CarbsGrams, meal.FatGrams = 0, 0, 0, 0
	meal.SugarGrams, meal.SodiumGrams, meal.DietaryFiberGrams = 0, 0, 0
	if clarifyLog != nil {
		meal.ClarifyLog = *clarifyLog
	}
	return nil
}

// failMeal marks meal failed, conditioned on lease still matching — see
// persistAnalysis. If a newer attempt has since claimed the row, this is a
// silent no-op (returns applied=false, err=nil): that attempt owns the meal
// now, and this one reporting failure over it would be wrong regardless of
// what this attempt itself observed. Callers must treat that case as
// "reload the real current state" (see reloadIfSuperseded), not as a
// failure of their own.
//
// A non-nil err is a different situation entirely: the conditional UPDATE
// itself failed (a real database error), not a lost lease. Reloading and
// responding 200/201 in that case would hide the failure and, if the reload
// happens to succeed, could report the meal as still `processing` with no
// indication anything went wrong. Callers must surface a non-nil err as
// their own failure (e.g. HTTP 500), the same way they already would if the
// original vision call itself had errored before ever reaching failMeal.
func (h *foodHandlers) failMeal(meal *database.FoodMeal, lease time.Time) (applied bool, err error) {
	res := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND updated_at = ?", meal.ID, lease).
		Update("status", database.MealStatusFailed)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected > 0 {
		meal.Status = database.MealStatusFailed
		return true, nil
	}
	return false, nil
}

// extOf returns the stored file extension from a photostorage relative path
// ("{user_id}/{owner_kind}/{owner_id}.{ext}"), or "" if there isn't one.
func extOf(relPath string) string {
	i := strings.LastIndexByte(relPath, '.')
	if i < 0 {
		return ""
	}
	return relPath[i+1:]
}

func mimeTypeForExt(ext string) string {
	return contentTypeForExt(ext)
}
