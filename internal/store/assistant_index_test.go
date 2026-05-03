package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

func TestAssistantIndexSearchRanksDocumentsAndFiles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_assistant_index", Email: "assistant-index@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_assistant_index", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.CreateHome(ctx, home); err != nil {
		t.Fatalf("CreateHome: %v", err)
	}

	embeddingJSON, _ := json.Marshal([]float64{1, 0, 0, 0})
	userID := user.ID
	document := domain.AssistantDocument{
		ID:           "adoc_store_list",
		HomeID:       home.ID,
		UserID:       &userID,
		SourceType:   "profile_note",
		SourceID:     "note_internal_store",
		SourceKey:    "profile_note:" + user.ID + ":note_internal_store",
		Title:        "Store List",
		Path:         "store-list",
		CanonicalURI: "hank://notes/store-list",
		MetadataJSON: `{}`,
		SearchText:   "Store List dog food milk",
		UpdatedAt:    now,
	}
	if err := db.UpsertAssistantDocumentWithChunks(ctx, document, []domain.AssistantChunk{
		{
			ID:               "achunk_store_list_0",
			DocumentID:       document.ID,
			ChunkIndex:       0,
			Content:          "Dog food is on the store list.",
			TokenCount:       7,
			EmbeddingJSON:    string(embeddingJSON),
			EmbeddingModel:   "test",
			EmbeddingVersion: "v1",
			UpdatedAt:        now,
		},
	}); err != nil {
		t.Fatalf("UpsertAssistantDocumentWithChunks: %v", err)
	}

	if err := db.UpsertAssistantFileIndex(ctx, domain.AssistantFileIndex{
		ID:               "afile_tax_2025",
		HomeID:           home.ID,
		Path:             "Documents/Taxes/2025",
		Name:             "2025",
		IsDirectory:      true,
		SearchText:       "Documents Taxes 2025",
		MetadataJSON:     `{}`,
		EmbeddingJSON:    string(embeddingJSON),
		EmbeddingModel:   "test",
		EmbeddingVersion: "v1",
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("UpsertAssistantFileIndex: %v", err)
	}

	results, err := db.SearchAssistantContext(ctx, home.ID, user.ID, "dog food store", []float64{1, 0, 0, 0}, 5)
	if err != nil {
		t.Fatalf("SearchAssistantContext: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected note search results")
	}
	if results[0].SourceType != "profile_note" || results[0].Title != "Store List" {
		t.Fatalf("top result = %#v, want Store List profile note", results[0])
	}

	results, err = db.SearchAssistantContext(ctx, home.ID, user.ID, "tax documents 2025", nil, 5)
	if err != nil {
		t.Fatalf("SearchAssistantContext files: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected file search results")
	}
	if results[0].SourceType != "file" || results[0].Path != "Documents/Taxes/2025" {
		t.Fatalf("top file result = %#v, want tax folder", results[0])
	}
}
