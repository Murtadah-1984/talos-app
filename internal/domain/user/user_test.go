package user_test

import (
	"testing"

	"github.com/talos-platform/talos-platform/internal/domain/user"
)

func TestRole_Satisfies(t *testing.T) {
	tests := []struct {
		held, required user.Role
		want           bool
	}{
		{user.RolePlatformAdmin, user.RoleViewer, true},
		{user.RoleViewer, user.RolePlatformAdmin, false},
		{user.RoleClusterAdmin, user.RoleOperator, true},
		{user.RoleOperator, user.RoleClusterAdmin, false},
		{user.RoleOperator, user.RoleOperator, true},
	}
	for _, tt := range tests {
		if got := tt.held.Satisfies(tt.required); got != tt.want {
			t.Errorf("Role(%s).Satisfies(%s) = %v, want %v", tt.held, tt.required, got, tt.want)
		}
	}
}
