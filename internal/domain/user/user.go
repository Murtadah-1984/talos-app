// Package user models platform users and role-based access control.
//
// Authorization is resource-scoped: a RoleBinding grants a Role on a specific
// resource (organization, project, environment, or cluster), and access checks
// walk up the tenancy hierarchy (cluster -> environment -> project -> organization)
// looking for the first applicable binding.
package user

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Role is a platform-defined authorization level (§26).
type Role string

const (
	RolePlatformAdmin Role = "PLATFORM_ADMIN"
	RoleOrgAdmin      Role = "ORGANIZATION_ADMIN"
	RoleProjectAdmin  Role = "PROJECT_ADMIN"
	RoleClusterAdmin  Role = "CLUSTER_ADMIN"
	RoleOperator      Role = "OPERATOR"
	RoleViewer        Role = "VIEWER"
)

// weight ranks roles from least to most privileged, for "does role A satisfy
// requirement B" checks within the same scope.
var weight = map[Role]int{
	RoleViewer:        1,
	RoleOperator:      2,
	RoleClusterAdmin:  3,
	RoleProjectAdmin:  4,
	RoleOrgAdmin:      5,
	RolePlatformAdmin: 6,
}

// Satisfies reports whether r grants at least the privilege of required.
func (r Role) Satisfies(required Role) bool {
	return weight[r] >= weight[required]
}

// ResourceKind is the kind of resource a RoleBinding scopes to.
type ResourceKind string

const (
	ResourcePlatform     ResourceKind = "PLATFORM"
	ResourceOrganization ResourceKind = "ORGANIZATION"
	ResourceProject      ResourceKind = "PROJECT"
	ResourceEnvironment  ResourceKind = "ENVIRONMENT"
	ResourceCluster      ResourceKind = "CLUSTER"
)

// User is a platform principal, authenticated via OIDC (or a dev credential
// provider in local development).
type User struct {
	ID       shared.ID
	Email    string
	Name     string
	Subject  string // OIDC "sub" claim
	Disabled bool
	shared.Timestamps
}

// RoleBinding grants a Role on a specific resource to a User.
type RoleBinding struct {
	ID           shared.ID
	UserID       shared.ID
	Role         Role
	ResourceKind ResourceKind
	ResourceID   shared.ID
	shared.Timestamps
}

// Repository persists users and role bindings.
type Repository interface {
	Create(ctx context.Context, u *User) error
	Get(ctx context.Context, id shared.ID) (*User, error)
	GetBySubject(ctx context.Context, subject string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, page shared.Page) ([]*User, error)
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id shared.ID) error

	CreateRoleBinding(ctx context.Context, rb *RoleBinding) error
	ListRoleBindingsForUser(ctx context.Context, userID shared.ID) ([]*RoleBinding, error)
	DeleteRoleBinding(ctx context.Context, id shared.ID) error
}
