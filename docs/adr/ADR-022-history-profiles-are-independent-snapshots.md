# ADR-022: Reuse recent nutrition as independent snapshots with explicit acceptance

## Status
Accepted

A manual correction currently belongs to one Food Item. Reusing its original model estimate
would lose that correction. Binding repeated food to a mutable Custom Food would also let later
catalog changes alter the meaning of a weight edit. For recent meal reuse, copy the source's
current nutrient totals divided by its positive weight into an independent History Profile.
Record the source Food Meal and Food Item and preserve the original source and confidence.
Scale subsequent portion changes from that snapshot.

Photo recognition receives a bounded, owner-scoped recent history as evidence and may suggest a
match. The server validates the returned mapping against its own source snapshot. Apply it only
after explicit owner acceptance, with target and source freshness checks. A direct Repeat Meal
action also creates an unconfirmed draft. Neither action establishes measured nutrition or
authorizes logging intake automatically.

Keep these profiles separate from Custom Foods. The owner does not need to create a permanent
catalog entry for a batch of food eaten over a few days. Source deletion or editing never changes
an accepted copy. A direct macro correction or reference rebind replaces the copied profile and
clears its provenance.
