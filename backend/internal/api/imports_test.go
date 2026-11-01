package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
)

func TestImports(t *testing.T) {
	e := setup(t)
	root := e.addUser("root", auth.RoleAdmin)
	anna := e.addUser("anna", auth.RoleUser)
	admin, user := e.loggedIn("root"), e.loggedIn("anna")
	if err := os.MkdirAll(filepath.Join(e.importDir, "summer"), 0o750); err != nil {
		t.Fatal(err)
	}
	const imports = "/api/v1/imports"
	body := map[string]any{"ownerId": anna.ID, "folder": "summer"}

	t.Run("only an administrator can use it", func(t *testing.T) {
		wantStatus(t, user.get(imports), http.StatusForbidden)
		wantStatus(t, user.get(imports+"/folders"), http.StatusForbidden)
		wantStatus(t, user.get(imports+"/1"), http.StatusForbidden)
		wantStatus(t, user.json(http.MethodPost, imports, body), http.StatusForbidden)
		wantStatus(t, user.json(http.MethodPost, imports+"/1/stop", nil), http.StatusForbidden)
	})

	t.Run("lists the folders", func(t *testing.T) {
		res := admin.get(imports + "/folders")
		wantStatus(t, res, http.StatusOK)
		if got := decode[[]string](t, res); len(got) != 1 || got[0] != "summer" {
			t.Errorf("folders = %v", got)
		}
	})

	t.Run("rejects a folder outside the root and an unknown owner", func(t *testing.T) {
		wantStatus(t, admin.json(http.MethodPost, imports, map[string]any{"ownerId": anna.ID, "folder": "../.."}), http.StatusBadRequest)
		wantStatus(t, admin.json(http.MethodPost, imports, map[string]any{"ownerId": 9999, "folder": ""}), http.StatusBadRequest)
	})

	res := admin.json(http.MethodPost, imports, body)
	wantStatus(t, res, http.StatusCreated)
	run := decode[importRunJSON](t, res)
	if run.OwnerID != anna.ID || run.Folder != "summer" || run.Status != "listing" {
		t.Errorf("run = %+v", run)
	}
	path := imports + "/" + itoa(run.ID)

	t.Run("one at a time", func(t *testing.T) {
		wantStatus(t, admin.json(http.MethodPost, imports, body), http.StatusConflict)
	})

	t.Run("shows the run", func(t *testing.T) {
		res := admin.get(path)
		wantStatus(t, res, http.StatusOK)
		got := decode[struct {
			Run      importRunJSON       `json:"run"`
			Problems []importProblemJSON `json:"problems"`
		}](t, res)
		if got.Run.ID != run.ID || got.Problems == nil {
			t.Errorf("response = %+v", got)
		}

		res = admin.get(imports)
		wantStatus(t, res, http.StatusOK)
		if list := decode[[]importRunJSON](t, res); len(list) != 1 || list[0].ID != run.ID {
			t.Errorf("runs = %+v", list)
		}
		wantStatus(t, admin.get(imports+"/9999"), http.StatusNotFound)
	})

	t.Run("stops once", func(t *testing.T) {
		wantStatus(t, admin.json(http.MethodPost, path+"/stop", nil), http.StatusNoContent)
		wantStatus(t, admin.json(http.MethodPost, path+"/stop", nil), http.StatusConflict)
	})

	t.Run("a stopped run frees the way for another", func(t *testing.T) {
		wantStatus(t, admin.json(http.MethodPost, imports, map[string]any{"ownerId": root.ID, "folder": ""}), http.StatusCreated)
	})
}
