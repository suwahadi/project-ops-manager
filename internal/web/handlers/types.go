package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/web/templates"
)

type typeFormData struct {
	IsEdit     bool
	Type       assettype.Type
	Name       string
	Icon       string
	Offsets    string
	Errors     map[string]string
	Icons      []string
	FieldTypes []assettype.FieldType
	FieldForm  fieldFormData
}

type fieldFormData struct {
	Key      string
	Label    string
	Type     string
	Options  string
	Required bool
	Sort     string
	Errors   map[string]string
}

func (a *App) TypeList(w http.ResponseWriter, r *http.Request) {
	types, err := a.AssetTypes.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "settings_types", "Settings", "settings", shell("types", types))
}

func (a *App) TypeNew(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "type_form", "Settings", "settings", shell("types", typeFormData{
		Icon: "box", Offsets: "30, 14, 7, 1", Errors: map[string]string{}, Icons: templates.IconNames, FieldTypes: assettype.FieldTypes,
	}))
}

func typeInputFrom(r *http.Request) (assettype.TypeInput, typeFormData) {
	d := typeFormData{
		Name: strings.TrimSpace(r.PostFormValue("name")), Icon: r.PostFormValue("icon"), Offsets: r.PostFormValue("offsets"),
		Errors: map[string]string{}, Icons: templates.IconNames, FieldTypes: assettype.FieldTypes,
	}
	if d.Name == "" {
		d.Errors["name"] = "Name is required."
	}
	validIcon := false
	for _, i := range templates.IconNames {
		if i == d.Icon {
			validIcon = true
		}
	}
	if !validIcon {
		d.Icon = "box"
	}
	offsets, err := assettype.ParseOffsets(d.Offsets)
	if err != nil {
		d.Errors["offsets"] = "Reminder offsets: " + err.Error() + "."
	}
	return assettype.TypeInput{Name: d.Name, Icon: d.Icon, Offsets: offsets}, d
}

func (a *App) TypeCreate(w http.ResponseWriter, r *http.Request) {
	in, d := typeInputFrom(r)
	if len(d.Errors) == 0 {
		id, err := a.AssetTypes.Create(r.Context(), in)
		if errors.Is(err, assettype.ErrDuplicate) {
			d.Errors["name"] = "A type with this name already exists."
		} else if err != nil {
			a.serverError(w, r, err)
			return
		} else {
			a.redirectWithFlash(w, r, "/settings/types/"+id, "Asset type created. Add custom fields below.", false)
			return
		}
	}
	a.render(w, r, http.StatusUnprocessableEntity, "type_form", "Settings", "settings", shell("types", d))
}

func (a *App) loadType(w http.ResponseWriter, r *http.Request) (assettype.Type, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		a.notFound(w, r)
		return assettype.Type{}, false
	}
	t, err := a.AssetTypes.Get(r.Context(), id)
	if errors.Is(err, assettype.ErrNotFound) {
		a.notFound(w, r)
		return t, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return t, false
	}
	return t, true
}

func (a *App) renderTypeEdit(w http.ResponseWriter, r *http.Request, status int, t assettype.Type, d typeFormData) {
	d.IsEdit = true
	d.Type = t
	d.Icons = templates.IconNames
	d.FieldTypes = assettype.FieldTypes
	if d.Errors == nil {
		d.Errors = map[string]string{}
	}
	if d.FieldForm.Errors == nil {
		d.FieldForm.Errors = map[string]string{}
	}
	if d.FieldForm.Type == "" {
		d.FieldForm.Type = string(assettype.FieldText)
	}
	a.render(w, r, status, "type_form", "Settings", "settings", shell("types", d))
}

func (a *App) TypeEdit(w http.ResponseWriter, r *http.Request) {
	t, ok := a.loadType(w, r)
	if !ok {
		return
	}
	a.renderTypeEdit(w, r, http.StatusOK, t, typeFormData{Name: t.Name, Icon: t.Icon, Offsets: t.OffsetsText()})
}

func (a *App) TypeUpdate(w http.ResponseWriter, r *http.Request) {
	t, ok := a.loadType(w, r)
	if !ok {
		return
	}
	in, d := typeInputFrom(r)
	if len(d.Errors) == 0 {
		err := a.AssetTypes.Update(r.Context(), t.ID, in)
		if errors.Is(err, assettype.ErrDuplicate) {
			d.Errors["name"] = "A type with this name already exists."
		} else if err != nil {
			a.serverError(w, r, err)
			return
		} else {
			a.redirectWithFlash(w, r, "/settings/types/"+t.ID, "Asset type saved.", false)
			return
		}
	}
	a.renderTypeEdit(w, r, http.StatusUnprocessableEntity, t, d)
}

func (a *App) TypeDelete(w http.ResponseWriter, r *http.Request) {
	t, ok := a.loadType(w, r)
	if !ok {
		return
	}
	err := a.AssetTypes.Delete(r.Context(), t.ID)
	switch {
	case errors.Is(err, assettype.ErrBuiltin):
		a.redirectWithFlash(w, r, "/settings/types/"+t.ID, "Built-in types cannot be deleted.", true)
	case errors.Is(err, assettype.ErrInUse):
		a.redirectWithFlash(w, r, "/settings/types/"+t.ID, "This type still has assets and cannot be deleted.", true)
	case err != nil:
		a.serverError(w, r, err)
	default:
		a.redirectWithFlash(w, r, "/settings/types", "Asset type deleted.", false)
	}
}

func fieldInputFrom(r *http.Request, t assettype.Type, isNew bool) (assettype.FieldInput, fieldFormData) {
	f := fieldFormData{
		Key: strings.TrimSpace(r.PostFormValue("key")), Label: strings.TrimSpace(r.PostFormValue("label")),
		Type: r.PostFormValue("field_type"), Options: r.PostFormValue("options"), Required: r.PostFormValue("is_required") != "",
		Sort: r.PostFormValue("sort_order"), Errors: map[string]string{},
	}
	if f.Label == "" {
		f.Errors["label"] = "Label is required."
	}
	if isNew {
		if f.Key == "" {
			f.Key = assettype.KeyFromLabel(f.Label)
		}
		if !assettype.ValidKey(f.Key) {
			f.Errors["key"] = "Key must start with a letter and use only lowercase letters, digits and underscores."
		}
	}
	ft, ok := assettype.ParseFieldType(f.Type)
	if !ok {
		f.Errors["field_type"] = "Choose a field type."
	}
	opts := assettype.ParseOptions(f.Options)
	if ft == assettype.FieldSelect && len(opts) == 0 {
		f.Errors["options"] = "Select fields need at least one option, one per line."
	}
	sort := len(t.Fields) + 1
	if f.Sort != "" {
		n, err := strconv.Atoi(f.Sort)
		if err != nil {
			f.Errors["sort_order"] = "Sort order must be a whole number."
		}
		sort = n
	}
	return assettype.FieldInput{Key: f.Key, Label: f.Label, Type: ft, Options: opts, Required: f.Required, Sort: sort}, f
}

func (a *App) FieldCreate(w http.ResponseWriter, r *http.Request) {
	t, ok := a.loadType(w, r)
	if !ok {
		return
	}
	in, f := fieldInputFrom(r, t, true)
	if len(f.Errors) == 0 {
		err := a.AssetTypes.CreateField(r.Context(), t.ID, in)
		if errors.Is(err, assettype.ErrDuplicate) {
			f.Errors["key"] = "This type already has a field with this key."
		} else if err != nil {
			a.serverError(w, r, err)
			return
		} else {
			a.redirectWithFlash(w, r, "/settings/types/"+t.ID, "Field added.", false)
			return
		}
	}
	a.renderTypeEdit(w, r, http.StatusUnprocessableEntity, t, typeFormData{Name: t.Name, Icon: t.Icon, Offsets: t.OffsetsText(), FieldForm: f})
}

func (a *App) FieldUpdate(w http.ResponseWriter, r *http.Request) {
	t, ok := a.loadType(w, r)
	if !ok {
		return
	}
	field, err := a.AssetTypes.Field(r.Context(), t.ID, r.PathValue("fid"))
	if err != nil {
		a.notFound(w, r)
		return
	}
	in, f := fieldInputFrom(r, t, false)
	if len(f.Errors) > 0 {
		var msgs []string
		for _, m := range f.Errors {
			msgs = append(msgs, m)
		}
		a.redirectWithFlash(w, r, "/settings/types/"+t.ID, strings.Join(msgs, " "), true)
		return
	}
	if err := a.AssetTypes.UpdateField(r.Context(), field, in); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings/types/"+t.ID, "Field saved.", false)
}

type fieldRemoveData struct {
	Type     assettype.Type
	Field    assettype.Field
	Affected int64
}

func (a *App) FieldRemoveConfirm(w http.ResponseWriter, r *http.Request) {
	t, ok := a.loadType(w, r)
	if !ok {
		return
	}
	field, err := a.AssetTypes.Field(r.Context(), t.ID, r.PathValue("fid"))
	if err != nil {
		a.notFound(w, r)
		return
	}
	n, err := a.AssetTypes.CountFieldValues(r.Context(), field)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.fragment(w, r, http.StatusOK, "type_form", "field_remove_dialog", fieldRemoveData{Type: t, Field: field, Affected: n})
}

func (a *App) FieldRemove(w http.ResponseWriter, r *http.Request) {
	t, ok := a.loadType(w, r)
	if !ok {
		return
	}
	field, err := a.AssetTypes.Field(r.Context(), t.ID, r.PathValue("fid"))
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.AssetTypes.RemoveField(r.Context(), field); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings/types/"+t.ID, "Field removed and its values deleted.", false)
}
