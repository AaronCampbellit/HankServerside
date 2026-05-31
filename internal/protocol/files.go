package protocol

import "time"

const (
	FileTransferOperationDownload = "download"
	FileTransferOperationUpload   = "upload"
)

const (
	FileOperationMove     = "move"
	FileOperationCopy     = "copy"
	FileOperationDelete   = "delete"
	FileOperationUpload   = "upload"
	FileOperationDownload = "download"
)

type FileItem struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	IsDirectory bool      `json:"is_directory"`
	Size        int64     `json:"size"`
	ModifiedAt  time.Time `json:"modified_at"`
}

type FilesListRequest struct {
	SourceID string `json:"source_id,omitempty"`
	Path     string `json:"path"`
}

type FilesListResponse struct {
	Items []FileItem `json:"items"`
}

type FilesStatRequest struct {
	SourceID string `json:"source_id,omitempty"`
	Path     string `json:"path"`
}

type FilesStatResponse struct {
	Item FileItem `json:"item"`
}

type FilesSearchRequest struct {
	SourceID string `json:"source_id,omitempty"`
	Query    string `json:"query"`
	Limit    int    `json:"limit,omitempty"`
}

type FilesSearchResponse struct {
	Items []FileItem `json:"items"`
}

type FilesCreateDirectoryRequest struct {
	SourceID string `json:"source_id,omitempty"`
	Path     string `json:"path"`
}

type FilesRenameRequest struct {
	SourceID string `json:"source_id,omitempty"`
	From     string `json:"from"`
	To       string `json:"to"`
}

type FilesMoveRequest struct {
	SourceID            string `json:"source_id,omitempty"`
	DestinationSourceID string `json:"destination_source_id,omitempty"`
	JobID               string `json:"job_id,omitempty"`
	From                string `json:"from"`
	To                  string `json:"to"`
	IsDirectory         bool   `json:"is_directory"`
}

type FileOperationJobResponse struct {
	OK         bool   `json:"ok"`
	JobID      string `json:"job_id,omitempty"`
	Status     string `json:"status,omitempty"`
	BytesTotal int64  `json:"bytes_total,omitempty"`
	BytesDone  int64  `json:"bytes_done,omitempty"`
	FilesTotal int64  `json:"files_total,omitempty"`
	FilesDone  int64  `json:"files_done,omitempty"`
}

type FilesDeleteRequest struct {
	SourceID    string `json:"source_id,omitempty"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"is_directory"`
}

type FilesDownloadRequest struct {
	SourceID string `json:"source_id,omitempty"`
	Path     string `json:"path"`
}

type FilesDownloadResponse struct {
	Path          string `json:"path"`
	ContentBase64 string `json:"content_base64"`
}

type FilesUploadRequest struct {
	SourceID      string `json:"source_id,omitempty"`
	Path          string `json:"path"`
	ContentBase64 string `json:"content_base64"`
}

type FileTransferOpen struct {
	Operation string `json:"operation"`
	SourceID  string `json:"source_id,omitempty"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
}

type FileTransferReady struct {
	Operation string `json:"operation"`
	SourceID  string `json:"source_id,omitempty"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

type FileTransferChunk struct {
	Offset        int64  `json:"offset,omitempty"`
	ContentBase64 string `json:"content_base64"`
}

type FileTransferComplete struct {
	Operation string `json:"operation"`
	SourceID  string `json:"source_id,omitempty"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
	Size      int64  `json:"size"`
}

type EmptyResponse struct {
	OK bool `json:"ok"`
}
