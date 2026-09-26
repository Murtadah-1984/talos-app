-- Lets one platform process manage more than one Talos cluster's PKI: each
-- cluster can point at its own talosconfig in the SecretStore. Empty means
-- "use the process's single default talosconfig" (PLATFORM_TALOS_CONFIG_FILE),
-- the original single-cluster deployment shape (§ Phase 8, multi-cluster
-- credential handling).
ALTER TABLE clusters ADD COLUMN talos_config_ref TEXT NOT NULL DEFAULT '';
