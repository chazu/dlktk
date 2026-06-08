package store

import "github.com/chazu/pudl/pkg/factstore"

// ResolveDir picks the pudl store directory: an explicit flag wins; otherwise the
// repo-scoped .pudl/ (walking up from cwd) if present; otherwise the global
// ~/.pudl. dlktk shares this store with pudl under the dlktk/* namespace.
func ResolveDir(flag, cwd string) string {
	if flag != "" {
		return flag
	}
	if ws, err := factstore.DiscoverWorkspace(cwd); err == nil && ws != nil {
		if ws.RepoDir != "" {
			return ws.RepoDir
		}
		if ws.GlobalDir != "" {
			return ws.GlobalDir
		}
	}
	return factstore.GlobalDir()
}
