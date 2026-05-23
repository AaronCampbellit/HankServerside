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

var mediaOptionPattern = regexp.MustCompile(`(?i)^\s*(?:option\s*)?(\d+)\s*$`)

func mediaAvailabilityQuery(prompt string) (string, bool) {
	trimmed := strings.TrimSpace(prompt)
	lowered := strings.ToLower(trimmed)
	if !strings.Contains(lowered, "download") {
		return "", false
	}
	if strings.Contains(lowered, "available for download") || strings.Contains(lowered, "availabe for download") {
		query := trimmed
		for _, marker := range []string{" available for download", " availabe for download", " Available for download", " Availabe for download"} {
			query = strings.Replace(query, marker, "", 1)
		}
		query = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(query), "is "))
		query = strings.TrimSpace(strings.TrimPrefix(query, "the "))
		return query, query != ""
	}
	for _, prefix := range []string{"can i download ", "find ", "search "} {
		if strings.HasPrefix(lowered, prefix) {
			query := strings.TrimSpace(trimmed[len(prefix):])
			query = strings.TrimSpace(strings.TrimSuffix(query, "for download"))
			query = strings.TrimSpace(strings.TrimSuffix(query, "download"))
			return query, query != ""
		}
	}
	return "", false
}

func (s *Server) answerMediaSearch(ctx context.Context, home domain.Home, query string) (assistantMessageContent, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return assistantMessageContent{Text: "Tell me the movie or show name to search for download availability."}, nil
	}
	envelope, err := s.sendAgentCommand(ctx, home.ID, protocol.CommandMediaSearch, protocol.MediaSearchRequest{Query: query, Limit: 10})
	if err != nil || envelope.Error != nil {
		return assistantMessageContent{Text: "I couldn't reach the media source through the home agent right now."}, nil
	}
	payload, err := protocol.DecodePayload[protocol.MediaSearchResponse](envelope)
	if err != nil {
		return assistantMessageContent{}, err
	}
	if len(payload.Results) == 0 {
		return assistantMessageContent{Text: fmt.Sprintf("I couldn't find `%s` available for download.", query)}, nil
	}
	if len(payload.Results) == 1 {
		card := assistantMediaCard(payload.Results[0], 1)
		return s.answerMediaSelection(ctx, home, card)
	}

	cards := make([]assistantResultCard, 0, len(payload.Results))
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("I found %d media matches for `%s`. Reply with the option number or title:", len(payload.Results), query))
	for index, result := range payload.Results {
		card := assistantMediaCard(result, index+1)
		cards = append(cards, card)
		builder.WriteString(fmt.Sprintf("\n%d. %s", index+1, mediaCardLine(card)))
	}
	return assistantMessageContent{Text: builder.String(), Cards: cards}, nil
}

func (s *Server) answerMediaSelection(ctx context.Context, home domain.Home, card assistantResultCard) (assistantMessageContent, error) {
	selection := protocol.MediaSearchResult{
		ID:         card.MediaOptionID,
		Title:      card.Title,
		Year:       card.Year,
		Type:       card.MediaType,
		Summary:    card.Summary,
		PagePath:   card.Path,
		SearchText: card.SearchText,
	}
	plan, err := s.planMediaDownload(ctx, home.ID, selection)
	if err != nil {
		return assistantMessageContent{Text: fmt.Sprintf("I found `%s`, but I couldn't prepare the download plan through the home agent right now.", card.Title)}, nil
	}
	if plan.ItemCount == 0 || plan.MissingLinkCount >= plan.ItemCount {
		return assistantMessageContent{Text: fmt.Sprintf("I found `%s`, but no downloadable movie or episode entries were available.", card.Title)}, nil
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

func (s *Server) planMediaDownload(ctx context.Context, homeID string, selection protocol.MediaSearchResult) (protocol.MediaDownloadPlan, error) {
	envelope, err := s.sendAgentCommand(ctx, homeID, protocol.CommandMediaPlanDownload, protocol.MediaPlanDownloadRequest{Selection: selection})
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
	envelope, err := s.sendAgentCommand(ctx, homeID, protocol.CommandMediaDownloadStart, protocol.MediaDownloadStartRequest{Selection: selection})
	if err != nil {
		return protocol.MediaDownloadStartResponse{}, err
	}
	if envelope.Error != nil {
		return protocol.MediaDownloadStartResponse{}, errors.New(envelope.Error.Message)
	}
	return protocol.DecodePayload[protocol.MediaDownloadStartResponse](envelope)
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

func assistantMediaCard(result protocol.MediaSearchResult, option int) assistantResultCard {
	title := result.Title
	if result.Year > 0 {
		title = fmt.Sprintf("%s (%d)", result.Title, result.Year)
	}
	summary := mediaResultSummary(result)
	return assistantResultCard{
		Kind:          "media",
		Title:         title,
		Summary:       summary,
		ActionTitle:   fmt.Sprintf("Reply %d to choose", option),
		Path:          result.PagePath,
		SearchText:    result.SearchText,
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
