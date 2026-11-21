package templates

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed layout/*.html partials/*.html pages/*.html
var files embed.FS

type Renderer struct {
	pages map[string]*template.Template
}

func New(funcs template.FuncMap) (*Renderer, error) {
	all := template.FuncMap{}
	for k, v := range baseFuncs() {
		all[k] = v
	}
	for k, v := range funcs {
		all[k] = v
	}
	base, err := template.New("").Funcs(all).ParseFS(files, "layout/*.html", "partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	pageFiles, err := fs.Glob(files, "pages/*.html")
	if err != nil {
		return nil, err
	}
	r := &Renderer{pages: map[string]*template.Template{}}
	for _, f := range pageFiles {
		t, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if _, err := t.ParseFS(files, f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		r.pages[strings.TrimSuffix(path.Base(f), ".html")] = t
	}
	return r, nil
}

func (r *Renderer) Page(w http.ResponseWriter, status int, page string, data any) error {
	return r.execute(w, status, page, "layout", data)
}

func (r *Renderer) Bare(w http.ResponseWriter, status int, page string, data any) error {
	return r.execute(w, status, page, "bare", data)
}

func (r *Renderer) Fragment(w http.ResponseWriter, status int, page, name string, data any) error {
	return r.execute(w, status, page, name, data)
}

func (r *Renderer) HTML(page, name string, data any) ([]byte, error) {
	t, ok := r.pages[page]
	if !ok {
		return nil, fmt.Errorf("unknown page %q", page)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("render %s/%s: %w", page, name, err)
	}
	return buf.Bytes(), nil
}

func (r *Renderer) execute(w http.ResponseWriter, status int, page, name string, data any) error {
	t, ok := r.pages[page]
	if !ok {
		return fmt.Errorf("unknown page %q", page)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Errorf("render %s/%s: %w", page, name, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := io.Copy(w, &buf)
	return err
}
