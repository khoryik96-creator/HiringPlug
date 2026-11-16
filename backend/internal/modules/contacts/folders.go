package contacts

import "github.com/khoryik96-creator/hiringplug/backend/internal/platform/folderfilter"

// UseFolders sets where a folder's records come from (internal/app): a list
// in a folder shows only those.
func (m *Module) UseFolders(f folderfilter.Items) { m.service.folderItems = f }
