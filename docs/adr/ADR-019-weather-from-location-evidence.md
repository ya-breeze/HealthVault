# ADR-019: Weather History Uses Timestamped Approximate Location Evidence

## Status

Proposed

## Context and Problem Statement

The owner wants local weather history for later comparison with health and wellbeing. A stored timezone defines a calendar boundary, not a position. Health Connect uploads can contain old measurements, so the phone's location at upload time cannot locate those measurements. Filling travel periods with weather at a home city would create misleading evidence.

## Decision Drivers

- Preserve when and where the available evidence applies.
- Keep collection voluntary and retain only approximate locations.
- Continue collection independently of widgets and health uploads.
- Retain gaps instead of inventing location history.

## Considered Options

- Use the representative city of the timezone. Rejected because it does not identify the user's location or travel within one timezone.
- Use a configured home city for all dates. Rejected because trips would silently receive home weather.
- Collect precise continuous location. Rejected because coarse hourly evidence is sufficient for this feature and a precise route is unnecessary.
- Collect timestamped approximate locations and enrich only compatible intervals. Chosen.

## Decision Outcome

The native Android client optionally collects approximate location in the background, rounds coordinates before persistence, and sends timestamped observations through the existing authenticated session. Hourly collection is a scheduling target; Android can delay it. The feature does not depend on widget installation. Disabling or changing the signed-in account clears unsent observations and stops collection.

HealthVault stores user-scoped location evidence separately from hourly weather and medical records. It assigns weather only to complete hours bracketed by a continuous chain of nearby observations with acceptable accuracy and bounded separation in time. Uncertain intervals remain explicit gaps. Late evidence can revise coverage and remove weather that no longer qualifies. This is an estimate of regional weather, not a measurement of the user's personal exposure.

Open-Meteo supplies model-based historical weather. Each stored hour retains source, model, units, fetch time and supporting location observations. Instantaneous fields describe the hour start; precipitation is the accumulation over that hour, read from the provider value at the hour end. Provider outages preserve pending enrichment for later retry. Background database writes participate in the existing backup capture barrier.

### Consequences

- Adding location evidence and a weather provider introduces new persisted provenance and an external dependency.
- The app needs separate approximate foreground and background location permissions. Actual background behavior requires phone acceptance.
- Short unobserved round trips remain undetectable. Indoor climate, altitude-specific exposure and causal medical conclusions remain outside the feature.
- Existing health history cannot be enriched without corresponding location evidence.
- Google Play delivery needs its background-location disclosure and review; an APK build does not prove publication approval.
