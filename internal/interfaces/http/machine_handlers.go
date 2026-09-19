package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/machine"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/user"
)

func machineFilterForCluster(clusterID shared.ID) machine.Filter {
	return machine.Filter{ClusterID: &clusterID}
}

func mountMachines(r chi.Router, d Deps) {
	r.Route("/machines", func(sub chi.Router) {
		sub.Get("/", func(w http.ResponseWriter, r *http.Request) {
			machines, err := d.MachineService.List(r.Context(), machineFilterFromQuery(r), pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, machines)
		})

		sub.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			m, err := d.MachineService.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, m)
		})

		sub.Get("/{id}/health", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			h, err := d.MachineService.Health(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, h)
		})

		sub.Post("/{id}/reboot", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			scopeKind, scopeID := machineClusterScope(r, d, id)
			u, err := requireRole(r, d, scopeKind, scopeID, user.RoleOperator)
			if err != nil {
				writeError(w, err)
				return
			}
			op, err := d.MachineService.Reboot(r.Context(), id, u.ID, idempotencyKey(r, "reboot-"+id.String()))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, op)
		})

		sub.Post("/{id}/upgrade", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			scopeKind, scopeID := machineClusterScope(r, d, id)
			u, err := requireRole(r, d, scopeKind, scopeID, user.RoleClusterAdmin)
			if err != nil {
				writeError(w, err)
				return
			}
			var req struct {
				Image string `json:"image"`
			}
			if err := decodeJSON(r, &req); err != nil {
				writeError(w, err)
				return
			}
			op, err := d.MachineService.Upgrade(r.Context(), id, u.ID, idempotencyKey(r, "upgrade-"+id.String()), req.Image)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, op)
		})

		// Hard power control (§11, §12, ADR-0007): goes through the
		// machine's infrastructure provider (BMC or hypervisor API), not
		// Talos — distinct from /reboot above, which asks the OS to do it
		// gracefully and does nothing if the OS is unresponsive.
		sub.Post("/{id}/power/on", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			scopeKind, scopeID := machineClusterScope(r, d, id)
			u, err := requireRole(r, d, scopeKind, scopeID, user.RoleOperator)
			if err != nil {
				writeError(w, err)
				return
			}
			op, err := d.MachineService.HardPowerOn(r.Context(), id, u.ID, idempotencyKey(r, "power-on-"+id.String()))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, op)
		})

		sub.Post("/{id}/power/off", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			scopeKind, scopeID := machineClusterScope(r, d, id)
			u, err := requireRole(r, d, scopeKind, scopeID, user.RoleClusterAdmin)
			if err != nil {
				writeError(w, err)
				return
			}
			op, err := d.MachineService.HardPowerOff(r.Context(), id, u.ID, idempotencyKey(r, "power-off-"+id.String()))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, op)
		})

		sub.Post("/{id}/power/cycle", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			scopeKind, scopeID := machineClusterScope(r, d, id)
			u, err := requireRole(r, d, scopeKind, scopeID, user.RoleClusterAdmin)
			if err != nil {
				writeError(w, err)
				return
			}
			op, err := d.MachineService.HardPowerCycle(r.Context(), id, u.ID, idempotencyKey(r, "power-cycle-"+id.String()))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, op)
		})
	})
}

// machineClusterScope resolves the cluster a machine belongs to, so
// destructive machine operations can be authorized at cluster scope (§17,
// §26) instead of requiring a separate per-machine RBAC model. Machines not
// yet assigned to a cluster fall back to PLATFORM scope, since there is no
// narrower resource to bind a role to.
func machineClusterScope(r *http.Request, d Deps, machineID shared.ID) (user.ResourceKind, shared.ID) {
	m, err := d.MachineService.Get(r.Context(), machineID)
	if err != nil || m.ClusterID == nil {
		return user.ResourcePlatform, shared.ID{}
	}
	return user.ResourceCluster, *m.ClusterID
}

func machineFilterFromQuery(r *http.Request) machine.Filter {
	q := r.URL.Query()
	var filter machine.Filter
	if v := q.Get("clusterId"); v != "" {
		if id, err := shared.ParseID(v); err == nil {
			filter.ClusterID = &id
		}
	}
	if v := q.Get("siteId"); v != "" {
		if id, err := shared.ParseID(v); err == nil {
			filter.SiteID = &id
		}
	}
	if v := q.Get("providerId"); v != "" {
		if id, err := shared.ParseID(v); err == nil {
			filter.ProviderID = &id
		}
	}
	filter.Role = machine.Role(q.Get("role"))
	filter.Phase = machine.Phase(q.Get("phase"))
	return filter
}
