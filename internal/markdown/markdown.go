package markdown

import (
	"bytes"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

type Renderer struct {
	md     goldmark.Markdown
	policy *bluemonday.Policy
}

func New() *Renderer {
	policy := bluemonday.UGCPolicy()
	policy.RequireNoReferrerOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)
	policy.AllowAttrs("type", "checked", "disabled").Matching(bluemonday.SpaceSeparatedTokens).OnElements("input")
	return &Renderer{
		md:     goldmark.New(goldmark.WithExtensions(extension.GFM)),
		policy: policy,
	}
}

func (r *Renderer) Render(src string) template.HTML {
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(r.policy.SanitizeBytes(buf.Bytes()))
}
