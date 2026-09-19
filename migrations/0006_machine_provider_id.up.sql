-- A machine's platform-internal ID (UUID) is never the same value an
-- infrastructure provider uses to identify it (e.g. a Proxmox VMID, or a
-- bare-metal inventory key). Infrastructure-provider actions (power
-- control, deletion) need the provider's own identifier, which was
-- previously not persisted anywhere (§10, ADR-0007, Phase 6).
ALTER TABLE machines ADD COLUMN provider_machine_id TEXT NOT NULL DEFAULT '';
