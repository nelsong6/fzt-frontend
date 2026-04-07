// Package frontend provides shared frontend behavior for fzt tools:
// command palette mechanics, frontend identity, and action routing.
//
// Every tool that wants to be an "fzt app" imports this package alongside
// the fzt engine (github.com/nelsong6/fzt/core).
package frontend

import (
	"github.com/nelsong6/fzt/core"
)

// InjectCommandFolder appends the `:` command folder and its children to the
// tree's AllItems. When a frontend is registered (FrontendName set), the first
// level holds frontend commands and a nested `:` subfolder holds core commands.
// When no frontend is registered, the first level holds core commands directly.
func InjectCommandFolder(s *core.State, coreVersion string) {
	ctx := s.TopCtx()
	hasFrontend := s.FrontendName != ""

	coreVerStr := coreVersion
	if coreVerStr == "" {
		coreVerStr = "dev"
	}

	base := len(ctx.AllItems)
	ctlFolderIdx := base
	var items []core.Item

	if hasFrontend {
		items = buildTwoLevelCommandTree(s, ctlFolderIdx, coreVerStr)
	} else {
		items = buildCoreLevelCommandTree(ctlFolderIdx, coreVerStr)
	}

	ctx.AllItems = append(ctx.AllItems, items...)
	ctx.Items = core.RootItemsOf(ctx.AllItems)
}

// buildCoreLevelCommandTree builds `:` → core commands directly (no frontend layer).
func buildCoreLevelCommandTree(ctlFolderIdx int, coreVersion string) []core.Item {
	idx := ctlFolderIdx + 1

	versionFolderIdx := idx
	idx++
	versionOnIdx := idx
	idx++
	versionOffIdx := idx
	idx++
	updateIdx := idx

	ctlChildren := []int{versionFolderIdx, updateIdx}

	return []core.Item{
		{
			Fields: []string{":", "fzt " + coreVersion}, Depth: 0, HasChildren: true,
			ParentIdx: -1, Children: ctlChildren, Hidden: true,
		},
		{
			Fields: []string{"version", "Show/hide version in title bar"}, Depth: 1,
			HasChildren: true, ParentIdx: ctlFolderIdx,
			Children: []int{versionOnIdx, versionOffIdx},
		},
		{Fields: []string{"on", "Show version"}, Depth: 2, ParentIdx: versionFolderIdx},
		{Fields: []string{"off", "Hide version"}, Depth: 2, ParentIdx: versionFolderIdx},
		{Fields: []string{"update", "Update fzt to latest release"}, Depth: 1, ParentIdx: ctlFolderIdx},
	}
}

// buildTwoLevelCommandTree builds `:` → frontend commands + `::` → core commands.
func buildTwoLevelCommandTree(s *core.State, ctlFolderIdx int, coreVersion string) []core.Item {
	idx := ctlFolderIdx + 1
	var ctlChildren []int

	frontendName := s.FrontendName
	frontendVer := s.FrontendVersion
	if frontendVer == "" {
		frontendVer = "unknown"
	}

	feVersionFolderIdx := idx
	ctlChildren = append(ctlChildren, feVersionFolderIdx)
	idx++
	feVersionOnIdx := idx
	idx++
	feVersionOffIdx := idx
	idx++

	for range s.FrontendCommands {
		ctlChildren = append(ctlChildren, idx)
		idx++
	}

	coreSubfolderIdx := idx
	ctlChildren = append(ctlChildren, coreSubfolderIdx)
	idx++

	coreVersionFolderIdx := idx
	coreSubChildren := []int{coreVersionFolderIdx}
	idx++
	coreVersionOnIdx := idx
	idx++
	coreVersionOffIdx := idx
	idx++

	coreUpdateIdx := idx
	coreSubChildren = append(coreSubChildren, coreUpdateIdx)

	var items []core.Item

	items = append(items, core.Item{
		Fields: []string{":"}, Depth: 0, HasChildren: true,
		ParentIdx: -1, Children: ctlChildren, Hidden: true,
	})

	items = append(items, core.Item{
		Fields: []string{"version", frontendName + " " + frontendVer}, Depth: 1,
		HasChildren: true, ParentIdx: ctlFolderIdx,
		Children: []int{feVersionOnIdx, feVersionOffIdx},
	})
	items = append(items, core.Item{Fields: []string{"on", "Show version"}, Depth: 2, ParentIdx: feVersionFolderIdx})
	items = append(items, core.Item{Fields: []string{"off", "Hide version"}, Depth: 2, ParentIdx: feVersionFolderIdx})

	for _, cmd := range s.FrontendCommands {
		items = append(items, core.Item{
			Fields: []string{cmd.Name, cmd.Description}, Depth: 1, ParentIdx: ctlFolderIdx,
		})
	}

	items = append(items, core.Item{
		Fields: []string{":", "fzt " + coreVersion}, Depth: 1,
		HasChildren: true, ParentIdx: ctlFolderIdx, Children: coreSubChildren,
	})

	items = append(items, core.Item{
		Fields: []string{"version", "Show/hide version in title bar"}, Depth: 2,
		HasChildren: true, ParentIdx: coreSubfolderIdx,
		Children: []int{coreVersionOnIdx, coreVersionOffIdx},
	})
	items = append(items, core.Item{Fields: []string{"on", "Show version"}, Depth: 3, ParentIdx: coreVersionFolderIdx})
	items = append(items, core.Item{Fields: []string{"off", "Hide version"}, Depth: 3, ParentIdx: coreVersionFolderIdx})

	items = append(items, core.Item{Fields: []string{"update", "Update fzt to latest release"}, Depth: 2, ParentIdx: coreSubfolderIdx})

	return items
}

// HandleCommandAction processes a selected leaf item in the command tree.
// Returns an action string for the frontend, or "" if handled internally.
func HandleCommandAction(s *core.State, item core.Item) string {
	if len(item.Fields) == 0 {
		return ""
	}
	name := item.Fields[0]

	switch name {
	case "on":
		s.ShowVersion = true
		return ""
	case "off":
		s.ShowVersion = false
		return ""
	case "update":
		return "update"
	}

	for _, cmd := range s.FrontendCommands {
		if cmd.Name == name {
			return cmd.Action
		}
	}

	return ""
}

// IsInCommandScope returns true if the current scope is inside a `:` folder.
func IsInCommandScope(s *core.State) bool {
	ctx := s.TopCtx()
	for _, level := range ctx.Scope[1:] {
		if level.ParentIdx >= 0 && level.ParentIdx < len(ctx.AllItems) {
			if len(ctx.AllItems[level.ParentIdx].Fields) > 0 && ctx.AllItems[level.ParentIdx].Fields[0] == ":" {
				return true
			}
		}
	}
	return false
}

// ScopeCtlTitle returns the appropriate title for the current scope.
// Returns "" if not inside a `:` command folder, "fzt ctl" if inside `::`,
// or "<frontendName> ctl" if inside the first `:`.
func ScopeCtlTitle(s *core.State) string {
	ctx := s.TopCtx()
	colonDepth := 0
	for _, level := range ctx.Scope[1:] {
		if level.ParentIdx >= 0 && level.ParentIdx < len(ctx.AllItems) {
			if len(ctx.AllItems[level.ParentIdx].Fields) > 0 && ctx.AllItems[level.ParentIdx].Fields[0] == ":" {
				colonDepth++
			}
		}
	}
	if colonDepth == 0 {
		return ""
	}
	if colonDepth >= 2 {
		return "fzt ctl"
	}
	name := s.FrontendName
	if name == "" {
		name = "fzt"
	}
	return name + " ctl"
}

// ApplyConfig sets frontend identity and commands from Config onto State.
// Call before InjectCommandFolder.
func ApplyConfig(s *core.State, cfg core.Config) {
	if cfg.FrontendName != "" {
		s.FrontendName = cfg.FrontendName
	}
	if cfg.FrontendVersion != "" {
		s.FrontendVersion = cfg.FrontendVersion
	}
	if len(cfg.FrontendCommands) > 0 {
		s.FrontendCommands = cfg.FrontendCommands
	}
}
