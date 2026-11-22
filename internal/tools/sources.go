package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/arturonavax/anx/internal/domain"
	"github.com/arturonavax/anx/internal/gittransport"
	"gopkg.in/yaml.v3"
)

// ToolSourceType identifies one of the three first-class ANX tool sources.
type ToolSourceType string

const (
	SourceTypeBuiltin    ToolSourceType = "builtin"
	SourceTypeSingle     ToolSourceType = "single"
	SourceTypeCollection ToolSourceType = "collection"
)

// ToolSource declares an external or builtin repository providing one or more tools.
type ToolSource struct {
	ID       string         `json:"id" yaml:"id"`
	Type     ToolSourceType `json:"type" yaml:"type"`
	URL      string         `json:"url,omitempty" yaml:"url,omitempty"`
	Path     string         `json:"path,omitempty" yaml:"path,omitempty"`
	Ref      string         `json:"ref,omitempty" yaml:"ref,omitempty"`
	Auth     string         `json:"auth,omitempty" yaml:"auth,omitempty"`
	SSHKey   string         `json:"ssh_key,omitempty" yaml:"ssh_key,omitempty"`
	Optional bool           `json:"optional,omitempty" yaml:"optional,omitempty"`
	Priority int            `json:"priority,omitempty" yaml:"priority,omitempty"`
	Enabled  *bool          `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

// IsEnabled returns true unless explicitly disabled.
func (s ToolSource) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// SourcesFile represents the versioned schema for ~/.config/anx/tool_sources.yaml.
type SourcesFile struct {
	SchemaVersion int          `json:"schema_version" yaml:"schema_version"`
	ToolSources   []ToolSource `json:"tool_sources" yaml:"tool_sources"`
}

// SingleToolManifest is the schema for a standalone `tool.yaml` in single or collection repos.
type SingleToolManifest struct {
	SchemaVersion   int      `json:"schema_version,omitempty" yaml:"schema_version,omitempty"`
	ID              string   `json:"id" yaml:"id"`
	Name            string   `json:"name" yaml:"name"`
	Category        string   `json:"category,omitempty" yaml:"category,omitempty"`
	Description     string   `json:"description,omitempty" yaml:"description,omitempty"`
	Version         string   `json:"version,omitempty" yaml:"version,omitempty"`
	Executable      string   `json:"executable" yaml:"executable"`
	Entrypoint      string   `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty"`
	BuildCmd        []string `json:"build_cmd,omitempty" yaml:"build_cmd,omitempty"`
	Language        string   `json:"language,omitempty" yaml:"language,omitempty"`
	VersionArgs     []string `json:"version_args,omitempty" yaml:"version_args,omitempty"`
	AllowGlobalPATH bool     `json:"allow_global_path,omitempty" yaml:"allow_global_path,omitempty"`
	FallbackBuiltin bool     `json:"fallback_builtin,omitempty" yaml:"fallback_builtin,omitempty"`
	RepoURL         string   `json:"repo_url,omitempty" yaml:"repo_url,omitempty"`
	UpdateStrategy  string   `json:"update_strategy,omitempty" yaml:"update_strategy,omitempty"`
	UpdateCmd       []string `json:"update_cmd,omitempty" yaml:"update_cmd,omitempty"`
}

// ResolvedTool pairs a discovered tool manifest with its source provenance.
type ResolvedTool struct {
	SourceID   string
	SourceType ToolSourceType
	SourceDir  string
	Entrypoint string
	BuildCmd   []string
	Manifest   domain.ToolManifest
}

// ToolCollision records when two sources declare the same tool ID.
type ToolCollision struct {
	ToolID        string `json:"tool_id"`
	WinnerSource  string `json:"winner_source"`
	ShadowedByIDs []string `json:"shadowed_sources"`
}

// SourceManager manages persistent multi-source tool repositories and atomic installations.
type SourceManager struct {
	ConfigFile string
	CacheDir   string
	BinDir     string
	BundledDir string
	Git        *gittransport.Adapter
}

// NewSourceManager constructs a SourceManager anchored in the user's ANX directories.
func NewSourceManager(configDir, dataDir, binDir, bundledDir string) *SourceManager {
	return &SourceManager{
		ConfigFile: filepath.Join(configDir, "tool_sources.yaml"),
		CacheDir:   filepath.Join(dataDir, "tool-sources"),
		BinDir:     binDir,
		BundledDir: bundledDir,
		Git:        gittransport.NewAdapter(),
	}
}

// LoadSources reads tool_sources.yaml, ensuring the builtin source is present.
func (m *SourceManager) LoadSources() ([]ToolSource, error) {
	var doc SourcesFile
	data, err := os.ReadFile(m.ConfigFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("reading %s: %w", m.ConfigFile, err)
		}
		return []ToolSource{{ID: "bundled", Type: SourceTypeBuiltin}}, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", m.ConfigFile, err)
	}
	if doc.SchemaVersion != 0 && doc.SchemaVersion != 1 {
		return nil, fmt.Errorf("%s: unsupported schema_version %d (expected 1)", m.ConfigFile, doc.SchemaVersion)
	}
	for _, s := range doc.ToolSources {
		if err := ValidateSource(s); err != nil {
			return nil, fmt.Errorf("%s: %w", m.ConfigFile, err)
		}
	}
	hasBuiltin := false
	for _, s := range doc.ToolSources {
		if s.Type == SourceTypeBuiltin {
			hasBuiltin = true
			break
		}
	}
	if !hasBuiltin {
		doc.ToolSources = append([]ToolSource{{ID: "bundled", Type: SourceTypeBuiltin}}, doc.ToolSources...)
	}
	return doc.ToolSources, nil
}

// SaveSources atomically persists tool_sources.yaml.
func (m *SourceManager) SaveSources(sources []ToolSource) error {
	for _, s := range sources {
		if err := ValidateSource(s); err != nil {
			return err
		}
	}
	doc := SourcesFile{
		SchemaVersion: 1,
		ToolSources:   sources,
	}
	raw, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.ConfigFile), 0o755); err != nil {
		return err
	}
	tmp := m.ConfigFile + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.ConfigFile)
}

// ValidateSource validates a ToolSource declaration.
func ValidateSource(s ToolSource) error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("%w: tool source id is required", ErrInvalidManifest)
	}
	if strings.ContainsAny(s.ID, "/\\ \t\n") {
		return fmt.Errorf("%w: invalid tool source id %q", ErrInvalidManifest, s.ID)
	}
	switch s.Type {
	case SourceTypeBuiltin:
		return nil
	case SourceTypeSingle, SourceTypeCollection:
		if strings.TrimSpace(s.URL) == "" && strings.TrimSpace(s.Path) == "" {
			return fmt.Errorf("%w: tool source %q requires url or path", ErrInvalidManifest, s.ID)
		}
		if s.Auth != "" {
			if _, err := gittransport.ParseAuthMode(s.Auth); err != nil {
				return fmt.Errorf("%w: tool source %q: %v", ErrInvalidManifest, s.ID, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: tool source %q has unsupported type %q", ErrInvalidManifest, s.ID, s.Type)
	}
}

// AddSource registers a new single or collection tool source (local or Git),
// clones remote sources via gittransport, validates its manifest(s), and saves it.
func (m *SourceManager) AddSource(ctx context.Context, src ToolSource) (ToolSource, []ResolvedTool, error) {
	if src.ID == "" {
		src.ID = deriveSourceID(src.URL, src.Path)
	}
	if src.Auth == "" {
		src.Auth = string(gittransport.AuthAuto)
	}

	dir, err := m.ensureSourceDir(ctx, src, true)
	if err != nil {
		return src, nil, err
	}

	// Auto-detect source type if empty
	if src.Type == "" {
		src.Type = detectSourceType(dir)
		if src.Type == "" {
			return src, nil, fmt.Errorf("%w: %s has neither tool.yaml (single) nor tools.yaml/tools/ (collection)", ErrInvalidManifest, dir)
		}
	}

	if err := ValidateSource(src); err != nil {
		return src, nil, err
	}

	discovered, err := m.discoverFromDir(src, dir)
	if err != nil {
		return src, nil, err
	}
	if len(discovered) == 0 {
		return src, nil, fmt.Errorf("%w: source %q at %s declared no tools", ErrInvalidManifest, src.ID, dir)
	}

	sources, err := m.LoadSources()
	if err != nil {
		return src, nil, err
	}
	replaced := false
	for i := range sources {
		if sources[i].ID == src.ID {
			sources[i] = src
			replaced = true
			break
		}
	}
	if !replaced {
		sources = append(sources, src)
	}
	if err := m.SaveSources(sources); err != nil {
		return src, nil, err
	}
	return src, discovered, nil
}

// RemoveSource removes a registered source by ID.
func (m *SourceManager) RemoveSource(id string) error {
	if id == "bundled" {
		return errors.New("cannot remove builtin bundled source")
	}
	sources, err := m.LoadSources()
	if err != nil {
		return err
	}
	filtered := make([]ToolSource, 0, len(sources))
	found := false
	for _, s := range sources {
		if s.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, s)
	}
	if !found {
		return fmt.Errorf("tool source %q not found", id)
	}
	return m.SaveSources(filtered)
}

// UpdateSource fetches updates for a Git-backed source and re-validates its manifests.
func (m *SourceManager) UpdateSource(ctx context.Context, id string) ([]ResolvedTool, error) {
	sources, err := m.LoadSources()
	if err != nil {
		return nil, err
	}
	for _, s := range sources {
		if s.ID != id {
			continue
		}
		if s.Type == SourceTypeBuiltin {
			return m.discoverFromDir(s, m.BundledDir)
		}
		dir, err := m.ensureSourceDir(ctx, s, false)
		if err != nil {
			return nil, err
		}
		if s.URL != "" && !gittransport.NormalizeURL(s.URL).IsLocal {
			if err := m.git().UpdateWithOptions(ctx, gittransport.UpdateOptions{
				Dir:    dir,
				Ref:    s.Ref,
				SSHKey: s.SSHKey,
			}); err != nil {
				return nil, err
			}
		}
		return m.discoverFromDir(s, dir)
	}
	return nil, fmt.Errorf("tool source %q not found", id)
}

// VerifySSHAccess verifies that a remote SSH repository can be queried/cloned with the given SSH key.
func (m *SourceManager) VerifySSHAccess(ctx context.Context, rawURL, sshKey string) (gittransport.ProbeResult, error) {
	return m.git().VerifySSHAccess(ctx, rawURL, sshKey)
}

// VerifySource validates a source's accessibility and manifest schemas without mutating binaries.
func (m *SourceManager) VerifySource(ctx context.Context, id string) ([]ResolvedTool, error) {
	sources, err := m.LoadSources()
	if err != nil {
		return nil, err
	}
	for _, s := range sources {
		if s.ID != id {
			continue
		}
		if s.Type == SourceTypeBuiltin {
			return m.discoverFromDir(s, m.BundledDir)
		}
		dir := m.sourceLocalDir(s)
		if _, err := os.Stat(dir); err != nil {
			if s.URL != "" {
				mode, _ := gittransport.ParseAuthMode(s.Auth)
				if _, pErr := m.git().ProbeWithOptions(ctx, s.URL, gittransport.ProbeOptions{
					Auth:   mode,
					Ref:    s.Ref,
					SSHKey: s.SSHKey,
				}); pErr != nil {
					return nil, pErr
				}
			}
			return nil, fmt.Errorf("source %q is not cached locally at %s; run 'anx tools source update %s'", id, dir, id)
		}
		return m.discoverFromDir(s, dir)
	}
	return nil, fmt.Errorf("tool source %q not found", id)
}

// DiscoverAll loads tools across all enabled sources with deterministic collision resolution.
// Returns:
//   - byID: winning tool per plain ID AND namespaced "<source>/<id>"
//   - collisions: every tool ID declared by multiple sources
func (m *SourceManager) DiscoverAll() (map[string]ResolvedTool, []ToolCollision, error) {
	sources, err := m.LoadSources()
	if err != nil {
		return nil, nil, err
	}

	byID := make(map[string]ResolvedTool)
	shadowed := make(map[string][]string)

	for _, s := range sources {
		if !s.IsEnabled() {
			continue
		}
		var dir string
		if s.Type == SourceTypeBuiltin {
			dir = m.BundledDir
		} else {
			dir = m.sourceLocalDir(s)
		}
		if dir == "" {
			continue
		}
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		found, err := m.discoverFromDir(s, dir)
		if err != nil {
			return nil, nil, fmt.Errorf("source %q: %w", s.ID, err)
		}
		for _, rt := range found {
			namespaced := rt.SourceID + "/" + rt.Manifest.ID
			byID[namespaced] = rt

			if existing, exists := byID[rt.Manifest.ID]; exists {
				// Later non-builtin sources override builtin; otherwise first-declared wins deterministically.
				if existing.SourceType == SourceTypeBuiltin && rt.SourceType != SourceTypeBuiltin {
					shadowed[rt.Manifest.ID] = append(shadowed[rt.Manifest.ID], existing.SourceID)
					byID[rt.Manifest.ID] = rt
				} else {
					shadowed[rt.Manifest.ID] = append(shadowed[rt.Manifest.ID], rt.SourceID)
				}
			} else {
				byID[rt.Manifest.ID] = rt
			}
		}
	}

	var collisions []ToolCollision
	for id, sh := range shadowed {
		collisions = append(collisions, ToolCollision{
			ToolID:        id,
			WinnerSource:  byID[id].SourceID,
			ShadowedByIDs: sh,
		})
	}
	slices.SortFunc(collisions, func(a, b ToolCollision) int {
		return strings.Compare(a.ToolID, b.ToolID)
	})
	return byID, collisions, nil
}

// InstallTool atomically installs or updates a discovered tool into BinDir.
// If build/copy/verification fails, any previously installed binary is preserved untouched.
func (m *SourceManager) InstallTool(ctx context.Context, rt ResolvedTool) (string, error) {
	if m.BinDir == "" {
		return "", errors.New("tools bin directory is not configured")
	}
	if err := os.MkdirAll(m.BinDir, 0o755); err != nil {
		return "", fmt.Errorf("creating tools bin dir: %w", err)
	}

	binName := rt.Manifest.Executable
	if binName == "" {
		binName = rt.Manifest.ID
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(binName), ".exe") {
		binName += ".exe"
	}
	finalPath := filepath.Join(m.BinDir, binName)

	stageDir, err := os.MkdirTemp(m.BinDir, ".staging-"+rt.Manifest.ID+"-*")
	if err != nil {
		return "", fmt.Errorf("creating staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir)
	stagedBin := filepath.Join(stageDir, binName)

	switch {
	case len(rt.BuildCmd) > 0:
		// Execute declared build command with ANX_TOOL_OUTPUT pointing to stagedBin
		cmdArgs := make([]string, len(rt.BuildCmd))
		for i, a := range rt.BuildCmd {
			cmdArgs[i] = strings.ReplaceAll(a, "${OUTPUT}", stagedBin)
		}
		cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = rt.SourceDir
		cmd.Env = append(os.Environ(), "ANX_TOOL_OUTPUT="+stagedBin)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("building tool %q failed (previous installation preserved): %w\n%s", rt.Manifest.ID, err, strings.TrimSpace(string(out)))
		}
		if _, statErr := os.Stat(stagedBin); statErr != nil {
			return "", fmt.Errorf("build command for %q did not produce %s (previous installation preserved)", rt.Manifest.ID, stagedBin)
		}
	case rt.Entrypoint != "":
		srcPath := filepath.Join(rt.SourceDir, filepath.FromSlash(rt.Entrypoint))
		absSrc, err := filepath.Abs(srcPath)
		if err != nil {
			return "", err
		}
		absRoot, err := filepath.Abs(rt.SourceDir)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(absRoot, absSrc)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("tool %q entrypoint %q escapes source directory", rt.Manifest.ID, rt.Entrypoint)
		}
		data, err := os.ReadFile(absSrc)
		if err != nil {
			return "", fmt.Errorf("reading entrypoint %q: %w", absSrc, err)
		}
		if err := os.WriteFile(stagedBin, data, 0o755); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("tool %q from source %q has neither entrypoint nor build_cmd", rt.Manifest.ID, rt.SourceID)
	}

	if err := os.Chmod(stagedBin, 0o755); err != nil && runtime.GOOS != "windows" {
		return "", err
	}

	// Atomic replacement on same filesystem
	if err := os.Rename(stagedBin, finalPath); err != nil {
		return "", fmt.Errorf("atomically installing %s: %w", finalPath, err)
	}
	return finalPath, nil
}

func (m *SourceManager) git() *gittransport.Adapter {
	if m.Git != nil {
		return m.Git
	}
	return gittransport.NewAdapter()
}

func (m *SourceManager) sourceLocalDir(s ToolSource) string {
	if s.Path != "" {
		return s.Path
	}
	norm := gittransport.NormalizeURL(s.URL)
	if norm.IsLocal {
		return norm.Canonical
	}
	return filepath.Join(m.CacheDir, s.ID)
}

func (m *SourceManager) ensureSourceDir(ctx context.Context, s ToolSource, allowClone bool) (string, error) {
	if s.Type == SourceTypeBuiltin {
		return m.BundledDir, nil
	}
	if s.Path != "" {
		if !dirExists(s.Path) {
			return "", fmt.Errorf("local tool source path %q does not exist", s.Path)
		}
		return s.Path, nil
	}
	norm := gittransport.NormalizeURL(s.URL)
	if norm.IsLocal {
		if !dirExists(norm.Canonical) {
			return "", fmt.Errorf("local tool source path %q does not exist", norm.Canonical)
		}
		return norm.Canonical, nil
	}
	dest := filepath.Join(m.CacheDir, s.ID)
	if dirExists(dest) {
		return dest, nil
	}
	if !allowClone {
		return "", fmt.Errorf("tool source %q not cloned at %s", s.ID, dest)
	}
	mode, err := gittransport.ParseAuthMode(s.Auth)
	if err != nil {
		return "", err
	}
	_, err = m.git().Clone(ctx, gittransport.CloneOptions{
		URL:    s.URL,
		Ref:    s.Ref,
		Dest:   dest,
		Depth:  1,
		Auth:   mode,
		SSHKey: s.SSHKey,
	})
	if err != nil {
		return "", err
	}
	return dest, nil
}

func (m *SourceManager) discoverFromDir(s ToolSource, dir string) ([]ResolvedTool, error) {
	if dir == "" || !dirExists(dir) {
		return nil, nil
	}
	var out []ResolvedTool

	switch s.Type {
	case SourceTypeSingle:
		rt, err := loadSingleToolFile(s, dir, filepath.Join(dir, "tool.yaml"))
		if err != nil {
			return nil, err
		}
		out = append(out, rt)

	case SourceTypeBuiltin, SourceTypeCollection:
		// 1. Optional collection-level tools.yaml
		rootTools := filepath.Join(dir, "tools.yaml")
		if fi, err := os.Stat(rootTools); err == nil && !fi.IsDir() {
			data, err := os.ReadFile(rootTools)
			if err != nil {
				return nil, err
			}
			var doc toolsDocument
			dec := yaml.NewDecoder(bytes.NewReader(data))
			dec.KnownFields(true)
			if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("parsing %s: %w", rootTools, err)
			}
			for _, t := range doc.Tools {
				if err := ValidateManifest(t); err != nil {
					return nil, fmt.Errorf("%s: %w", rootTools, err)
				}
				out = append(out, ResolvedTool{
					SourceID:   s.ID,
					SourceType: s.Type,
					SourceDir:  dir,
					Manifest:   t,
				})
			}
		}

		// 2. Discover tools/<name>/tool.yaml (or direct <name>/tool.yaml for bundled/)
		searchRoots := []string{filepath.Join(dir, "tools")}
		if s.Type == SourceTypeBuiltin {
			searchRoots = []string{dir}
		}
		seen := make(map[string]bool)
		for _, existing := range out {
			seen[existing.Manifest.ID] = true
		}
		for _, sr := range searchRoots {
			entries, err := os.ReadDir(sr)
			if err != nil {
				continue
			}
			for _, ent := range entries {
				if !ent.IsDir() {
					continue
				}
				toolDir := filepath.Join(sr, ent.Name())
				manifestPath := filepath.Join(toolDir, "tool.yaml")
				if _, err := os.Stat(manifestPath); err != nil {
					continue
				}
				rt, err := loadSingleToolFile(s, toolDir, manifestPath)
				if err != nil {
					return nil, err
				}
				if seen[rt.Manifest.ID] {
					return nil, fmt.Errorf("%w: duplicate tool %q in source %q", ErrInvalidManifest, rt.Manifest.ID, s.ID)
				}
				seen[rt.Manifest.ID] = true
				out = append(out, rt)
			}
		}
	}

	slices.SortFunc(out, func(a, b ResolvedTool) int {
		return strings.Compare(a.Manifest.ID, b.Manifest.ID)
	})
	return out, nil
}

func loadSingleToolFile(s ToolSource, toolDir, manifestPath string) (ResolvedTool, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ResolvedTool{}, fmt.Errorf("reading %s: %w", manifestPath, err)
	}
	var st SingleToolManifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&st); err != nil {
		return ResolvedTool{}, fmt.Errorf("parsing %s: %w", manifestPath, err)
	}
	if st.SchemaVersion != 0 && st.SchemaVersion != 1 {
		return ResolvedTool{}, fmt.Errorf("%s: unsupported schema_version %d", manifestPath, st.SchemaVersion)
	}
	tm := domain.ToolManifest{
		ID:              st.ID,
		Name:            st.Name,
		Category:        st.Category,
		Description:     st.Description,
		Executable:      st.Executable,
		Language:        st.Language,
		VersionArgs:     st.VersionArgs,
		AllowGlobalPATH: st.AllowGlobalPATH,
		FallbackBuiltin: st.FallbackBuiltin,
		RepoURL:         st.RepoURL,
		UpdateStrategy:  st.UpdateStrategy,
		UpdateCmd:       st.UpdateCmd,
	}
	if tm.RepoURL == "" && s.URL != "" {
		tm.RepoURL = gittransport.NormalizeURL(s.URL).Redacted
	}
	if err := ValidateManifest(tm); err != nil {
		return ResolvedTool{}, fmt.Errorf("%s: %w", manifestPath, err)
	}
	return ResolvedTool{
		SourceID:   s.ID,
		SourceType: s.Type,
		SourceDir:  toolDir,
		Entrypoint: st.Entrypoint,
		BuildCmd:   st.BuildCmd,
		Manifest:   tm,
	}, nil
}

func detectSourceType(dir string) ToolSourceType {
	if _, err := os.Stat(filepath.Join(dir, "tool.yaml")); err == nil {
		return SourceTypeSingle
	}
	if _, err := os.Stat(filepath.Join(dir, "tools.yaml")); err == nil {
		return SourceTypeCollection
	}
	if dirExists(filepath.Join(dir, "tools")) {
		return SourceTypeCollection
	}
	return ""
}

func deriveSourceID(rawURL, rawPath string) string {
	src := strings.TrimSpace(rawURL)
	if src == "" {
		src = strings.TrimSpace(rawPath)
	}
	src = strings.TrimSuffix(src, "/")
	src = strings.TrimSuffix(src, ".git")
	base := filepath.Base(src)
	if idx := strings.LastIndex(base, ":"); idx >= 0 {
		base = base[idx+1:]
	}
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" || base == "." || base == "/" {
		return "custom-tools"
	}
	return base
}
