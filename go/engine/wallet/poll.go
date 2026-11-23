package wallet

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"harbour-freesailwallet/engine/pass"
)

type poller struct {
	Client *http.Client
}

func defaultPoller() poller {
	return poller{Client: &http.Client{Timeout: 20 * time.Second}}
}

func (p poller) client() *http.Client {
	base := p.Client
	if base == nil {
		base = &http.Client{Timeout: 20 * time.Second}
	}
	copied := *base
	copied.CheckRedirect = refuseCrossHostRedirect
	if copied.Timeout == 0 {
		copied.Timeout = 20 * time.Second
	}
	return &copied
}

func refuseCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	first := via[0]
	if req.URL.Scheme != first.URL.Scheme || req.URL.Host != first.URL.Host {
		return fmt.Errorf("refusing redirect to %s", req.URL.Host)
	}
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return nil
}

// PollResult is the outcome of one poll of the wallet.
type PollResult struct {
	Checked   int         `json:"checked"`
	Updated   int         `json:"updated"`
	Unchanged int         `json:"unchanged"`
	Errors    []PassError `json:"errors"`
	LastPoll  string      `json:"lastPoll"`
}

// PassError is a per-pass poll failure.
type PassError struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type pollJob struct {
	id    string
	pass  *pass.Pass
	since *time.Time
}

type pollFetch struct {
	job      pollJob
	body     []byte
	modified *time.Time
	status   int
	err      error
}

// Poll checks every active pass that declares a web service.
func (s *Store) Poll() (*PollResult, error) {
	return s.poll(nil)
}

// PollPass checks one pass. Archived passes are skipped.
func (s *Store) PollPass(id string) (*PollResult, error) {
	return s.poll(&id)
}

func (s *Store) poll(only *string) (*PollResult, error) {
	jobs, res, err := s.preparePoll(only)
	if err != nil {
		return nil, err
	}
	fetched := make([]pollFetch, 0, len(jobs))
	for _, job := range jobs {
		body, modified, status, err := s.fetchPass(job.pass, job.since)
		fetched = append(fetched, pollFetch{
			job: job, body: body, modified: modified, status: status, err: err,
		})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range fetched {
		s.finishPoll(res, item)
	}
	now := s.now().UTC()
	s.state.LastPoll = now
	res.LastPoll = now.Format(time.RFC3339)
	if err := s.save(); err != nil {
		return nil, err
	}
	return res, nil
}

// preparePoll records retention changes and lists the passes to fetch.
// The network calls happen after this lock is released.
func (s *Store) preparePoll(only *string) ([]pollJob, *PollResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := &PollResult{Errors: []PassError{}}
	if only != nil {
		if _, ok := s.state.Passes[*only]; !ok {
			return nil, nil, fmt.Errorf("pass not found")
		}
	}
	var jobs []pollJob
	dirty := false
	for id, rec := range s.state.Passes {
		if rec == nil || (only != nil && id != *only) || rec.ArchivedAt != nil {
			continue
		}
		p, err := s.inspectCurrent(id)
		if err != nil {
			rec.LastError = err.Error()
			res.Errors = append(res.Errors, PassError{ID: id, Message: err.Error()})
			dirty = true
			continue
		}
		if s.syncRetention(rec, p) {
			dirty = true
		}
		if rec.ArchivedAt != nil || !p.HasWebService {
			continue
		}
		var since *time.Time
		if rec.LastModified != nil {
			t := rec.LastModified.UTC()
			since = &t
		}
		jobs = append(jobs, pollJob{id: id, pass: p, since: since})
	}
	if dirty {
		if err := s.save(); err != nil {
			return nil, nil, err
		}
	}
	return jobs, res, nil
}

func (s *Store) finishPoll(res *PollResult, item pollFetch) {
	rec := s.state.Passes[item.job.id]
	if rec == nil || rec.ArchivedAt != nil {
		return
	}
	res.Checked++
	if item.err != nil {
		rec.LastError = item.err.Error()
		res.Errors = append(res.Errors, PassError{ID: item.job.id, Message: item.err.Error()})
		return
	}
	if item.status == http.StatusNotModified || item.status == 0 && item.body == nil {
		rec.LastError = ""
		res.Unchanged++
		return
	}
	updated, err := s.applyBytes(item.job.id, item.body, "poll", item.modified)
	if err != nil {
		rec.LastError = err.Error()
		res.Errors = append(res.Errors, PassError{ID: item.job.id, Message: err.Error()})
		return
	}
	if updated {
		res.Updated++
	} else {
		res.Unchanged++
	}
}

func (s *Store) fetchPass(p *pass.Pass, since *time.Time) ([]byte, *time.Time, int, error) {
	endpoint, err := passURL(p.WebServiceURL, p.PassTypeIdentifier, p.SerialNumber)
	if err != nil {
		return nil, nil, 0, err
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	req.Header.Set("Authorization", "ApplePass "+p.AuthenticationToken)
	if since != nil && !since.IsZero() {
		req.Header.Set("If-Modified-Since", since.UTC().Format(http.TimeFormat))
	}
	resp, err := s.fetch.client().Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, nil, http.StatusNotModified, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, resp.StatusCode, fmt.Errorf("update returned HTTP %d", resp.StatusCode)
	}
	body, err := pass.ReadLimited(resp.Body, pass.MaxEntryBytes)
	if err != nil {
		return nil, nil, resp.StatusCode, err
	}
	modified := s.now().UTC()
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		modified = t.UTC()
	}
	return body, &modified, http.StatusOK, nil
}

func passURL(base, passType, serial string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("webServiceURL must use https")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/passes/" + url.PathEscape(passType) + "/" + url.PathEscape(serial)
	return u.String(), nil
}

// applyBytes replaces the current pass. A file that is not a pass is refused
// and the previous bytes stay in place. An import may store an untrusted pass.
// A polled update must verify and must be the same pass.
func (s *Store) applyBytes(id string, data []byte, source string, modified *time.Time) (bool, error) {
	next, err := pass.Inspect(data, s.inspectOpt(""))
	if err != nil {
		return false, fmt.Errorf("updated pass was not stored: %w", err)
	}
	if pass.ID(next.PassTypeIdentifier, next.SerialNumber) != id {
		return false, fmt.Errorf("updated pass does not match this pass")
	}
	if source == "poll" && !next.Trusted {
		return false, fmt.Errorf("updated pass is not trusted")
	}
	rec := s.state.Passes[id]
	if rec == nil {
		rec = &record{PassTypeIdentifier: next.PassTypeIdentifier, SerialNumber: next.SerialNumber}
		s.state.Passes[id] = rec
	}
	curPath, err := s.currentPath(id)
	if err != nil {
		return false, err
	}
	oldBytes, readErr := os.ReadFile(curPath)
	if readErr == nil && string(oldBytes) == string(data) {
		rec.LastError = ""
		if modified != nil {
			rec.LastModified = modified
		}
		s.syncRetention(rec, next)
		return false, nil
	}
	var changes []pass.FieldChange
	var messages []string
	if readErr == nil {
		if old, err := pass.Inspect(oldBytes, s.inspectOpt("")); err == nil {
			changes, messages = pass.DiffFields(old, next)
		}
		stamp := s.now().UTC().Format("20060102T150405.000000000Z")
		rel := "history/" + stamp + ".pkpass"
		dest, err := s.passFile(id, rel)
		if err != nil {
			return false, err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return false, err
		}
		if err := os.WriteFile(dest, oldBytes, 0o644); err != nil {
			return false, err
		}
		for i := len(rec.Revisions) - 1; i >= 0; i-- {
			if rec.Revisions[i].File == "" {
				rec.Revisions[i].File = rel
				break
			}
		}
	}
	dir, err := s.passDir(id)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(curPath, data, 0o644); err != nil {
		return false, err
	}
	_ = os.RemoveAll(filepath.Join(dir, "img"))
	rec.PassTypeIdentifier = next.PassTypeIdentifier
	rec.SerialNumber = next.SerialNumber
	rec.LastError = ""
	if modified != nil {
		rec.LastModified = modified
	}
	rec.Revisions = append(rec.Revisions, revision{
		At:       s.now().UTC(),
		Source:   source,
		Changes:  changes,
		Messages: messages,
	})
	s.syncRetention(rec, next)
	return true, nil
}
