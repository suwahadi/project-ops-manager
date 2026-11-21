package handlers

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/suwahadi/project-ops-manager/internal/document"
)

const maxFilesPerUpload = 10

func readUploads(r *http.Request, field string) ([]document.Upload, error) {
	if r.MultipartForm == nil {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				return nil, errors.New("The upload is too large.")
			}
			if errors.Is(err, http.ErrNotMultipart) {
				return nil, nil
			}
			return nil, errors.New("Could not read the upload.")
		}
	}
	headers := r.MultipartForm.File[field]
	if len(headers) > maxFilesPerUpload {
		return nil, fmt.Errorf("Upload at most %d files at once.", maxFilesPerUpload)
	}
	var out []document.Upload
	for _, h := range headers {
		if h.Size > document.MaxFileSize {
			return nil, fmt.Errorf("%s is larger than 20 MB.", document.CleanName(h.Filename))
		}
		f, err := h.Open()
		if err != nil {
			return nil, errors.New("Could not read the upload.")
		}
		data, err := io.ReadAll(io.LimitReader(f, document.MaxFileSize+1))
		f.Close()
		if err != nil {
			return nil, errors.New("Could not read the upload.")
		}
		if len(data) == 0 {
			continue
		}
		out = append(out, document.Upload{Name: h.Filename, Data: data})
	}
	return out, nil
}

func (a *App) prepareUploads(uploads []document.Upload) ([]document.Prepared, error) {
	var prepared []document.Prepared
	for _, u := range uploads {
		p, err := a.Documents.Prepare(u, a.Now())
		if err != nil {
			a.Documents.RemovePrepared(prepared)
			name := document.CleanName(u.Name)
			switch {
			case errors.Is(err, document.ErrUnsupported):
				return nil, fmt.Errorf("%s: only PDF, JPEG, PNG and WebP files are accepted.", name)
			case errors.Is(err, document.ErrTooLarge):
				return nil, fmt.Errorf("%s is larger than 20 MB.", name)
			}
			a.Logger.Warn("prepare upload failed", "error", err.Error())
			return nil, fmt.Errorf("%s could not be processed.", name)
		}
		prepared = append(prepared, p)
	}
	return prepared, nil
}

func (a *App) DocumentUpload(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	back := "/assets/" + det.ID + "?tab=documents"
	uploads, err := readUploads(r, "documents")
	if err != nil {
		a.redirectWithFlash(w, r, back, err.Error(), true)
		return
	}
	if len(uploads) == 0 {
		a.redirectWithFlash(w, r, back, "Choose at least one file.", true)
		return
	}
	prepared, err := a.prepareUploads(uploads)
	if err != nil {
		a.redirectWithFlash(w, r, back, err.Error(), true)
		return
	}
	if err := a.Assets.AttachDocuments(r.Context(), det.ID, prepared); err != nil {
		a.Documents.RemovePrepared(prepared)
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, back, fmt.Sprintf("%d %s uploaded.", len(prepared), pluralize(len(prepared), "document", "documents")), false)
}

func pluralize(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (a *App) DocumentServe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		http.NotFound(w, r)
		return
	}
	d, err := a.Documents.Get(r.Context(), id)
	if errors.Is(err, document.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	f, err := os.Open(a.Documents.Path(d.StoredPath))
	if err != nil {
		a.Logger.Error("open document file", "document_id", d.ID, "error", err)
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", d.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": d.OriginalName}))
	w.Header().Set("Cache-Control", "private, max-age=300")
	if strings.HasPrefix(d.ContentType, "application/pdf") {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; object-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
	}
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func (a *App) DocumentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		a.notFound(w, r)
		return
	}
	d, err := a.Documents.Get(r.Context(), id)
	if errors.Is(err, document.ErrNotFound) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.Documents.Delete(r.Context(), id); err != nil {
		a.serverError(w, r, err)
		return
	}
	tab := "documents"
	if d.RenewalID != nil {
		tab = "renewals"
	}
	a.redirectWithFlash(w, r, "/assets/"+d.OwnerAssetID+"?tab="+tab, "Document deleted.", false)
}
