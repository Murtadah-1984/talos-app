// Package infraprovider models registered infrastructure providers and their
// encrypted credentials (ADR-0007, ADR-0006).
package infraprovider

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Type identifies which InfrastructureProvider implementation to use.
type Type string

const (
	TypeBareMetal    Type = "BARE_METAL"
	TypeProxmox      Type = "PROXMOX"
	TypeVMware       Type = "VMWARE"
	TypeOpenStack    Type = "OPENSTACK"
	TypeAWS          Type = "AWS"
	TypeAzure        Type = "AZURE"
	TypeGCP          Type = "GCP"
	TypeEquinixMetal Type = "EQUINIX_METAL"
)

// InfrastructureProvider is a registered provider instance (e.g. "our Proxmox
// cluster in Basra") belonging to an organization.
type InfrastructureProvider struct {
	ID             shared.ID
	OrganizationID shared.ID
	Name           string
	Type           Type
	Endpoint       string
	// CredentialRef points at the encrypted credential record; the plaintext
	// value never lives on this struct.
	CredentialRef shared.ID
	shared.Timestamps
}

// Credential is an encrypted credential blob associated with a provider (or a
// Git provider). The Ciphertext is opaque to everything except the configured
// SecretStore backend (ADR-0006).
type Credential struct {
	ID         shared.ID
	OwnerID    shared.ID // the provider/git-provider this credential belongs to
	Ciphertext []byte
	KeyRef     string // identifies which SecretStore key/backend encrypted this
	shared.Timestamps
}

// Repository persists infrastructure providers and credentials.
type Repository interface {
	Create(ctx context.Context, p *InfrastructureProvider) error
	Get(ctx context.Context, id shared.ID) (*InfrastructureProvider, error)
	ListByOrganization(ctx context.Context, orgID shared.ID, page shared.Page) ([]*InfrastructureProvider, error)
	Update(ctx context.Context, p *InfrastructureProvider) error
	Delete(ctx context.Context, id shared.ID) error

	PutCredential(ctx context.Context, c *Credential) error
	GetCredential(ctx context.Context, id shared.ID) (*Credential, error)
	DeleteCredential(ctx context.Context, id shared.ID) error
}
