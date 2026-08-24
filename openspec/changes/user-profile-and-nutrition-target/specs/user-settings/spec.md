## ADDED Requirements

### Requirement: Concurrent-write safety across settings editors

`PUT /api/users/me/settings` replaces the entire settings object with no server-side merge (see
"Replacing settings" above), so round-trip safety against concurrent writes from different frontend
features is entirely the frontend's responsibility. The frontend SHALL serialize every write to the
settings object — from every feature that writes to it — through one shared queue, so that no two
writes issued in the same session can interleave and cause one to silently overwrite the other's
already-saved change with a stale read-modify-write.

This applies regardless of how many independent features write to the blob. As of this
requirement, three do: the dashboard-order editor, the Display Language switcher, and the profile
form (`user-profile`). A feature performing its own independent `GET`-then-`PUT` outside the shared
queue SHALL NOT be considered compliant, even if it happens not to race in ordinary use — the
failure only surfaces when two writes land in the same session with no navigation in between.

#### Scenario: Two writers in the same session both persist

- **WHEN** a user saves a dashboard reorder and then, in the same session with no navigation in
  between, changes their Display Language
- **THEN** both the reordered `dashboard_order` and the new `display_language` SHALL be present on
  the next read — neither write SHALL be silently lost

#### Scenario: Three writers in the same session all persist

- **WHEN** a user saves a dashboard reorder, changes their Display Language, and saves the profile
  form, all in the same session with no navigation in between any of them
- **THEN** all three changes SHALL be present on the next read
