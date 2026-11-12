package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"vocab-backend/internal/billing"
	"vocab-backend/internal/model"
)

type editableTestUsers struct {
	*fakeUserStore
	limit *int64
}

// FindByID 为新管理接口提供可编辑账号快照。
func (f *editableTestUsers) FindByID(_ context.Context, id int) (model.UserWithStats, error) {
	for _, u := range f.usersByName {
		if u.ID == id {
			return model.UserWithStats{User: u}, nil
		}
	}
	return model.UserWithStats{}, sql.ErrNoRows
}

// UpdateProfile 模拟原子保存，测试 handler 是否正确传递全量字段和金额。
func (f *editableTestUsers) UpdateProfile(_ context.Context, id int, name string, admin, disabled bool, limit *int64) error {
	for old, u := range f.usersByName {
		if u.ID == id {
			delete(f.usersByName, old)
			u.Username = name
			u.IsAdmin = admin
			u.Disabled = disabled
			f.usersByName[name] = u
			f.limit = limit
			return nil
		}
	}
	return sql.ErrNoRows
}

type adminBudgetStore struct{ users *editableTestUsers }

// Snapshot 返回测试账号保存后的实时限额。
func (s adminBudgetStore) Snapshot(_ context.Context, _ int, date string) (billing.Snapshot, error) {
	return billing.Snapshot{Date: date, Limit: s.users.limit, Spent: 123456, Reserved: 789}, nil
}

// Reserve 本测试只访问管理接口，任何供应商调用都视为失败。
func (s adminBudgetStore) Reserve(context.Context, billing.Reservation) error {
	return billing.ErrUnavailable
}

// Settle 本测试不产生实际供应商调用。
func (s adminBudgetStore) Settle(context.Context, string, int64, string, []byte) error {
	return billing.ErrUnavailable
}

// WordOwner 本测试不恢复查词任务。
func (s adminBudgetStore) WordOwner(context.Context, int) (int, error) { return 0, sql.ErrNoRows }

// adminFixture 创建管理员及普通用户，所有接口请求仍走真实鉴权路由。
func adminFixture() (*App, *editableTestUsers, *fakeSessionStore) {
	a, u, s := newTestApp()
	admin := u.addUser("admin", "", true)
	learner := u.addUser("learner", "", false)
	n := billing.DefaultLimit
	editor := &editableTestUsers{u, &n}
	a.users = editor
	a.billing = billing.New(adminBudgetStore{editor})
	s.usersByID[admin.ID] = admin
	s.usersByID[learner.ID] = learner
	s.byToken["admin"] = sessionRecord{admin.ID, time.Now().Add(time.Hour)}
	s.byToken["learner"] = sessionRecord{learner.ID, time.Now().Add(time.Hour)}
	return a, editor, s
}

// TestAdminUserEndpoints 普通用户无法读写他人详情，管理员编辑金额和账号即时返回。
func TestAdminUserEndpoints(t *testing.T) {
	a, u, _ := adminFixture()
	handler := a.Handler(t.TempDir())
	cases := []struct {
		method, path, token, body string
		status                    int
	}{
		{"GET", "/api/admin/users/2", "learner", "", 403},
		{"PUT", "/api/admin/users/2", "learner", `{"username":"changed","is_admin":false,"disabled":false,"daily_limit_cny":"8.00"}`, 403},
		{"GET", "/api/admin/users/2", "admin", "", 200},
		{"PUT", "/api/admin/users/1", "admin", `{"username":"admin","is_admin":false,"disabled":false,"daily_limit_cny":null}`, 400},
		{"PUT", "/api/admin/users/1", "admin", `{"username":"admin","is_admin":true,"disabled":true,"daily_limit_cny":null}`, 400},
		{"PUT", "/api/admin/users/2", "admin", `{"username":"changed","is_admin":false,"disabled":false,"daily_limit_cny":"0.001"}`, 400},
		{"PUT", "/api/admin/users/2", "admin", `{"username":"changed","is_admin":false,"disabled":false}`, 400},
		{"PUT", "/api/admin/users/2", "admin", `{"username":"changed","is_admin":false,"disabled":false,"daily_limit_cny":"8.00"}`, 200},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, res.Code, res.Body.String())
		}
	}
	if u.limit == nil || *u.limit != 8*billing.NanoPerYuan {
		t.Fatal("limit not saved", u.limit)
	}
	var out adminUserDetail
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/2", nil)
	req.Header.Set("Authorization", "Bearer admin")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	var result struct {
		User   model.UserWithStats `json:"user"`
		Budget struct {
			Spent string `json:"spent_cny"`
		} `json:"budget"`
	}
	if json.Unmarshal(res.Body.Bytes(), &result) != nil || result.User.Username != "changed" || result.Budget.Spent != "0.000123456" {
		t.Fatal(res.Body.String(), out)
	}
}
