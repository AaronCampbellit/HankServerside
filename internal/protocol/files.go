package protocol

import "time"

const (
	FileTransferOperationDownload = "download"
	FileTransferOperationUpload   = "upload"
)

type FileItem struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	IsDirectory bool      `json:"is_directory"`
	Size        int64     `json:"size"`
	ModifiedAt  time.Time `json:"modified_at"`
}

type FilesListRequest struct {
	Path string `json:"path"`
}

type FilesListResponse struct {
	Items []FileItem `json:"items"`
}

type FilesStatRequest struct {
	Path string `json:"path"`
}

type FilesStatResponse struct {
	Item FileItem `json:"item"`
}

type FilesCreateDirectoryRequest struct {
	Path string `json:"path"`
}

type FilesRenameRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type FilesDeleteRequest struct {
	Path        string `json:"path"`
	IsDirectory bool   `json:"is_directory"`
}

type FilesDownloadRequest struct {
	Path string `json:"path"`
}

type FilesDownloadResponse struct {
	Path          string `json:"path"`
	ContentBase64 string `json:"content_base64"`
}

type FilesUploadRequest struct {
	Path          string `json:"path"`
	ContentBase64 string `json:"content_base64"`
}

type FileTransferOpen struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
}

type FileTransferReady struct {
	Operation string `json:"operation"`
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
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
	Size      int64  `json:"size"`
}

type EmptyResponse struct {
	OK bool `json:"ok"`
}
