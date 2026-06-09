package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

var (
	mediaOptionPattern      = regexp.MustCompile(`(?i)^\s*(?:option\s*)?(\d+)\s*$`)
	mediaQuotedTitlePattern = regexp.MustCompile(`"([^"]+)"`)
)

func mediaAvailabilityQuery(prompt string) (string, bool) {
	trimmed := strings.TrimSpace(prompt)
	lowered := strings.ToLower(trimmed)
	hasDownloadIntent := mediaPromptHasDownloadIntent(lowered)
	hasTypedSearchIntent := hasSearchVerb(lowered) && mediaPromptHasTypeHint(trimmed)
	if !hasDownloadIntent && mediaPromptLooksLikeFileRequest(lowered) {
		return "", false
	}
	if !hasDownloadIntent && !hasTypedSearchIntent {
		return "", false
	}

	if match := mediaQuotedTitlePattern.FindStringSubmatch(trimmed); len(match) == 2 {
		query := cleanQuotedMediaQuery(match[1])
		return query, query != ""
	}

	query := stripMediaPromptPrefix(trimmed)
	query = stripMediaPromptSuffix(query)
	query = cleanMediaPromptQuery(query)
	return query, query != ""
}

func mediaPromptHasDownloadIntent(lowered string) bool {
	if !strings.Contains(lowered, "download") {
		return false
	}
	if mediaPromptLooksLikeFileRequest(lowered) && !mediaPromptHasTypeHint(lowered) {
		return false
	}
	return strings.Contains(lowered, "available") ||
		strings.Contains(lowered, "availabe") ||
		strings.Contains(lowered, "for download") ||
		strings.Contains(lowered, "to download") ||
		strings.HasPrefix(lowered, "can i download ") ||
		strings.HasPrefix(lowered, "can you download ") ||
		strings.HasPrefix(lowered, "download ") ||
		hasSearchVerb(lowered)
}

func mediaPromptLooksLikeFileRequest(lowered string) bool {
	return strings.Contains(lowered, "file") ||
		strings.Contains(lowered, "folder") ||
		strings.Contains(lowered, "directory") ||
		strings.Contains(lowered, "smb") ||
		strings.Contains(lowered, "share") ||
		strings.Contains(lowered, "document") ||
		strings.Contains(lowered, "pdf") ||
		strings.Contains(lowered, " tax") ||
		strings.Contains(lowered, "tax ") ||
		strings.Contains(lowered, "taxes")
}

func mediaPromptHasTypeHint(prompt string) bool {
	for _, token := range strings.Fields(normalizedMediaSelection(prompt)) {
		if isMediaTypeHint(token) {
			return true
		}
	}
	return false
}

func isMediaTypeHint(token string) bool {
	switch token {
	case "movie", "movies", "film", "films", "series", "show", "shows", "tv", "season", "seasons", "episode", "episodes":
		return true
	default:
		return false
	}
}

func stripMediaPromptPrefix(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	lowered := strings.ToLower(trimmed)
	prefixes := []string{
		"can you download ",
		"can i download ",
		"please search for ",
		"please search ",
		"please find ",
		"search for ",
		"look for ",
		"find all ",
		"search ",
		"find ",
		"is ",
		"are ",
		"download ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowered, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return stripAssistantSearchPrefix(trimmed)
}

func stripMediaPromptSuffix(query string) string {
	suffixes := []string{
		" available for download",
		" availabe for download",
		" available to download",
		" availabe to download",
		" for download",
		" to download",
		" available",
		" availabe",
		" download",
	}
	for {
		trimmed := strings.TrimSpace(query)
		lowered := strings.ToLower(trimmed)
		changed := false
		for _, suffix := range suffixes {
			if strings.HasSuffix(lowered, suffix) {
				query = strings.TrimSpace(trimmed[:len(trimmed)-len(suffix)])
				changed = true
				break
			}
		}
		if !changed {
			return trimmed
		}
	}
}

func cleanQuotedMediaQuery(query string) string {
	return strings.TrimSpace(query)
}

func cleanMediaPromptQuery(query string) string {
	query = strings.TrimSpace(query)
	query = strings.Trim(query, " \t\r\n\"'`")
	for {
		lowered := strings.ToLower(query)
		if strings.HasPrefix(lowered, "for ") {
			query = strings.TrimSpace(query[len("for "):])
			continue
		}
		if strings.HasPrefix(lowered, "called ") {
			query = strings.TrimSpace(query[len("called "):])
			continue
		}
		if strings.HasPrefix(lowered, "named ") {
			query = strings.TrimSpace(query[len("named "):])
			continue
		}
		break
	}
	if mediaQueryEndsWithTypeHint(query) {
		query = stripLeadingMediaArticle(query)
	}
	return query
}

func mediaQueryEndsWithTypeHint(query string) bool {
	tokens := strings.Fields(normalizedMediaSelection(query))
	if len(tokens) == 0 {
		return false
	}
	return isMediaTypeHint(tokens[len(tokens)-1])
}

func stripLeadingMediaArticle(query string) string {
	trimmed := strings.TrimSpace(query)
	lowered := strings.ToLower(trimmed)
	for _, prefix := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lowered, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return trimmed
}

func (s *Server) answerMediaSearch(ctx context.Context, home domain.Home, query string) (assistantMessageContent, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "warn",
			Scope:   "media",
			Event:   "media.search.empty_query",
			Summary: "Media workflow matched, but no usable title was parsed.",
		})
		return assistantMessageContent{Text: "Tell me the movie or show name to search for download availability."}, nil
	}
	s.recordAssistantTrace(ctx, assistantTraceEvent{
		Scope:   "media",
		Event:   "media.search.start",
		Summary: "Starting media search through the home agent.",
		Details: traceDetails(map[string]any{
			"query": query,
			"limit": 10,
		}),
	})
	envelope, err := s.sendMediaCommand(ctx, home.ID, protocol.CommandMediaSearch, protocol.MediaSearchRequest{Query: query, Limit: 10})
	if err != nil || envelope.Error != nil {
		errorMessage := ""
		if err != nil {
			errorMessage = err.Error()
		} else if envelope.Error != nil {
			errorMessage = envelope.Error.Message
		}
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "media",
			Event:   "media.search.failed",
			Summary: "Media search could not reach or complete through the home agent.",
			Details: traceDetails(map[string]any{
				"query": query,
				"error": errorMessage,
			}),
		})
		return assistantMessageContent{Text: "I couldn't reach the media source through the home agent right now."}, nil
	}
	payload, err := protocol.DecodePayload[protocol.MediaSearchResponse](envelope)
	if err != nil {
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "media",
			Event:   "media.search.decode_failed",
			Summary: "Media search response could not be decoded.",
			Details: traceDetails(map[string]any{
				"query": query,
				"error": err.Error(),
			}),
		})
		return assistantMessageContent{}, err
	}
	s.recordAssistantTrace(ctx, assistantTraceEvent{
		Scope:   "media",
		Event:   "media.search.results",
		Summary: "Media search returned results.",
		Details: traceDetails(map[string]any{
			"query":        query,
			"result_count": len(payload.Results),
		}),
	})
	if len(payload.Results) == 0 {
		return assistantMessageContent{Text: fmt.Sprintf("I couldn't find `%s` available for download.", query)}, nil
	}
	if len(payload.Results) == 1 {
		card := assistantMediaCard(payload.Results[0], 1)
		return s.answerMediaSelection(ctx, home, card)
	}

	cards := make([]assistantResultCard, 0, len(payload.Results))
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("I found %d media matches for `%s`. Choose one of the options below:", len(payload.Results), query))
	for index, result := range payload.Results {
		card := assistantMediaCard(result, index+1)
		cards = append(cards, card)
		builder.WriteString(fmt.Sprintf("\n%s", mediaCardLine(card)))
	}
	return assistantMessageContent{Text: builder.String(), Cards: cards}, nil
}

func (s *Server) answerMediaSelection(ctx context.Context, home domain.Home, card assistantResultCard) (assistantMessageContent, error) {
	s.recordAssistantTrace(ctx, assistantTraceEvent{
		Scope:   "media",
		Event:   "media.selection.start",
		Summary: "Preparing the selected media option.",
		Details: traceDetails(map[string]any{
			"title": card.Title,
			"path":  card.Path,
			"type":  card.MediaType,
		}),
	})
	selection := protocol.MediaSearchResult{
		ID:         card.MediaOptionID,
		Title:      card.Title,
		Year:       card.Year,
		Type:       card.MediaType,
		Summary:    card.Summary,
		PosterURL:  card.ImageURL,
		PagePath:   card.Path,
		SearchText: card.SearchText,
	}
	plan, err := s.planMediaDownload(ctx, home.ID, selection)
	if err != nil {
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "media",
			Event:   "media.plan.failed",
			Summary: "Could not prepare the media download plan.",
			Details: traceDetails(map[string]any{
				"title": card.Title,
				"path":  card.Path,
				"error": err.Error(),
			}),
		})
		return assistantMessageContent{Text: fmt.Sprintf("I found `%s`, but I couldn't prepare the download plan through the home agent right now.", card.Title)}, nil
	}
	s.recordAssistantTrace(ctx, assistantTraceEvent{
		Scope:   "media",
		Event:   "media.plan.prepared",
		Summary: "Prepared the media download plan.",
		Details: traceDetails(map[string]any{
			"title":                   firstNonBlank(plan.Selection.Title, card.Title),
			"type":                    plan.Selection.Type,
			"items":                   plan.ItemCount,
			"preferred_quality_count": plan.PreferredQualityCount,
			"fallback_quality_count":  plan.FallbackQualityCount,
			"missing_link_count":      plan.MissingLinkCount,
			"existing_count":          plan.ExistingCount,
			"destination":             plan.DestinationPath,
		}),
	})
	if plan.ItemCount == 0 || plan.MissingLinkCount >= plan.ItemCount {
		return assistantMessageContent{Text: fmt.Sprintf("I found `%s`, but no downloadable movie or episode entries were available.", card.Title)}, nil
	}
	if !mediaPlanRequiresConfirmation(plan) {
		response, err := s.startMediaDownload(ctx, home.ID, plan.Selection)
		if err != nil {
			s.recordAssistantTrace(ctx, assistantTraceEvent{
				Level:   "error",
				Scope:   "media",
				Event:   "media.download.failed",
				Summary: "Could not start the media download after planning.",
				Details: traceDetails(map[string]any{
					"title": firstNonBlank(plan.Selection.Title, card.Title),
					"path":  card.Path,
					"error": err.Error(),
				}),
			})
			return assistantMessageContent{Text: fmt.Sprintf("I found `%s`, but I couldn't start the download through the home agent right now.", card.Title)}, nil
		}
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Scope:   "media",
			Event:   "media.confirmation.skipped",
			Summary: "Media workflow is configured to start downloads without an approval pause.",
			Details: traceDetails(map[string]any{
				"title":       firstNonBlank(plan.Selection.Title, card.Title),
				"destination": plan.DestinationPath,
				"job_id":      response.Job.JobID,
			}),
		})
		return mediaDownloadStartedContent(firstNonBlank(plan.Selection.Title, card.Title), plan.Selection, response.Job, plan.DestinationPath), nil
	}
	pending := assistantPendingAction{
		Kind: "media_download",
		MediaDownload: &assistantPendingMediaDownload{
			Selection:             plan.Selection,
			Title:                 firstNonBlank(plan.Selection.Title, card.Title),
			MediaType:             plan.Selection.Type,
			ItemCount:             plan.ItemCount,
			PreferredQualityCount: plan.PreferredQualityCount,
			FallbackQualityCount:  plan.FallbackQualityCount,
			MissingLinkCount:      plan.MissingLinkCount,
			ExistingCount:         plan.ExistingCount,
			DestinationPath:       plan.DestinationPath,
			Confirmation:          mediaConfirmationText(plan),
		},
	}
	return assistantMessageContent{
		Text:  mediaPlanText(plan),
		Cards: []assistantResultCard{card},
		Meta: map[string]interface{}{
			"pending_action": pending,
		},
	}, nil
}

func mediaPlanRequiresConfirmation(plan protocol.MediaDownloadPlan) bool {
	if plan.RequireConfirmation == nil {
		return true
	}
	return *plan.RequireConfirmation
}

func (s *Server) planMediaDownload(ctx context.Context, homeID string, selection protocol.MediaSearchResult) (protocol.MediaDownloadPlan, error) {
	envelope, err := s.sendMediaCommand(ctx, homeID, protocol.CommandMediaPlanDownload, protocol.MediaPlanDownloadRequest{Selection: selection})
	if err != nil {
		return protocol.MediaDownloadPlan{}, err
	}
	if envelope.Error != nil {
		return protocol.MediaDownloadPlan{}, errors.New(envelope.Error.Message)
	}
	payload, err := protocol.DecodePayload[protocol.MediaPlanDownloadResponse](envelope)
	if err != nil {
		return protocol.MediaDownloadPlan{}, err
	}
	return payload.Plan, nil
}

func (s *Server) startMediaDownload(ctx context.Context, homeID string, selection protocol.MediaSearchResult) (protocol.MediaDownloadStartResponse, error) {
	envelope, err := s.sendMediaCommand(ctx, homeID, protocol.CommandMediaDownloadStart, protocol.MediaDownloadStartRequest{Selection: selection})
	if err != nil {
		return protocol.MediaDownloadStartResponse{}, err
	}
	if envelope.Error != nil {
		return protocol.MediaDownloadStartResponse{}, errors.New(envelope.Error.Message)
	}
	response, err := protocol.DecodePayload[protocol.MediaDownloadStartResponse](envelope)
	if err != nil {
		return protocol.MediaDownloadStartResponse{}, err
	}
	s.recordAssistantTrace(ctx, assistantTraceEvent{
		Scope:   "media",
		Event:   "media.download.started",
		Summary: "Home agent started the media download job.",
		Details: traceDetails(map[string]any{
			"job_id": response.Job.JobID,
			"title":  response.Job.Title,
			"status": response.Job.Status,
			"items":  response.Job.TotalCount,
		}),
	})
	return response, nil
}

func mediaDownloadStartedContent(title string, selection protocol.MediaSearchResult, job protocol.MediaDownloadJobStatus, destination string) assistantMessageContent {
	title = firstNonBlank(title, job.Title, selection.Title, "selected media")
	destination = firstNonBlank(destination, "Media destination")
	return assistantMessageContent{
		Text: fmt.Sprintf("Started the media download job for `%s`.", title),
		Cards: []assistantResultCard{
			{
				Kind:        "media",
				Title:       title,
				Summary:     fmt.Sprintf("Job %s is %s. %d item(s) queued for %s.", job.JobID, job.Status, job.TotalCount, destination),
				ActionTitle: "View Job",
				Path:        selection.PagePath,
				MediaType:   selection.Type,
				ImageURL:    selection.PosterURL,
				JobID:       job.JobID,
			},
		},
	}
}

func (s *Server) resolvePreviousMediaSelection(ctx context.Context, sessionID string, prompt string) (assistantResultCard, bool) {
	cards := s.latestMediaCards(ctx, sessionID)
	if len(cards) == 0 {
		return assistantResultCard{}, false
	}
	if match := mediaOptionPattern.FindStringSubmatch(prompt); len(match) == 2 {
		option, _ := strconv.Atoi(match[1])
		if option > 0 && option <= len(cards) {
			return cards[option-1], true
		}
	}
	promptKey := normalizedMediaSelection(prompt)
	if promptKey == "" {
		return assistantResultCard{}, false
	}
	for _, card := range cards {
		if normalizedMediaSelection(card.Title) == promptKey {
			return card, true
		}
	}
	for _, card := range cards {
		if strings.Contains(normalizedMediaSelection(card.Title), promptKey) || strings.Contains(promptKey, normalizedMediaSelection(card.Title)) {
			return card, true
		}
	}
	return assistantResultCard{}, false
}

func (s *Server) latestMediaCards(ctx context.Context, sessionID string) []assistantResultCard {
	messages, err := s.store.ListAssistantMessages(ctx, sessionID)
	if err != nil {
		return nil
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != assistantRoleAssistant {
			continue
		}
		var content assistantMessageContent
		if err := json.Unmarshal([]byte(messages[i].ContentJSON), &content); err != nil {
			continue
		}
		var cards []assistantResultCard
		for _, card := range content.Cards {
			if card.Kind == "media" && card.Path != "" && card.JobID == "" {
				cards = append(cards, card)
			}
		}
		if len(cards) > 0 {
			return cards
		}
	}
	return nil
}

func assistantMediaCard(result protocol.MediaSearchResult, _ int) assistantResultCard {
	title := result.Title
	if result.Year > 0 {
		title = fmt.Sprintf("%s (%d)", result.Title, result.Year)
	}
	summary := mediaResultSummary(result)
	return assistantResultCard{
		Kind:          "media",
		Title:         title,
		Summary:       summary,
		ActionTitle:   "Choose",
		Path:          result.PagePath,
		SearchText:    result.SearchText,
		ImageURL:      result.PosterURL,
		MediaOptionID: result.ID,
		MediaType:     result.Type,
		Year:          result.Year,
	}
}

func mediaResultSummary(result protocol.MediaSearchResult) string {
	parts := []string{}
	switch result.Type {
	case protocol.MediaTypeMovie:
		parts = append(parts, "Movie")
	case protocol.MediaTypeSeries:
		parts = append(parts, "TV show")
	}
	if result.Summary != "" {
		parts = append(parts, result.Summary)
	}
	if result.Rating != "" {
		parts = append(parts, "Rating "+result.Rating)
	}
	return strings.Join(parts, " | ")
}

func mediaCardLine(card assistantResultCard) string {
	summary := strings.TrimSpace(card.Summary)
	if summary == "" {
		return card.Title
	}
	return fmt.Sprintf("%s - %s", card.Title, summary)
}

func mediaPlanText(plan protocol.MediaDownloadPlan) string {
	title := firstNonBlank(plan.Selection.Title, "Selected media")
	if plan.Selection.Type == protocol.MediaTypeSeries {
		return fmt.Sprintf("I found `%s` and prepared %d available episode(s) for download. %d will use 1080p and %d will fall back to 720p. Confirm before I start the batch.", title, plan.ItemCount, plan.PreferredQualityCount, plan.FallbackQualityCount)
	}
	return fmt.Sprintf("I found `%s` and prepared it for download. Confirm before I start the download.", title)
}

func mediaConfirmationText(plan protocol.MediaDownloadPlan) string {
	title := firstNonBlank(plan.Selection.Title, "selected media")
	if plan.Selection.Type == protocol.MediaTypeSeries {
		return fmt.Sprintf("Confirm downloading %d episode(s) of `%s` to the Media share root.", plan.ItemCount, title)
	}
	return fmt.Sprintf("Confirm downloading `%s` to the Media share root.", title)
}

func normalizedMediaSelection(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "option ")
	var builder strings.Builder
	lastSpace := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(builder.String())
}
