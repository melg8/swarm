// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Icon serving of the equipment widget: the item icon pack lives in
// data/icons of the repository (3134 PNG files of the classic client
// naming scheme, see data/icons/Readme.txt) and the web interface
// serves them at /icons/<name>.png with long lived caching - an icon
// name never changes once the pack is in place.

// iconsCacheMaxAge is the cache duration of one icon response.
const iconsCacheMaxAge = "public, max-age=86400"

// iconsDirCandidates returns the icon pack locations checked in
// order: data/icons relative to the working directory first (the bot
// runs from the repository root, like the geodata detection of
// cmd/swarm), then the walk up from the current directory for other
// launch directories. The candidates are computed per call so tests
// with a changed working directory see them.
func iconsDirCandidates() []string {
	candidates := []string{filepath.Join("data", "icons")}
	if dir, err := os.Getwd(); err == nil {
		for walk := dir; ; {
			parent := filepath.Dir(walk)
			if parent == walk {
				break
			}
			walk = parent
			candidates = append(candidates,
				filepath.Join(walk, "data", "icons"))
		}
	}

	return candidates
}

// detectIconsDir picks the first icon pack directory that exists and
// contains PNG files, or an empty string when none does.
func detectIconsDir() string {
	for _, candidate := range iconsDirCandidates() {
		entries, err := os.ReadDir(candidate)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".png") {
				return candidate
			}
		}
	}

	return ""
}

// serveIcons answers GET /icons/{name}.png from the icon pack on disk.
// Missing packs answer 404 for every name, the widget renders its
// fallback glyphs instead. The path is cleaned and must stay inside
// the pack directory.
func (s *Server) serveIcons(w http.ResponseWriter, r *http.Request) {
	dir := s.iconsDir.Load().(string)
	if dir == "" {
		http.NotFound(w, r)

		return
	}

	name := r.PathValue("name")
	// The URL carries the .png extension, the pack stores the icon
	// names without it.
	name = strings.TrimSuffix(name, ".png")
	if name == "" || strings.ContainsAny(name, `/\`) {
		http.NotFound(w, r)

		return
	}
	// The file name allows letters, digits, underscores and a hyphen
	// of the classic client icon naming scheme, nothing else.
	for _, ch := range name {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' ||
			ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			http.NotFound(w, r)

			return
		}
	}

	path := filepath.Join(dir, name+".png")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)

		return
	}

	w.Header().Set("Cache-Control", iconsCacheMaxAge)
	http.ServeFile(w, r, path)
}

// initIconsDir resolves the icon pack directory once at server start
// and remembers it for the handler.
func (s *Server) initIconsDir(logger *log.Logger) {
	dir := ""
	if override := os.Getenv("SWARM_ICONS"); override != "" {
		dir = override
	} else {
		dir = detectIconsDir()
	}
	if dir == "" {
		logger.Println("No icon pack found in data/icons, the equipment" +
			" widget renders fallback glyphs")
	} else {
		logger.Printf("Icon pack ready: %s", dir)
	}
	s.iconsDir.Store(dir)
}
