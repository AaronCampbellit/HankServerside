package protocol

const (
	CommandHermesChat = "hermes.chat"
)

type HermesChatRequest struct {
	Prompt         string `json:"prompt"`
	ConversationID string `json:"conversation_id,omitempty"`
	SessionKey     string `json:"session_key,omitempty"`
}

type HermesChatResponse struct {
	Text           string `json:"text"`
	Model          string `json:"model,omitempty"`
	ResponseID     string `json:"response_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}
