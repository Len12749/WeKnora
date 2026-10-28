package handler

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 工单写操作只对管理员开放：路由层的 Admin guard 在 EnableRBAC=false 时只记日志，
// 所以 handler 自己必须再判一次，这里把这层判断钉住。
func TestTicketHandlerIsAdmin(t *testing.T) {
	h := &TicketHandler{}

	cases := []struct {
		name string
		role types.TenantRole
		want bool
	}{
		{"viewer 只能看", types.TenantRoleViewer, false},
		{"contributor 只能看", types.TenantRoleContributor, false},
		{"admin 可操作", types.TenantRoleAdmin, true},
		{"owner 可操作", types.TenantRoleOwner, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, c.role)
			if got := h.isAdmin(ctx); got != c.want {
				t.Fatalf("role=%s isAdmin=%v, want %v", c.role, got, c.want)
			}
		})
	}
}
