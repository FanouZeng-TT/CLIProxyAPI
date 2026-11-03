# Plugin model catalog refresh

The host can synchronize a plugin's model catalog without reconfiguring or
reloading its native library. This updates model availability and scheduler
metadata; it does not replace plugin binaries or invalidate client session caches.

## Contract

- `model.static` requests advertise `model_catalog_refresh: true` only when the
  service has installed the model runtime refresh handler.
- A model provider may return `catalog_revision`, an opaque identifier for the
  exact snapshot in its response.
- From a background task, the plugin calls `host.models.refresh` with `{}`.
  The callback authenticates the calling plugin identity and active instance.
- The service serializes refreshes against startup and configuration model
  registration, reads static catalogs, registers executors, refreshes credential
  model availability, and rebuilds scheduler entries.
- After completion, the callback returns
  `{"ok":true,"result":{"catalog_revision":"<applied revision>"}}`.
  Read failures, cancellation, and retired instances are not acknowledged.
- Plugins should compare the acknowledgement with their served snapshot, commit
  routing metadata only after confirmation, and retry failed or superseded
  updates. A revision must include model capability changes, not just model IDs.

The callback reads `model.static` again. A plugin must never invoke it inline
from `model.static`, or while holding locks needed by static catalog construction.
Use a single background task to coalesce notifications and perform retries.

Older hosts omit the capability flag. Plugins must retain their original
static registration behavior and explain that automatic synchronization requires
a compatible host. No native ABI or plugin schema version change is necessary:
the new fields and callback are opt-in.

Catalog read failures preserve the previous collected snapshot and executor
availability. In-flight requests retain their already prepared executor targets.
