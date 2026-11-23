package wallet

import (
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"harbour-freesailwallet/engine/pass"
)

// Store is the on-disk wallet. Pass bytes stay in the files directory.
type Store struct {
	mu     sync.Mutex
	dir    string
	state  disk
	locale string
	scale  float64
	now    func() time.Time
	fetch  poller
	roots  *x509.CertPool
}

type disk struct {
	LastPoll time.Time          `json:"lastPoll,omitempty"`
	Passes   map[string]*record `json:"passes"`
}

type record struct {
	PassTypeIdentifier string     `json:"passTypeIdentifier"`
	SerialNumber       string     `json:"serialNumber"`
	ArchivedAt         *time.Time `json:"archivedAt,omitempty"`
	KeepActive         bool       `json:"keepActive,omitempty"`
	LastModified       *time.Time `json:"lastModified,omitempty"`
	LastError          string     `json:"lastError,omitempty"`
	Revisions          []revision `json:"revisions"`
	BundlePath         string     `json:"bundlePath,omitempty"`
	HomeIndex          *int       `json:"homeIndex,omitempty"`
}

type revision struct {
	At       time.Time          `json:"at"`
	Source   string             `json:"source"`
	File     string             `json:"file,omitempty"`
	Changes  []pass.FieldChange `json:"changes,omitempty"`
	Messages []string           `json:"messages,omitempty"`
}

// Options configures a store. Zero values use the defaults.
type Options struct {
	Now    func() time.Time
	Locale string
	Scale  float64
	Poll   poller
	Roots  *x509.CertPool
}

// Open loads or creates a wallet in dir.
func Open(dir string) (*Store, error) {
	return OpenOptions(dir, Options{})
}

// OpenOptions loads a wallet with test hooks.
func OpenOptions(dir string, opt Options) (*Store, error) {
	if dir == "" {
		return nil, errors.New("wallet directory is empty")
	}
	if err := os.MkdirAll(filepath.Join(dir, "passes"), 0o755); err != nil {
		return nil, err
	}
	st := &Store{
		dir:    dir,
		locale: opt.Locale,
		scale:  opt.Scale,
		now:    opt.Now,
		fetch:  opt.Poll,
		roots:  opt.Roots,
	}
	if st.now == nil {
		st.now = time.Now
	}
	if st.scale <= 0 {
		st.scale = 3
	}
	if st.locale == "" {
		st.locale = "en"
	}
	if st.fetch.Client == nil {
		st.fetch = defaultPoller()
	}
	if err := st.load(); err != nil {
		return nil, err
	}
	return st, nil
}

// SetDisplay stores the locale and pixel scale used when rendering passes.
func (s *Store) SetDisplay(locale string, scale float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if locale != "" {
		s.locale = locale
	}
	if scale > 0 {
		s.scale = scale
	}
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.indexPath())
	if errors.Is(err, os.ErrNotExist) {
		s.state = disk{Passes: map[string]*record{}}
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &s.state); err != nil {
		return fmt.Errorf("wallet.json: %w", err)
	}
	if s.state.Passes == nil {
		s.state.Passes = map[string]*record{}
	}
	return nil
}

func (s *Store) save() error {
	if s.state.Passes == nil {
		s.state.Passes = map[string]*record{}
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.indexPath(), b)
}

func (s *Store) indexPath() string { return filepath.Join(s.dir, "wallet.json") }

func validID(id string) error {
	if id == "" || id == "." || id == ".." || strings.Contains(id, "..") {
		return fmt.Errorf("invalid pass id")
	}
	if strings.ContainsAny(id, `/\`) || id != safe(id) {
		return fmt.Errorf("invalid pass id")
	}
	return nil
}

func withinDir(root, full string) error {
	root = filepath.Clean(root)
	full = filepath.Clean(full)
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return fmt.Errorf("path escapes the pass directory")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes the pass directory")
	}
	return nil
}

func (s *Store) passDir(id string) (string, error) {
	if err := validID(id); err != nil {
		return "", err
	}
	root := filepath.Join(s.dir, "passes")
	dir := filepath.Join(root, id)
	if err := withinDir(root, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func (s *Store) currentPath(id string) (string, error) {
	dir, err := s.passDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "current.pkpass"), nil
}

func (s *Store) passFile(id, rel string) (string, error) {
	dir, err := s.passDir(id)
	if err != nil {
		return "", err
	}
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == "." || rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes the pass directory")
	}
	full := filepath.Join(dir, rel)
	if err := withinDir(dir, full); err != nil {
		return "", err
	}
	return full, nil
}

// Import adds a pkpass, a pkpasses bundle, or a wallet backup zip.
func (s *Store) Import(data []byte) (*ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if isBackup(data) {
		return s.restore(data)
	}
	parts, err := bundleParts(data)
	if err != nil {
		return nil, err
	}
	res := &ImportResult{}
	if len(parts) > 10 && looksLikePkpasses(data) {
		res.Problems = append(res.Problems, "a .pkpasses bundle is limited to 10 passes; extra files were skipped")
		parts = parts[:10]
	}
	for _, part := range parts {
		sum, prob := s.importOne(part, "import")
		if prob != "" {
			res.Problems = append(res.Problems, prob)
			continue
		}
		res.Passes = append(res.Passes, sum)
	}
	if len(res.Passes) == 0 && len(res.Problems) > 0 {
		return res, errors.New(res.Problems[0])
	}
	return res, nil
}

func (s *Store) importOne(data []byte, source string) (pass.Summary, string) {
	p, err := pass.Inspect(data, s.inspectOpt(""))
	if err != nil {
		return pass.Summary{}, err.Error()
	}
	if _, err := s.applyBytes(p.ID, data, source, nil); err != nil {
		return pass.Summary{}, err.Error()
	}
	if err := s.save(); err != nil {
		return pass.Summary{}, err.Error()
	}
	got, err := s.inspectCurrent(p.ID)
	if err != nil {
		return pass.Summary{}, err.Error()
	}
	rec := s.state.Passes[p.ID]
	dirty := false
	if rec != nil && rec.HomeIndex == nil && rec.ArchivedAt == nil && s.hasCustomOrder(p.ID) {
		next := s.nextHomeIndex()
		rec.HomeIndex = &next
		dirty = true
	}
	if s.syncRetention(rec, got) {
		dirty = true
	}
	if dirty {
		if err := s.save(); err != nil {
			return pass.Summary{}, err.Error()
		}
	}
	return s.summary(p.ID, got, rec), ""
}

// List splits the wallet into active and archived passes.
func (s *Store) List() (*List, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &List{Active: []pass.Summary{}, Archived: []pass.Summary{}}
	if !s.state.LastPoll.IsZero() {
		out.LastPoll = s.state.LastPoll.UTC().Format(time.RFC3339)
	}
	dirty := false
	for id, rec := range s.state.Passes {
		if rec == nil {
			out.Problems = append(out.Problems, id+": missing wallet record")
			continue
		}
		p, err := s.inspectCurrent(id)
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: %s", id, err.Error()))
			continue
		}
		if s.syncRetention(rec, p) {
			dirty = true
		}
		sum := s.summary(id, p, rec)
		if rec.ArchivedAt != nil {
			sum.Archived = true
			out.Archived = append(out.Archived, sum)
		} else {
			out.Active = append(out.Active, sum)
		}
	}
	if dirty {
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	sort.Slice(out.Active, func(i, j int) bool { return s.lessActive(out.Active[i], out.Active[j]) })
	sort.Slice(out.Archived, func(i, j int) bool { return lessSummary(out.Archived[i], out.Archived[j]) })
	return out, nil
}

func (s *Store) lessActive(a, b pass.Summary) bool {
	ai, aok := s.homeIndex(a.ID)
	bi, bok := s.homeIndex(b.ID)
	if aok && bok && ai != bi {
		return ai < bi
	}
	if aok != bok {
		return aok
	}
	return lessSummary(a, b)
}

func (s *Store) homeIndex(id string) (int, bool) {
	rec := s.state.Passes[id]
	if rec == nil || rec.HomeIndex == nil {
		return 0, false
	}
	return *rec.HomeIndex, true
}

// hasCustomOrder reports whether some other active pass has a saved position.
func (s *Store) hasCustomOrder(except string) bool {
	for id, other := range s.state.Passes {
		if id == except || other == nil || other.ArchivedAt != nil || other.HomeIndex == nil {
			continue
		}
		return true
	}
	return false
}

func (s *Store) nextHomeIndex() int {
	max := -1
	for _, other := range s.state.Passes {
		if other == nil || other.ArchivedAt != nil || other.HomeIndex == nil {
			continue
		}
		if *other.HomeIndex > max {
			max = *other.HomeIndex
		}
	}
	return max + 1
}

func lessSummary(a, b pass.Summary) bool {
	if a.OrganizationName != b.OrganizationName {
		return a.OrganizationName < b.OrganizationName
	}
	return a.ID < b.ID
}

// IsArchived reports whether the pass is in the archive.
func (s *Store) IsArchived(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.state.Passes[id]
	return rec != nil && rec.ArchivedAt != nil
}

// Get returns the current display model for a pass.
func (s *Store) Get(id string) (*pass.Pass, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Passes[id]; !ok {
		return nil, fmt.Errorf("pass not found")
	}
	p, err := s.inspectCurrent(id)
	if err != nil {
		return nil, err
	}
	p.ID = id
	return p, nil
}

// GetRevision returns one stored revision. "current" is the live pass.
func (s *Store) GetRevision(id, revID string) (*pass.Pass, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.state.Passes[id]
	if !ok {
		return nil, fmt.Errorf("pass not found")
	}
	path, err := s.revisionPath(id, rec, revID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dir, err := s.passDir(id)
	if err != nil {
		return nil, err
	}
	return pass.Inspect(b, s.inspectOpt(filepath.Join(dir, "view", safe(revID))))
}

// History lists revisions, newest first.
func (s *Store) History(id string) (*History, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.state.Passes[id]
	if !ok {
		return nil, fmt.Errorf("pass not found")
	}
	h := &History{ID: id, Revisions: []RevisionView{}}
	for i := len(rec.Revisions) - 1; i >= 0; i-- {
		rev := rec.Revisions[i]
		h.Revisions = append(h.Revisions, revisionView(rev, rev.File == ""))
	}
	return h, nil
}

// Archive removes a pass from the home stack and pauses polling.
func (s *Store) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.state.Passes[id]
	if !ok {
		return fmt.Errorf("pass not found")
	}
	now := s.now().UTC()
	rec.ArchivedAt = &now
	rec.KeepActive = false
	rec.HomeIndex = nil
	return s.save()
}

// Unarchive returns a pass to the home stack and to polling.
func (s *Store) Unarchive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.state.Passes[id]
	if !ok {
		return fmt.Errorf("pass not found")
	}
	rec.ArchivedAt = nil
	rec.KeepActive = false
	if p, err := s.inspectCurrent(id); err == nil && retired(p) {
		rec.KeepActive = true
	}
	if rec.HomeIndex == nil && s.hasCustomOrder(id) {
		next := s.nextHomeIndex()
		rec.HomeIndex = &next
	}
	return s.save()
}

// Move reorders an active pass. place is top, bottom, up, or down.
// The new order is written to the wallet and kept across launches.
func (s *Store) Move(id, place string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.moveLocked(id, place)
	return err
}

// BringToFront places an active pass at home index 0. Archived passes are
// left in place. moved is false when the pass was already there.
func (s *Store) BringToFront(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.state.Passes[id]
	if !ok {
		return false, fmt.Errorf("pass not found")
	}
	if rec.ArchivedAt != nil {
		return false, nil
	}
	return s.moveLocked(id, "top")
}

func (s *Store) moveLocked(id, place string) (bool, error) {
	rec, ok := s.state.Passes[id]
	if !ok {
		return false, fmt.Errorf("pass not found")
	}
	if rec.ArchivedAt != nil {
		return false, fmt.Errorf("pass is archived")
	}
	ids := s.activeIDsLocked()
	from := -1
	for i, cur := range ids {
		if cur == id {
			from = i
			break
		}
	}
	if from < 0 {
		return false, fmt.Errorf("pass not found")
	}
	to, err := moveIndex(place, from, len(ids))
	if err != nil {
		return false, err
	}
	if to == from {
		return false, nil
	}
	next := make([]string, 0, len(ids))
	next = append(next, ids[:from]...)
	next = append(next, ids[from+1:]...)
	rest := append([]string{id}, next[to:]...)
	next = append(next[:to], rest...)
	for i, pid := range next {
		rec := s.state.Passes[pid]
		if rec == nil {
			continue
		}
		idx := i
		rec.HomeIndex = &idx
	}
	if err := s.save(); err != nil {
		return false, err
	}
	return true, nil
}

// activeIDsLocked matches lessActive. Home indexes decide the order. Names
// are read only when a pass has no index yet, and only from pass.json.
func (s *Store) activeIDsLocked() []string {
	type key struct {
		id   string
		home int
		has  bool
		org  string
	}
	keys := make([]key, 0, len(s.state.Passes))
	missing := 0
	seen := map[int]int{}
	for id, rec := range s.state.Passes {
		if rec == nil || rec.ArchivedAt != nil {
			continue
		}
		k := key{id: id}
		if rec.HomeIndex != nil {
			k.home = *rec.HomeIndex
			k.has = true
			seen[k.home]++
		} else {
			missing++
		}
		keys = append(keys, k)
	}
	dup := false
	for _, n := range seen {
		if n > 1 {
			dup = true
			break
		}
	}
	if missing > 1 || dup {
		for i := range keys {
			if !keys[i].has || seen[keys[i].home] > 1 {
				keys[i].org = s.organizationName(keys[i].id)
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.has && b.has && a.home != b.home {
			return a.home < b.home
		}
		if a.has != b.has {
			return a.has
		}
		if a.org != b.org {
			return a.org < b.org
		}
		return a.id < b.id
	})
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = k.id
	}
	return ids
}

func (s *Store) organizationName(id string) string {
	path, err := s.currentPath(id)
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return pass.OrganizationName(b, s.locale)
}

func moveIndex(place string, from, n int) (int, error) {
	switch place {
	case "top":
		return 0, nil
	case "bottom":
		return n - 1, nil
	case "up":
		if from == 0 {
			return from, nil
		}
		return from - 1, nil
	case "down":
		if from >= n-1 {
			return from, nil
		}
		return from + 1, nil
	default:
		return 0, fmt.Errorf("unknown move %q", place)
	}
}

// syncRetention archives an expired or voided pass unless the user put it
// back. A pass that is current again drops that exception. The caller saves.
func (s *Store) syncRetention(rec *record, p *pass.Pass) bool {
	if rec == nil || p == nil {
		return false
	}
	if !retired(p) {
		if !rec.KeepActive {
			return false
		}
		rec.KeepActive = false
		return true
	}
	if rec.ArchivedAt != nil || rec.KeepActive {
		return false
	}
	now := s.now().UTC()
	rec.ArchivedAt = &now
	rec.HomeIndex = nil
	return true
}

func retired(p *pass.Pass) bool {
	return p.Expired || p.Voided
}

// BarcodePNG draws the current pass barcode.
func (s *Store) BarcodePNG(id string, size int) (*BarcodeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.inspectCurrent(id)
	if err != nil {
		return nil, err
	}
	if p.Barcode == nil {
		return &BarcodeResult{Supported: false}, nil
	}
	png, err := pass.BarcodePNG(p.Barcode, size)
	if err != nil {
		return nil, err
	}
	return &BarcodeResult{
		PNG:       pass.PNGBase64(png),
		AltText:   p.Barcode.AltText,
		Format:    p.Barcode.Format,
		Message:   p.Barcode.Message,
		Supported: p.Barcode.Supported && len(png) > 0,
	}, nil
}

func (s *Store) inspectCurrent(id string) (*pass.Pass, error) {
	path, err := s.currentPath(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dir, err := s.passDir(id)
	if err != nil {
		return nil, err
	}
	return pass.Inspect(b, s.inspectOpt(filepath.Join(dir, "img")))
}

func (s *Store) inspectOpt(dir string) pass.Options {
	return pass.Options{Roots: s.roots, Now: s.now(), Locale: s.locale, Scale: s.scale, ImageDir: dir}
}

func (s *Store) summary(id string, p *pass.Pass, rec *record) pass.Summary {
	sum := p.Summary()
	sum.ID = id
	sum.LastError = rec.LastError
	sum.Archived = rec.ArchivedAt != nil
	if n := len(rec.Revisions); n >= 2 {
		prev := rec.Revisions[n-1]
		if len(prev.Messages) > 0 {
			sum.WhatChanged = prev.Messages[0]
		} else if len(prev.Changes) > 0 {
			c := prev.Changes[0]
			label := c.Label
			if label == "" {
				label = c.Key
			}
			sum.WhatChanged = label + ": " + c.To
		}
	}
	return sum
}

func (s *Store) revisionPath(id string, rec *record, revID string) (string, error) {
	if revID == "" || revID == "current" {
		return s.currentPath(id)
	}
	for _, rev := range rec.Revisions {
		if revisionView(rev, rev.File == "").ID != revID {
			continue
		}
		if rev.File == "" {
			return s.currentPath(id)
		}
		return s.passFile(id, rev.File)
	}
	return "", fmt.Errorf("revision not found")
}

func revisionView(rev revision, current bool) RevisionView {
	id := rev.At.UTC().Format(time.RFC3339Nano)
	if current {
		id = "current"
	}
	return RevisionView{
		ID:       id,
		At:       rev.At.UTC().Format(time.RFC3339),
		Source:   rev.Source,
		Current:  current,
		Changes:  rev.Changes,
		Messages: rev.Messages,
	}
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func safe(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "pass"
	}
	return b.String()
}

// ImportResult is the JSON result of an import.
type ImportResult struct {
	Passes   []pass.Summary `json:"passes"`
	Restored bool           `json:"restored"`
	Problems []string       `json:"problems"`
}

// List is the home screen payload.
type List struct {
	LastPoll string         `json:"lastPoll,omitempty"`
	Active   []pass.Summary `json:"active"`
	Archived []pass.Summary `json:"archived"`
	Problems []string       `json:"problems,omitempty"`
}

// History is the revision timeline for one pass.
type History struct {
	ID        string         `json:"id"`
	Revisions []RevisionView `json:"revisions"`
}

// RevisionView is one history row.
type RevisionView struct {
	ID       string             `json:"id"`
	At       string             `json:"at"`
	Source   string             `json:"source"`
	Current  bool               `json:"current"`
	Changes  []pass.FieldChange `json:"changes"`
	Messages []string           `json:"messages"`
}

// BarcodeResult is a drawn barcode.
type BarcodeResult struct {
	PNG       string `json:"png,omitempty"`
	AltText   string `json:"altText,omitempty"`
	Format    string `json:"format,omitempty"`
	Message   string `json:"message,omitempty"`
	Supported bool   `json:"supported"`
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Passes[id]; !ok {
		return fmt.Errorf("pass not found")
	}
	dir, err := s.passDir(id)
	if err != nil {
		return err
	}
	delete(s.state.Passes, id)
	if err := s.save(); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
