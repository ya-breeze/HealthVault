package mcpserver

const instructions = `HealthVault stores health data received from the HC Webhook Android app.

Data types available: steps, heart_rate, heart_rate_variability, sleep, distance,
active_calories, total_calories, weight, height, blood_pressure, blood_glucose,
oxygen_saturation, body_temperature, skin_temperature, respiratory_rate,
resting_heart_rate, exercise, hydration, nutrition, basal_metabolic_rate,
body_fat, lean_body_mass, vo2_max, bone_mass.

Use list_users to see available users, then query_data or summary to retrieve health data.
Time parameters use RFC3339 format (e.g. 2026-06-24T00:00:00Z).
Default time range when omitted: last 7 days.`
