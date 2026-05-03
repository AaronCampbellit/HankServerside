package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/dropfile/hankremote/internal/domain"
)

func (s *Store) UpsertAssistantDocumentWithChunks(ctx context.Context, document domain.AssistantDocument, chunks []domain.AssistantChunk) error {
	tx, err := s.beginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if document.MetadataJSON == "" {
		document.MetadataJSON = "{}"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO assistant_documents (
			id, home_id, user_id, source_type, source_id, source_key, title, path,
			canonical_uri, metadata_json, search_text, embedding_model, embedding_version, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_key) DO UPDATE SET
			home_id = excluded.home_id,
			user_id = excluded.user_id,
			source_type = excluded.source_type,
			source_id = excluded.source_id,
			title = excluded.title,
			path = excluded.path,
			canonical_uri = excluded.canonical_uri,
			metadata_json = excluded.metadata_json,
			search_text = excluded.search_text,
			embedding_model = excluded.embedding_model,
			embedding_version = excluded.embedding_version,
			updated_at = excluded.updated_at`,
		document.ID,
		document.HomeID,
		document.UserID,
		document.SourceType,
		document.SourceID,
		document.SourceKey,
		document.Title,
		document.Path,
		document.CanonicalURI,
		document.MetadataJSON,
		document.SearchText,
		document.EmbeddingModel,
		document.EmbeddingVersion,
		document.UpdatedAt,
	); err != nil {
		return err
	}

	row := tx.QueryRowContext(ctx, `SELECT id FROM assistant_documents WHERE source_key = ?`, document.SourceKey)
	var documentID string
	if err := row.Scan(&documentID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM assistant_chunks WHERE document_id = ?`, documentID); err != nil {
		return err
	}
	for _, chunk := range chunks {
		chunk.DocumentID = documentID
		if err := s.insertAssistantChunk(ctx, tx, chunk); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) UpsertAssistantFileIndex(ctx context.Context, item domain.AssistantFileIndex) error {
	if item.MetadataJSON == "" {
		item.MetadataJSON = "{}"
	}
	if s.vectorAvailable && vectorLiteralFromJSON(item.EmbeddingJSON) != "" {
		_, err := s.exec(ctx, `INSERT INTO assistant_file_index (
				id, home_id, service_profile_id, path, name, is_directory, size_bytes, modified_at,
				search_text, metadata_json, embedding_json, embedding_model, embedding_version, updated_at, embedding
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::vector)
			ON CONFLICT(home_id, path) DO UPDATE SET
				service_profile_id = excluded.service_profile_id,
				name = excluded.name,
				is_directory = excluded.is_directory,
				size_bytes = excluded.size_bytes,
				modified_at = excluded.modified_at,
				search_text = excluded.search_text,
				metadata_json = excluded.metadata_json,
				embedding_json = excluded.embedding_json,
				embedding_model = excluded.embedding_model,
				embedding_version = excluded.embedding_version,
				updated_at = excluded.updated_at,
				embedding = excluded.embedding`,
			item.ID, item.HomeID, item.ServiceProfileID, item.Path, item.Name, item.IsDirectory, item.SizeBytes, item.ModifiedAt,
			item.SearchText, item.MetadataJSON, item.EmbeddingJSON, item.EmbeddingModel, item.EmbeddingVersion, item.UpdatedAt,
			vectorLiteralFromJSON(item.EmbeddingJSON),
		)
		return err
	}
	_, err := s.exec(ctx, `INSERT INTO assistant_file_index (
			id, home_id, service_profile_id, path, name, is_directory, size_bytes, modified_at,
			search_text, metadata_json, embedding_json, embedding_model, embedding_version, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(home_id, path) DO UPDATE SET
			service_profile_id = excluded.service_profile_id,
			name = excluded.name,
			is_directory = excluded.is_directory,
			size_bytes = excluded.size_bytes,
			modified_at = excluded.modified_at,
			search_text = excluded.search_text,
			metadata_json = excluded.metadata_json,
			embedding_json = excluded.embedding_json,
			embedding_model = excluded.embedding_model,
			embedding_version = excluded.embedding_version,
			updated_at = excluded.updated_at`,
		item.ID, item.HomeID, item.ServiceProfileID, item.Path, item.Name, item.IsDirectory, item.SizeBytes, item.ModifiedAt,
		item.SearchText, item.MetadataJSON, item.EmbeddingJSON, item.EmbeddingModel, item.EmbeddingVersion, item.UpdatedAt,
	)
	return err
}

func (s *Store) SearchAssistantContext(ctx context.Context, homeID string, userID string, query string, queryEmbedding []float64, limit int) ([]domain.AssistantRetrievedContext, error) {
	if limit <= 0 {
		limit = 8
	}
	query = strings.TrimSpace(query)
	loweredQuery := strings.ToLower(query)
	terms := strings.Fields(loweredQuery)

	rows, err := s.query(ctx, `SELECT d.source_type, d.source_id, d.title, d.path, d.canonical_uri,
			d.search_text, d.updated_at, c.content, c.embedding_json
		FROM assistant_documents d
		LEFT JOIN assistant_chunks c ON c.document_id = d.id
		WHERE d.home_id = ? AND (d.user_id IS NULL OR d.user_id = ?)
		ORDER BY d.updated_at DESC, c.chunk_index ASC
		LIMIT 500`, homeID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]domain.AssistantRetrievedContext, 0)
	for rows.Next() {
		item, err := scanAssistantContextRow(rows, loweredQuery, terms, queryEmbedding)
		if err != nil {
			return nil, err
		}
		if item.Score > 0 {
			results = append(results, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	fileRows, err := s.query(ctx, `SELECT path, name, search_text, updated_at, embedding_json
		FROM assistant_file_index
		WHERE home_id = ?
		ORDER BY updated_at DESC
		LIMIT 300`, homeID)
	if err != nil {
		return nil, err
	}
	defer fileRows.Close()
	for fileRows.Next() {
		var pathValue, name, searchText, embeddingJSON string
		var updatedAt sql.NullTime
		if err := fileRows.Scan(&pathValue, &name, &searchText, &updatedAt, &embeddingJSON); err != nil {
			return nil, err
		}
		score := textScore(loweredQuery, terms, strings.ToLower(name+" "+pathValue+" "+searchText))
		score += embeddingScore(queryEmbedding, embeddingJSON)
		if score <= 0 {
			continue
		}
		results = append(results, domain.AssistantRetrievedContext{
			SourceType:   "file",
			SourceID:     pathValue,
			Title:        name,
			Path:         pathValue,
			CanonicalURI: "hank://files/" + strings.TrimPrefix(pathValue, "/"),
			Snippet:      pathValue,
			Score:        score,
			UpdatedAt:    updatedAt.Time,
		})
	}
	if err := fileRows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].UpdatedAt.After(results[j].UpdatedAt)
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *Store) insertAssistantChunk(ctx context.Context, tx *dbTx, chunk domain.AssistantChunk) error {
	if s.vectorAvailable && vectorLiteralFromJSON(chunk.EmbeddingJSON) != "" {
		_, err := tx.ExecContext(ctx, `INSERT INTO assistant_chunks (
				id, document_id, chunk_index, content, token_count, embedding_json,
				embedding_model, embedding_version, updated_at, embedding
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?::vector)`,
			chunk.ID, chunk.DocumentID, chunk.ChunkIndex, chunk.Content, chunk.TokenCount, chunk.EmbeddingJSON,
			chunk.EmbeddingModel, chunk.EmbeddingVersion, chunk.UpdatedAt, vectorLiteralFromJSON(chunk.EmbeddingJSON),
		)
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO assistant_chunks (
			id, document_id, chunk_index, content, token_count, embedding_json,
			embedding_model, embedding_version, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		chunk.ID, chunk.DocumentID, chunk.ChunkIndex, chunk.Content, chunk.TokenCount, chunk.EmbeddingJSON,
		chunk.EmbeddingModel, chunk.EmbeddingVersion, chunk.UpdatedAt,
	)
	return err
}

func scanAssistantContextRow(scanner interface{ Scan(dest ...any) error }, loweredQuery string, terms []string, queryEmbedding []float64) (domain.AssistantRetrievedContext, error) {
	var sourceType, sourceID, title, pathValue, canonicalURI, searchText string
	var updatedAt sql.NullTime
	var content, embeddingJSON sql.NullString
	if err := scanner.Scan(&sourceType, &sourceID, &title, &pathValue, &canonicalURI, &searchText, &updatedAt, &content, &embeddingJSON); err != nil {
		return domain.AssistantRetrievedContext{}, err
	}
	snippet := content.String
	if strings.TrimSpace(snippet) == "" {
		snippet = searchText
	}
	score := textScore(loweredQuery, terms, strings.ToLower(title+" "+pathValue+" "+searchText+" "+snippet))
	score += embeddingScore(queryEmbedding, embeddingJSON.String)
	return domain.AssistantRetrievedContext{
		SourceType:   sourceType,
		SourceID:     sourceID,
		Title:        title,
		Path:         pathValue,
		CanonicalURI: canonicalURI,
		Snippet:      trimSnippet(snippet, 220),
		Score:        score,
		UpdatedAt:    updatedAt.Time,
	}, nil
}

func textScore(query string, terms []string, haystack string) float64 {
	if query == "" {
		return 0
	}
	score := 0.0
	if strings.Contains(haystack, query) {
		score += 10
	}
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			score += 2
		}
	}
	return score
}

func embeddingScore(queryEmbedding []float64, raw string) float64 {
	if len(queryEmbedding) == 0 || raw == "" {
		return 0
	}
	var embedding []float64
	if err := json.Unmarshal([]byte(raw), &embedding); err != nil || len(embedding) == 0 {
		return 0
	}
	score := cosineSimilarity(queryEmbedding, embedding)
	if score <= 0 {
		return 0
	}
	return score * 6
}

func cosineSimilarity(a []float64, b []float64) float64 {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	if limit == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := 0; i < limit; i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func vectorLiteralFromJSON(raw string) string {
	if raw == "" {
		return ""
	}
	var values []float64
	if err := json.Unmarshal([]byte(raw), &values); err != nil || len(values) != 768 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(value, 'g', -1, 64))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func trimSnippet(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "..."
}
