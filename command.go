// Package frontend provides shared frontend behavior for fzt tools:
// command palette mechanics, frontend identity, and action routing.
//
// Every tool that wants to be an "fzt app" imports this package alongside
// the fzt engine (github.com/nelsong6/fzt/core).
package frontend

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nelsong6/fzt/core"
)

// EngineVersion is the fzt engine version this build was compiled against.
// Set via ldflags: -X github.com/nelsong6/fzt-frontend.EngineVersion=v0.2.39
var EngineVersion = "dev"

// cmdAction creates an *ItemAction with type "command" for command palette items.
func cmdAction(target string) *core.ItemAction {
	if target == "" {
		return nil
	}
	return &core.ItemAction{Type: "command", Target: target}
}

// EditCommands returns the canonical `edit` submenu for frontends that edit
// the current tree in place (add-after, add-folder, edit-item, delete,
// save). Include it in your Config.FrontendCommands. Non-editors (e.g.
// fzt-showcase) should omit it.
//
// Action strings are stable identifiers handled by HandleCommandAction and
// tui key handlers. Renaming palette Names is safe; renaming Actions is not.
func EditCommands() core.CommandItem {
	return core.CommandItem{
		Name: "edit", Description: "Edit tree in place",
		Children: []core.CommandItem{
			{Name: "add-after", Description: "Add item after cursor", Action: "add-after"},
			{Name: "add-folder", Description: "Create folder at cursor", Action: "add-folder"},
			{Name: "edit-item", Description: "Edit item properties", Action: "rename"},
			{Name: "delete", Description: "Delete highlighted item", Action: "delete"},
			{Name: "save", Description: "Save changes to cloud", Action: "save"},
		},
	}
}

// InjectCommandFolder appends the `:` command folder and its children to the
// tree's AllItems. When a frontend is registered (FrontendName set), the first
// level holds frontend commands and a nested `:` subfolder holds core commands.
// When no frontend is registered, the first level holds core commands directly.
func buildVersionRegistry(s *core.State, coreVersion string) {
	coreVerStr := coreVersion
	if coreVerStr == "" || coreVerStr == "dev" {
		coreVerStr = "ERROR: use go run ./build"
	}
	s.VersionRegistry = nil
	if s.FrontendName != "" {
		feVer := s.FrontendVersion
		if feVer == "" || feVer == "UNSET" {
			feVer = "ERROR: use go run ./build"
		}
		s.VersionRegistry = append(s.VersionRegistry, s.FrontendName+" "+feVer)
		s.VersionRegistry = append(s.VersionRegistry, "fzt "+coreVerStr)
		identity := s.IdentityLabel
		if identity == "" {
			identity = "(none)"
		}
		s.VersionRegistry = append(s.VersionRegistry, identity)
	} else {
		s.VersionRegistry = append(s.VersionRegistry, "fzt "+coreVerStr)
	}
}

func hasEnvTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func InjectCommandFolder(s *core.State, coreVersion string) {
	ctx := s.TopCtx()

	// Check if palette already exists in loaded data (data-driven mode)
	for _, item := range ctx.AllItems {
		if len(item.Fields) > 0 && item.Fields[0] == ":" && item.Hidden {
			// Palette loaded from cache — skip injection, just build version registry
			buildVersionRegistry(s, coreVersion)
			return
		}
	}

	hasFrontend := s.FrontendName != ""

	coreVerStr := coreVersion
	if coreVerStr == "" || coreVerStr == "dev" {
		coreVerStr = "ERROR: use go run ./build"
	}

	// Build version registry — each entry gets an index that "on" buttons reference
	s.VersionRegistry = nil
	if hasFrontend {
		feLabel := s.FrontendName
		feVer := s.FrontendVersion
		if feVer == "" || feVer == "UNSET" {
			feVer = "ERROR: use go run ./build"
		}
		s.VersionRegistry = append(s.VersionRegistry, feLabel+" "+feVer) // index 0: frontend
		s.VersionRegistry = append(s.VersionRegistry, "fzt "+coreVerStr) // index 1: engine
		identity := s.IdentityLabel
		if identity == "" {
			identity = "(none)"
		}
		s.VersionRegistry = append(s.VersionRegistry, identity) // index 2: identity
	} else {
		s.VersionRegistry = append(s.VersionRegistry, "fzt "+coreVerStr) // index 0: engine
	}

	base := len(ctx.AllItems)
	ctlFolderIdx := base
	var items []core.Item

	if hasFrontend {
		items = buildTwoLevelCommandTree(s, ctlFolderIdx, 0, 1, s.EnvTags) // feIdx=0, coreIdx=1
	} else {
		items = buildCoreLevelCommandTree(s.VersionRegistry, ctlFolderIdx, 0, s.EnvTags) // coreIdx=0
	}

	ctx.AllItems = append(ctx.AllItems, items...)
	ctx.Items = core.RootItemsOf(ctx.AllItems)
}

// buildCoreLevelCommandTree builds `:` → core commands directly (no frontend layer).
// versionIdx is the index into State.VersionRegistry for this level's version string.
func buildCoreLevelCommandTree(registry []string, ctlFolderIdx int, versionIdx int, envTags []string) []core.Item {
	idx := ctlFolderIdx + 1
	var ctlChildren []int
	var items []core.Item

	versionDesc := ""
	if versionIdx >= 0 && versionIdx < len(registry) {
		versionDesc = registry[versionIdx]
	}

	// version — always shown
	versionItemIdx := idx
	ctlChildren = append(ctlChildren, versionItemIdx)
	idx++
	items = append(items, core.Item{
		Fields: []string{"version", versionDesc}, Depth: 1,
		ParentIdx: ctlFolderIdx, Action: cmdAction("version"), PropertyOf: -1,
	})

	// Conditional core commands
	type coreCmd struct {
		fields    []string
		action    string
		condition string
	}
	coreCmds := []coreCmd{
		{[]string{"updatetimer", "Show time to next sync check"}, "updatetimer", ""},
		{[]string{"validate", "Validate credential store"}, "validate", ""},
	}
	for _, cmd := range coreCmds {
		if cmd.condition != "" && !hasEnvTag(envTags, cmd.condition) {
			continue
		}
		ctlChildren = append(ctlChildren, idx)
		idx++
		items = append(items, core.Item{
			Fields: cmd.fields, Depth: 1, ParentIdx: ctlFolderIdx,
			Action: cmdAction(cmd.action), DisplayCondition: cmd.condition, PropertyOf: -1,
		})
	}

	// Prepend the : folder itself
	ctlFolder := core.Item{
		Fields: []string{":"}, Depth: 0, HasChildren: true,
		ParentIdx: -1, Children: ctlChildren, Hidden: true, PropertyOf: -1,
	}
	return append([]core.Item{ctlFolder}, items...)
}

// buildTwoLevelCommandTree builds `:` → frontend commands + `::` → core commands.
// feIdx and coreIdx are indices into State.VersionRegistry.
//
// Index allocation: the function pre-allocates contiguous index ranges for all items
// before building the slice. Starting from ctlFolderIdx+1, it reserves indices for:
// version (1), whoami folder (3), each FrontendCommand + its Children,
// and the core subfolder. Items must be appended in the same order as indices were reserved.
func buildTwoLevelCommandTree(s *core.State, ctlFolderIdx int, feIdx int, coreIdx int, envTags []string) []core.Item {
	idx := ctlFolderIdx + 1
	var ctlChildren []int

	// frontend version — single toggle leaf
	feVersionIdx := idx
	ctlChildren = append(ctlChildren, feVersionIdx)
	idx++

	// whoami folder
	identityFolderIdx := idx
	ctlChildren = append(ctlChildren, identityFolderIdx)
	idx++
	identityOnIdx := idx
	idx++
	identityOffIdx := idx
	idx++

	for _, cmd := range s.FrontendCommands {
		ctlChildren = append(ctlChildren, idx)
		idx++
		idx += len(cmd.Children) // reserve indices for children
	}

	coreSubfolderIdx := idx
	ctlChildren = append(ctlChildren, coreSubfolderIdx)
	idx++

	// core version — always shown
	coreVersionIdx := idx
	coreSubChildren := []int{coreVersionIdx}
	idx++

	// Conditional core commands
	type coreCmd struct {
		fields    []string
		action    string
		condition string
	}
	coreCmds := []coreCmd{
		{[]string{"updatetimer", "Show time to next sync check"}, "updatetimer", ""},
		{[]string{"validate", "Validate credential store"}, "validate", ""},
	}
	for _, cmd := range coreCmds {
		if cmd.condition != "" && !hasEnvTag(envTags, cmd.condition) {
			continue
		}
		coreSubChildren = append(coreSubChildren, idx)
		idx++
	}

	var items []core.Item

	items = append(items, core.Item{
		Fields: []string{":"}, Depth: 0, HasChildren: true,
		ParentIdx: -1, Children: ctlChildren, Hidden: true, PropertyOf: -1,
	})

	// Frontend version toggle
	feVersionDesc := ""
	if feIdx >= 0 && feIdx < len(s.VersionRegistry) {
		feVersionDesc = s.VersionRegistry[feIdx]
	}
	items = append(items, core.Item{
		Fields: []string{"version", feVersionDesc}, Depth: 1,
		ParentIdx: ctlFolderIdx, Action: cmdAction("version"), PropertyOf: -1,
	})

	identityIdxStr := fmt.Sprintf("%d", len(s.VersionRegistry)-1)
	items = append(items, core.Item{
		Fields: []string{"whoami", "Show/hide loaded identity"}, Depth: 1,
		HasChildren: true, ParentIdx: ctlFolderIdx, PropertyOf: -1,
		Children: []int{identityOnIdx, identityOffIdx},
	})
	items = append(items, core.Item{Fields: []string{"on", "Show identity", identityIdxStr}, Depth: 2, ParentIdx: identityFolderIdx, Action: cmdAction("on"), PropertyOf: -1})
	items = append(items, core.Item{Fields: []string{"off", "Hide identity"}, Depth: 2, ParentIdx: identityFolderIdx, Action: cmdAction("off"), PropertyOf: -1})

	for _, cmd := range s.FrontendCommands {
		cmdIdx := ctlFolderIdx + len(items)
		hasChildren := len(cmd.Children) > 0
		cmdItem := core.Item{
			Fields: []string{cmd.Name, cmd.Description}, Depth: 1,
			ParentIdx: ctlFolderIdx, HasChildren: hasChildren,
			Action: cmdAction(cmd.Action), PropertyOf: -1,
		}
		if hasChildren {
			for i := range cmd.Children {
				cmdItem.Children = append(cmdItem.Children, cmdIdx+1+i)
			}
		}
		items = append(items, cmdItem)
		for _, child := range cmd.Children {
			items = append(items, core.Item{
				Fields: []string{child.Name, child.Description}, Depth: 2, ParentIdx: cmdIdx,
				Action: cmdAction(child.Action), PropertyOf: -1,
			})
		}
	}

	items = append(items, core.Item{
		Fields: []string{":", "fzt core"}, Depth: 1,
		HasChildren: true, ParentIdx: ctlFolderIdx, Children: coreSubChildren, PropertyOf: -1,
	})

	// Core version toggle
	coreVersionDesc := ""
	if coreIdx >= 0 && coreIdx < len(s.VersionRegistry) {
		coreVersionDesc = s.VersionRegistry[coreIdx]
	}
	items = append(items, core.Item{
		Fields: []string{"version", coreVersionDesc}, Depth: 2,
		ParentIdx: coreSubfolderIdx, Action: cmdAction("version"), PropertyOf: -1,
	})

	for _, cmd := range coreCmds {
		if cmd.condition != "" && !hasEnvTag(envTags, cmd.condition) {
			continue
		}
		items = append(items, core.Item{
			Fields: cmd.fields, Depth: 2, ParentIdx: coreSubfolderIdx,
			Action: cmdAction(cmd.action), DisplayCondition: cmd.condition, PropertyOf: -1,
		})
	}

	return items
}

// HandleCommandAction processes a selected leaf item in the command tree.
// Returns an action string for the frontend, or "" if handled internally.
func HandleCommandAction(s *core.State, item core.Item) string {
	if len(item.Fields) == 0 {
		return ""
	}
	// Route by stable Action field, fall back to display name
	action := ""
	if item.Action != nil {
		action = item.Action.Target
	}
	if action == "" {
		action = item.Fields[0]
	}

	switch action {
	case "version":
		// Emit the version as a status. Repeats pulse (SetTitle detects repeat).
		// Previously toggled between show/hide — removed: selecting the item
		// should just re-emit the status, not hide it.
		if len(item.Fields) >= 2 && item.Fields[1] != "" {
			s.SetTitle(item.Fields[1], 1)
		}
		return ""
	case "on":
		// "on" items store their VersionRegistry index in Fields[2] (used by whoami).
		if len(item.Fields) >= 3 {
			idx, err := strconv.Atoi(item.Fields[2])
			if err == nil && idx >= 0 && idx < len(s.VersionRegistry) {
				s.VersionDisplay = s.VersionRegistry[idx]
			}
		}
		return ""
	case "off":
		s.VersionDisplay = ""
		return ""
	case "updatetimer":
		if s.SyncTimerShown {
			s.ClearTitle()
		} else {
			s.SyncTimerShown = true
		}
		return ""
	case "toggle-states":
		s.StatesBannerOn = !s.StatesBannerOn
		if s.StatesBannerOn {
			s.SetTitle("states inspector on — actions suppressed", 1)
		} else {
			s.LastActionPreview = ""
			s.SetTitle("states inspector off", 0)
		}
		return ""
	case "validate":
		HandleValidate(s)
		return ""
	case "load-nelson", "load-nelson-ea", "load-nelson-r1":
		identity := strings.TrimPrefix(action, "load-")
		if s.ConfigDir == "" {
			s.SetTitle("no config directory set", 2)
			return ""
		}
		if err := os.WriteFile(filepath.Join(s.ConfigDir, ".identity"), []byte(identity), 0644); err != nil {
			s.SetTitle(fmt.Sprintf("failed to write identity: %v", err), 2)
			return ""
		}
		s.IdentityLabel = identity
		// Auto-sync after loading identity
		secret := s.JWTSecret
		if secret == "" {
			var err error
			secret, err = ReadJWTSecret()
			if err != nil {
				s.SetTitle(err.Error(), 2)
				return ""
			}
			s.JWTSecret = secret
		}
		_, ver, err := SyncMenu(s.ConfigDir, secret)
		if err != nil {
			s.SetTitle(fmt.Sprintf("loaded %s but sync failed: %v", identity, err), 2)
			return ""
		}
		s.MenuVersion = ver
		return "loaded"
	case "unload":
		if s.ConfigDir != "" {
			os.Remove(filepath.Join(s.ConfigDir, ".identity"))
			os.Remove(filepath.Join(s.ConfigDir, "menu-cache.yaml"))
		}
		return "unloaded"
	case "sync":
		if s.ConfigDir == "" {
			s.SetTitle("no config directory set", 2)
			return ""
		}
		secret := s.JWTSecret
		if secret == "" {
			var err error
			secret, err = ReadJWTSecret()
			if err != nil {
				s.SetTitle(err.Error(), 2)
				return ""
			}
			s.JWTSecret = secret
		}
		count, ver, err := SyncMenu(s.ConfigDir, secret)
		if err != nil {
			s.SetTitle(err.Error(), 2)
			return ""
		}
		s.MenuVersion = ver
		s.SetTitle(fmt.Sprintf("synced %d items @ v%d", count, ver), 1)
		return "synced"
	case "add-after":
		s.EditMode = "add-after"
		s.SetTitle("add after: navigate, Shift+Enter to place", 0)
		return ""
	case "add-folder":
		s.EditMode = "add-folder"
		s.SetTitle("add folder: navigate, Shift+Enter to place", 0)
		return ""
	case "rename":
		s.EditMode = "inspect"
		s.SetTitle("edit: navigate to item, Shift+Enter", 0)
		return ""
	case "delete":
		s.EditMode = "delete"
		s.SetTitle("delete: navigate to item, Shift+Enter", 0)
		return ""
	case "inspect":
		s.EditMode = "inspect"
		s.SetTitle("inspect: navigate to item, Shift+Enter", 0)
		return ""
	case "save":
		if s.ConfigDir == "" {
			s.SetTitle("no config directory set", 2)
			return ""
		}
		if !s.Dirty {
			s.SetTitle("no unsaved changes", 0)
			return ""
		}
		secret := s.JWTSecret
		if secret == "" {
			var err error
			secret, err = ReadJWTSecret()
			if err != nil {
				s.SetTitle(err.Error(), 2)
				return ""
			}
			s.JWTSecret = secret
		}
		ctx := s.TopCtx()
		menu := core.SerializeTree(ctx)
		version, err := SaveMenu(s.ConfigDir, secret, menu, s.MenuVersion)
		if err != nil {
			s.SetTitle(err.Error(), 2)
			return ""
		}
		s.Dirty = false
		s.MenuVersion = version
		s.SetTitle(fmt.Sprintf("saved v%d", version), 1)
		return ""
	case "update":
		return "update"
	}

	for _, cmd := range s.FrontendCommands {
		if cmd.Name == action || cmd.Action == action {
			return cmd.Action
		}
		for _, child := range cmd.Children {
			if child.Name == action || child.Action == action {
				return child.Action
			}
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
	if cfg.InitialDisplay != "" {
		s.IdentityLabel = cfg.InitialDisplay
	}
	if cfg.UpdateRepo != "" {
		s.UpdateRepo = cfg.UpdateRepo
	}
	if cfg.UpdateAssetPrefix != "" {
		s.UpdateAssetPrefix = cfg.UpdateAssetPrefix
	}
	if cfg.UpdateBinaryName != "" {
		s.UpdateBinaryName = cfg.UpdateBinaryName
	}
}
