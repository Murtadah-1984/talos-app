package auth

import (
	"context"
	"fmt"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/environment"
	"github.com/talos-platform/talos-platform/internal/domain/project"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/user"
)

// Authorizer implements resource-scoped RBAC (§26): a RoleBinding grants a
// Role on a specific resource, and a check walks up the tenancy hierarchy
// (cluster -> environment -> project -> organization -> platform) looking
// for the first binding that satisfies the required role.
type Authorizer struct {
	users        user.Repository
	clusters     cluster.Repository
	environments environment.Repository
	projects     project.Repository
}

func NewAuthorizer(users user.Repository, clusters cluster.Repository, environments environment.Repository, projects project.Repository) *Authorizer {
	return &Authorizer{users: users, clusters: clusters, environments: environments, projects: projects}
}

// scopeChain resolves the ordered list of (kind, id) scopes to check for a
// given starting resource, from most-specific to PLATFORM.
func (a *Authorizer) scopeChain(ctx context.Context, kind user.ResourceKind, id shared.ID) ([]struct {
	Kind user.ResourceKind
	ID   shared.ID
}, error) {
	chain := []struct {
		Kind user.ResourceKind
		ID   shared.ID
	}{{Kind: kind, ID: id}}

	switch kind {
	case user.ResourceCluster:
		c, err := a.clusters.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		chain = append(chain, struct {
			Kind user.ResourceKind
			ID   shared.ID
		}{user.ResourceEnvironment, c.EnvironmentID})
		env, err := a.environments.Get(ctx, c.EnvironmentID)
		if err != nil {
			return nil, err
		}
		chain = append(chain, struct {
			Kind user.ResourceKind
			ID   shared.ID
		}{user.ResourceProject, env.ProjectID})
		proj, err := a.projects.Get(ctx, env.ProjectID)
		if err != nil {
			return nil, err
		}
		chain = append(chain, struct {
			Kind user.ResourceKind
			ID   shared.ID
		}{user.ResourceOrganization, proj.OrganizationID})
	case user.ResourceEnvironment:
		env, err := a.environments.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		chain = append(chain, struct {
			Kind user.ResourceKind
			ID   shared.ID
		}{user.ResourceProject, env.ProjectID})
		proj, err := a.projects.Get(ctx, env.ProjectID)
		if err != nil {
			return nil, err
		}
		chain = append(chain, struct {
			Kind user.ResourceKind
			ID   shared.ID
		}{user.ResourceOrganization, proj.OrganizationID})
	case user.ResourceProject:
		proj, err := a.projects.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		chain = append(chain, struct {
			Kind user.ResourceKind
			ID   shared.ID
		}{user.ResourceOrganization, proj.OrganizationID})
	case user.ResourceOrganization, user.ResourcePlatform:
		// nothing further up the chain
	}

	// PLATFORM_ADMIN bindings apply everywhere; always check last.
	chain = append(chain, struct {
		Kind user.ResourceKind
		ID   shared.ID
	}{user.ResourcePlatform, shared.ID{}})
	return chain, nil
}

// Authorize returns nil if userID holds at least `required` on the given
// resource (or an ancestor of it), and shared.ErrForbidden otherwise.
func (a *Authorizer) Authorize(ctx context.Context, userID shared.ID, kind user.ResourceKind, id shared.ID, required user.Role) error {
	bindings, err := a.users.ListRoleBindingsForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading role bindings: %w", err)
	}
	if len(bindings) == 0 {
		return shared.ErrForbidden
	}

	chain, err := a.scopeChain(ctx, kind, id)
	if err != nil {
		return err
	}

	for _, scope := range chain {
		for _, b := range bindings {
			if b.ResourceKind != scope.Kind {
				continue
			}
			if scope.Kind != user.ResourcePlatform && b.ResourceID != scope.ID {
				continue
			}
			if b.Role.Satisfies(required) {
				return nil
			}
		}
	}
	return shared.ErrForbidden
}
