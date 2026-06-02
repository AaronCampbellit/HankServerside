package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

const (
	maxHomeQuickLinks       = 64
	maxQuickLinkTitleLength = 80
	maxQuickLinkURLLength   = 2048
	maxQuickLinkDescLength  = 180
	maxQuickLinkErrorLength = 180
)

type quickLinkRequest struct {
	Title              string `json:"title"`
	URL                string `json:"url"`
	Description        string `json:"description"`
	HealthCheckEnabled *bool  `json:"health_check_enabled"`
}

func (s *Server) handleHomeQuickLinks(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, membership domain.HomeMembership, parts []string) bool {
	if len(parts) == 1 && parts[0] == "quick-links" {
		switch r.Method {
		case http.MethodGet:
			s.writeHomeQuickLinks(w, r, home, membership)
			return true
		case http.MethodPost:
			if !canEditHomeQuickLinks(home, membership) {
				http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
				return true
			}
			s.createHomeQuickLink(w, r, home, auth)
			return true
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
	}

	if len(parts) == 2 && parts[0] == "quick-links" && parts[1] == "checks" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		s.checkAndWriteHomeQuickLinks(w, r, home, membership)
		return true
	}

	if len(parts) == 2 && parts[0] == "quick-links" && parts[1] == "order" {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !canEditHomeQuickLinks(home, membership) {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		s.reorderHomeQuickLinks(w, r, home, membership)
		return true
	}

	if len(parts) == 2 && parts[0] == "quick-links" {
		linkID := strings.TrimSpace(parts[1])
		switch r.Method {
		case http.MethodPut:
			if !canEditHomeQuickLinks(home, membership) {
				http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
				return true
			}
			s.updateHomeQuickLink(w, r, home, auth, linkID)
			return true
		case http.MethodDelete:
			if !canEditHomeQuickLinks(home, membership) {
				http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
				return true
			}
			s.deleteHomeQuickLink(w, r, home, linkID)
			return true
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
	}

	return false
}

func (s *Server) writeHomeQuickLinks(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership) {
	links, err := s.store.ListHomeQuickLinks(r.Context(), home.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, quickLinkListResponse(home, links, membership))
}

func (s *Server) createHomeQuickLink(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	var body quickLinkRequest
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	count, err := s.store.CountHomeQuickLinks(r.Context(), home.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if count >= maxHomeQuickLinks {
		http.Error(w, "quick link limit reached", http.StatusBadRequest)
		return
	}

	link, err := normalizedQuickLink(body, domain.HomeQuickLink{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	link.ID = newID("ql")
	link.HomeID = home.ID
	link.SortOrder = count * 10
	link.CreatedAt = now
	link.UpdatedAt = now
	link.UpdatedBy = auth.User.ID
	if err := s.store.CreateHomeQuickLink(r.Context(), link); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	created, err := s.store.GetHomeQuickLink(r.Context(), home.ID, link.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"link": created})
}

func (s *Server) updateHomeQuickLink(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, linkID string) {
	existing, err := s.store.GetHomeQuickLink(r.Context(), home.ID, linkID)
	if err != nil {
		writeStoreHTTPError(w, err)
		return
	}
	var body quickLinkRequest
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated, err := normalizedQuickLink(body, existing)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated.ID = existing.ID
	updated.HomeID = existing.HomeID
	updated.SortOrder = existing.SortOrder
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()
	updated.UpdatedBy = auth.User.ID
	updatedLink, err := s.store.UpdateHomeQuickLink(r.Context(), updated)
	if err != nil {
		writeStoreHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": updatedLink})
}

func (s *Server) deleteHomeQuickLink(w http.ResponseWriter, r *http.Request, home domain.Home, linkID string) {
	if err := s.store.DeleteHomeQuickLink(r.Context(), home.ID, linkID); err != nil {
		writeStoreHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) reorderHomeQuickLinks(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for index, id := range body.IDs {
		body.IDs[index] = strings.TrimSpace(id)
		if body.IDs[index] == "" {
			http.Error(w, "link ids are required", http.StatusBadRequest)
			return
		}
	}
	if err := s.store.ReorderHomeQuickLinks(r.Context(), home.ID, body.IDs); err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeStoreHTTPError(w, err)
		return
	}
	links, err := s.store.ListHomeQuickLinks(r.Context(), home.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, quickLinkListResponse(home, links, membership))
}

func (s *Server) checkAndWriteHomeQuickLinks(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership) {
	links, err := s.store.ListHomeQuickLinks(r.Context(), home.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	links = s.checkHomeQuickLinks(r.Context(), home.ID, links)
	writeJSON(w, http.StatusOK, quickLinkListResponse(home, links, membership))
}

func quickLinkListResponse(home domain.Home, links []domain.HomeQuickLink, membership domain.HomeMembership) map[string]any {
	return map[string]any{
		"links":    links,
		"can_edit": canEditHomeQuickLinks(home, membership),
	}
}

func canEditHomeQuickLinks(home domain.Home, membership domain.HomeMembership) bool {
	role := strings.ToLower(strings.TrimSpace(membership.Role))
	if role == domain.HomeRoleAdmin || role == "owner" {
		return true
	}
	return home.UserID != "" && membership.UserID != "" && home.UserID == membership.UserID
}

func normalizedQuickLink(body quickLinkRequest, existing domain.HomeQuickLink) (domain.HomeQuickLink, error) {
	link := existing
	link.Title = strings.TrimSpace(body.Title)
	link.URL = strings.TrimSpace(body.URL)
	link.Description = strings.TrimSpace(body.Description)
	if len(link.URL) > maxQuickLinkURLLength {
		return domain.HomeQuickLink{}, fmt.Errorf("url is too long")
	}
	parsed, err := url.Parse(link.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return domain.HomeQuickLink{}, fmt.Errorf("valid http or https url is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return domain.HomeQuickLink{}, fmt.Errorf("valid http or https url is required")
	}
	if parsed.User != nil {
		return domain.HomeQuickLink{}, fmt.Errorf("url credentials are not allowed")
	}
	if link.Title == "" {
		link.Title = parsed.Hostname()
	}
	if len(link.Title) > maxQuickLinkTitleLength {
		return domain.HomeQuickLink{}, fmt.Errorf("title is too long")
	}
	if len(link.Description) > maxQuickLinkDescLength {
		return domain.HomeQuickLink{}, fmt.Errorf("description is too long")
	}

	healthEnabled := true
	if body.HealthCheckEnabled != nil {
		healthEnabled = *body.HealthCheckEnabled
	} else if existing.ID != "" {
		healthEnabled = existing.HealthCheckEnabled
	}
	link.HealthCheckEnabled = healthEnabled
	if !healthEnabled {
		link.Status = domain.QuickLinkStatusDisabled
		link.StatusCode = 0
		link.LastCheckedAt = nil
		link.LastError = ""
		return link, nil
	}
	if existing.ID == "" || existing.URL != link.URL || existing.Status == domain.QuickLinkStatusDisabled || link.Status == "" {
		link.Status = domain.QuickLinkStatusUnchecked
		link.StatusCode = 0
		link.LastCheckedAt = nil
		link.LastError = ""
	}
	return link, nil
}

func (s *Server) checkHomeQuickLinks(ctx context.Context, homeID string, links []domain.HomeQuickLink) []domain.HomeQuickLink {
	checked := make([]domain.HomeQuickLink, len(links))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for index, link := range links {
		wg.Add(1)
		go func(index int, link domain.HomeQuickLink) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			checked[index] = s.checkHomeQuickLink(ctx, homeID, link)
		}(index, link)
	}
	wg.Wait()
	return checked
}

func (s *Server) checkHomeQuickLink(ctx context.Context, homeID string, link domain.HomeQuickLink) domain.HomeQuickLink {
	if !link.HealthCheckEnabled {
		updated, err := s.store.UpdateHomeQuickLinkStatus(ctx, homeID, link.ID, domain.QuickLinkStatusDisabled, 0, nil, "")
		if err == nil {
			link = updated
		} else {
			link.Status = domain.QuickLinkStatusDisabled
			link.StatusCode = 0
			link.LastCheckedAt = nil
			link.LastError = ""
		}
		return link
	}
	checkedAt := time.Now().UTC()
	status, statusCode, lastError := s.checkQuickLinkURL(ctx, link.URL)
	updated, err := s.store.UpdateHomeQuickLinkStatus(ctx, homeID, link.ID, status, statusCode, &checkedAt, lastError)
	if err != nil {
		link.Status = domain.QuickLinkStatusDown
		link.StatusCode = 0
		link.LastError = "status could not be saved"
		link.LastCheckedAt = &checkedAt
		return link
	}
	return updated
}

func (s *Server) checkQuickLinkURL(ctx context.Context, rawURL string) (string, int, string) {
	timeout := s.quickLinkCheckTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(checkCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return domain.QuickLinkStatusDown, 0, quickLinkErrorMessage(err)
	}
	request.Header.Set("User-Agent", "Hank Remote Quick Link Check")

	client := s.quickLinkHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return domain.QuickLinkStatusDown, 0, quickLinkErrorMessage(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 512))
	if response.StatusCode < http.StatusInternalServerError {
		return domain.QuickLinkStatusUp, response.StatusCode, ""
	}
	return domain.QuickLinkStatusDown, response.StatusCode, http.StatusText(response.StatusCode)
}

func quickLinkErrorMessage(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "request failed"
	}
	if len(message) > maxQuickLinkErrorLength {
		message = message[:maxQuickLinkErrorLength]
	}
	return message
}

func writeStoreHTTPError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
