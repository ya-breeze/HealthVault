package database

import "strings"

// Each supported dataset has one registration for discovery, ownership and
// projection. New tables do not become readable by accident. Field specifications
// below are server constants, never SQL or column names supplied by callers.
var healthDatasets = makeHealthDatasets()

func healthFields(spec string) []HealthDataField {
	out := []HealthDataField{}
	for _, s := range strings.Fields(spec) {
		parts := strings.SplitN(s, ":", 2)
		unit := ""
		if len(parts) == 2 {
			unit = parts[1]
		}
		out = append(out, HealthDataField{Name: parts[0], Unit: unit, Description: strings.ReplaceAll(parts[0], "_", " ")})
	}
	return out
}

func newHealthDataset(name, table, stamp, spec, description string, soft bool) healthDataset {
	fields := healthFields("id:identifier " + stamp + ":RFC3339 " + spec)
	ds := healthDataset{HealthDataType: HealthDataType{Name: name, Description: description, TimeField: stamp, Fields: fields, Notes: []string{"Missing records are not zero measurements; timestamps select stored evidence, not complete coverage."}}, table: table, timeColumn: "d.\"" + stamp + "\"", softDelete: soft}
	for _, f := range fields {
		ds.columns = append(ds.columns, "d.\""+f.Name+"\"")
	}
	return ds
}

func makeHealthDatasets() map[string]healthDataset {
	out := map[string]healthDataset{}
	point := func(name, table, spec string) {
		out[name] = newHealthDataset(name, table, "time", "source_payload_id:import_id "+spec, "Stored "+strings.ReplaceAll(name, "_", " ")+" measurements; source_payload_id links original imported evidence when present.", true)
	}
	interval := func(name, table, spec string) {
		out[name] = newHealthDataset(name, table, "start_time", "end_time:RFC3339 source_payload_id:import_id "+spec, "Stored "+strings.ReplaceAll(name, "_", " ")+" intervals; the range selects interval starts. Intervals may overlap.", true)
	}
	point("heart_rate", "heart_rates", "bpm:beats/minute")
	point("heart_rate_variability", "heart_rate_variabilities", "rmssd_millis:milliseconds")
	point("weight", "weights", "kilograms:kg")
	point("height", "heights", "meters:m")
	// Goal weights are manually authored and carry no source payload column.
	out["weight_goal"] = newHealthDataset("weight_goal", "weight_goals", "time", "kilograms:kg", "User-authored target body weight, not a measured weight.", true)
	point("blood_pressure", "blood_pressures", "systolic:mmHg diastolic:mmHg")
	point("blood_glucose", "blood_glucoses", "mmol_per_liter:mmol/L")
	point("oxygen_saturation", "oxygen_saturations", "percentage:percent")
	point("body_temperature", "body_temperatures", "celsius:degC")
	point("skin_temperature", "skin_temperatures", "delta_celsius:degC baseline_celsius:degC measurement_location:stored_code")
	point("respiratory_rate", "respiratory_rates", "rate:breaths/minute")
	point("resting_heart_rate", "resting_heart_rates", "bpm:beats/minute")
	point("basal_metabolic_rate", "basal_metabolic_rates", "watts:W")
	point("body_fat", "body_fats", "percentage:percent")
	point("lean_body_mass", "lean_body_masses", "kilograms:kg")
	point("vo2_max", "vo2_maxes", "ml_per_kg_per_min:mL/kg/min")
	point("bone_mass", "bone_masses", "kilograms:kg")
	point("speed", "speeds", "meters_per_second:m/s")
	interval("steps", "steps", "count:steps")
	interval("distance", "distances", "meters:m")
	interval("active_calories", "active_calories", "calories:kcal")
	interval("total_calories", "total_calories", "calories:kcal")
	interval("hydration", "hydrations", "liters:L")
	interval("nutrition", "nutritions", "calories:kcal protein_grams:g carbs_grams:g fat_grams:g sugar_grams:g sodium_grams:g dietary_fiber_grams:g name:text")
	interval("exercise", "exercises", "duration_seconds:s exercise_type:stored_type distance_meters:m steps:steps avg_cadence_spm:steps/minute max_cadence_spm:steps/minute stride_length_m:m")
	out["sleep"] = newHealthDataset("sleep", "sleeps", "start_time", "session_end_time:RFC3339 source_payload_id:import_id duration_seconds:s", "Stored sleep sessions anchored at session start. Child sleep_stage rows describe recorded stages.", true)
	stages := newHealthDataset("sleep_stage", "sleep_stages", "start_time", "sleep_id:sleep_id end_time:RFC3339 stage:stored_stage duration_seconds:s", "Recorded sleep stages; identity and deletion status follow the parent sleep session.", false)
	stages.join = "JOIN sleeps p ON p.id = d.sleep_id AND p.deleted_at IS NULL"
	stages.scope = "p"
	out["sleep_stage"] = stages
	macros := "calories:kcal protein_grams:g carbs_grams:g fat_grams:g sugar_grams:g sodium_grams:g dietary_fiber_grams:g saturated_fat_grams:g"
	out["food_meal"] = newHealthDataset("food_meal", "food_meals", "logged_at", "name:text description:text cooking_context:JSON status:meal_status "+macros, "Logged meals. Only confirmed meals contribute to confirmed food totals. Excludes raw model output, photo paths and internal processing fields.", true)
	items := newHealthDataset("food_item", "food_items", "created_at", "meal_id:meal_id name:text canonical_name:text preparation:text state:text brand:text macro_source:nutrition_source weight_grams:g confidence:ratio history_original_source:nutrition_source history_original_confidence:ratio fdc_id:reference_id custom_food_id:custom_food_id off_code:reference_id "+macros, "Food items with nutrition source and confidence. The query time is the parent meal's logged_at, not recognition time. Stored macros can be estimates; unknown nutrients are not measured zeros.", true)
	// Preserve safe nutrition snapshots and independent comparison profiles.
	profileFields := healthFields("history_profile:JSON history_source_meal_id:meal_id history_source_item_id:item_id has_estimate:boolean estimated_calories_per100g:kcal/100g estimated_protein_per100g:g/100g estimated_carbs_per100g:g/100g estimated_fat_per100g:g/100g estimated_sugar_per100g:g/100g estimated_sodium_per100g:g/100g estimated_dietary_fiber_per100g:g/100g estimated_saturated_fat_per100g:g/100g has_ingredient_reference:boolean ingredient_reference_calories_per100g:kcal/100g ingredient_reference_protein_per100g:g/100g ingredient_reference_carbs_per100g:g/100g ingredient_reference_fat_per100g:g/100g ingredient_reference_sugar_per100g:g/100g ingredient_reference_sodium_per100g:g/100g ingredient_reference_dietary_fiber_per100g:g/100g ingredient_reference_saturated_fat_per100g:g/100g")
	for _, field := range profileFields {
		items.Fields = append(items.Fields, field)
		items.columns = append(items.columns, "d.\""+field.Name+"\"")
	}
	items.TimeField = "logged_at"
	items.Fields = append(items.Fields, HealthDataField{Name: "logged_at", Unit: "RFC3339", Description: "parent meal logged time"})
	items.timeColumn = "p.logged_at"
	items.columns = append(items.columns, "p.logged_at AS logged_at")
	items.join = "JOIN food_meals p ON p.id = d.meal_id AND p.user_id = d.user_id AND p.family_id = d.family_id AND p.deleted_at IS NULL"
	out["food_item"] = items
	out["custom_food"] = newHealthDataset("custom_food", "custom_foods", "created_at", "name:text canonical_name:text calories_per100g:kcal/100g protein_per100g:g/100g carbs_per100g:g/100g fat_per100g:g/100g sugar_per100g:g/100g sodium_per100g:g/100g dietary_fiber_per100g:g/100g saturated_fat_per100g:g/100g", "Personal reusable nutrition references. Values are per 100g, not food intake; time selects reference creation.", true)
	out["weather_location"] = newHealthDataset("weather_location", "weather_locations", "observed_at", "latitude:degrees longitude:degrees accuracy_m:m", "Approximate phone location observations at their observed instant; not a route or location of older health measurements.", false)
	weather := newHealthDataset("weather_hour", "weather_hours", "hour", "first_observation_id:location_id last_observation_id:location_id latitude:degrees longitude:degrees temperature_c:degC apparent_temperature_c:degC relative_humidity_percent:percent surface_pressure_hpa:hPa mean_sea_level_pressure_hpa:hPa precipitation_mm:mm wind_speed_kmh:km/h source:text model:text fetched_at:RFC3339", "Ready model-based regional weather hours, not personal exposure. Precipitation is accumulated over the hour; other fields describe its start. Missing hours do not mean normal weather.", false)
	weather.predicate = "d.status = 'ready'"
	out["weather_hour"] = weather
	out["weather_coverage"] = newHealthDataset("weather_coverage", "weather_coverages", "from", "to:RFC3339 first_observation_id:location_id last_observation_id:location_id reason:coverage_reason", "Persisted adjacent-location coverage evidence. Nonempty reasons describe gaps; empty reasons do not establish a ready weather hour. Unobserved periods can have no row.", false)
	out["webhook_import"] = newHealthDataset("webhook_import", "webhook_payloads", "received_at", "payload_ts:RFC3339 app_version:text", "Original webhook receipt metadata. Use get_health_import with id and byte offset to retrieve complete original JSON including unknown future fields. Receipt time is not measurement time; imports duplicate normalized evidence and must not be added to it.", true)
	return out
}
