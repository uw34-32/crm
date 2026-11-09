package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"crm/pkg/model"
	"crm/pkg/store"
)

const (
	maxPhoneRunes  = 50
	maxSearchRunes = 200
	maxDomainBytes = 253
	maxLabelBytes  = 63

	msgNoFields       = "at least one field is required"
	msgInvalidPhone   = "phone must have a digit and at most 50 characters"
	msgInvalidEmail   = "email must be a valid address"
	msgInvalidListArg = "limit must be 1 to 200, offset 0 to 100000, and q at most 200 characters"
)

// crmFailure answers a failed CRM operation. Each not-found error already covers
// every way a resource can be out of reach, so all of them get one identical body.
func crmFailure(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrCompanyNotFound):
		errResp(w, http.StatusNotFound, "company not found")
	case errors.Is(err, store.ErrContactNotFound):
		errResp(w, http.StatusNotFound, "contact not found")
	case errors.Is(err, store.ErrPipelineNotFound):
		errResp(w, http.StatusNotFound, "pipeline not found")
	case errors.Is(err, store.ErrStageNotFound):
		errResp(w, http.StatusNotFound, "stage not found")
	case errors.Is(err, store.ErrDealNotFound):
		errResp(w, http.StatusNotFound, "deal not found")
	case errors.Is(err, store.ErrStageNotOpen):
		errResp(w, http.StatusBadRequest, "a deal starts in an open stage")
	case errors.Is(err, store.ErrLostReasonRequired):
		errResp(w, http.StatusBadRequest, "a lost_reason is required to move a deal into a lost stage")
	case errors.Is(err, store.ErrLostReasonNotAllowed):
		errResp(w, http.StatusBadRequest, "only a lost stage takes a lost_reason")
	case errors.Is(err, store.ErrPipelineHasDeals):
		errResp(w, http.StatusConflict, "the pipeline still holds deals")
	case errors.Is(err, store.ErrStageHasDeals):
		errResp(w, http.StatusConflict, "the stage still holds deals")
	case errors.Is(err, store.ErrLastOpenStage):
		errResp(w, http.StatusConflict, "a pipeline must keep at least one open stage")
	case errors.Is(err, store.ErrLastPipeline):
		errResp(w, http.StatusConflict, "a workspace must keep at least one pipeline")
	case errors.Is(err, store.ErrStageOrderMismatch):
		errResp(w, http.StatusConflict, "stage_ids must list every stage of the pipeline exactly once")
	default:
		workspaceFailure(w, op, err)
	}
}

// listParams reads limit, offset, and q. Anything out of range is refused rather
// than adjusted, so a client bug shows up instead of returning a surprising page.
func listParams(r *http.Request) (model.ListParams, bool) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return model.ListParams{}, false
	}
	params := model.ListParams{Limit: model.DefaultPageSize}
	if query.Has("limit") {
		params.Limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || params.Limit < 1 || params.Limit > model.MaxPageSize {
			return model.ListParams{}, false
		}
	}
	if query.Has("offset") {
		params.Offset, err = strconv.Atoi(query.Get("offset"))
		if err != nil || params.Offset < 0 || params.Offset > model.MaxPageOffset {
			return model.ListParams{}, false
		}
	}
	// A query string, unlike a JSON body, can carry invalid UTF-8, which PostgreSQL
	// would refuse with an error.
	search := query.Get("q")
	if hasNUL(search) || !utf8.ValidString(search) {
		return model.ListParams{}, false
	}
	params.Search = strings.TrimSpace(search)
	if utf8.RuneCountInString(params.Search) > maxSearchRunes {
		return model.ListParams{}, false
	}
	return params, true
}

// listRequest answers 400 and reports false if the list arguments are invalid.
func listRequest(w http.ResponseWriter, r *http.Request) (model.ListParams, bool) {
	params, ok := listParams(r)
	if !ok {
		errResp(w, http.StatusBadRequest, msgInvalidListArg)
	}
	return params, ok
}

// optionalNUL reports whether any field that is present holds a NUL.
func optionalNUL(fields ...*string) bool {
	for _, field := range fields {
		if field != nil && hasNUL(*field) {
			return true
		}
	}
	return false
}

// trimmedFits trims a field that is present, in place, and reports whether it is
// within the limit. A field that is absent fits.
func trimmedFits(field *string, maxRunes int) bool {
	if field == nil {
		return true
	}
	*field = strings.TrimSpace(*field)
	return utf8.RuneCountInString(*field) <= maxRunes
}

// requiredFits is trimmedFits for a field that may not be blank when present.
func requiredFits(field *string, maxRunes int) bool {
	return trimmedFits(field, maxRunes) && (field == nil || *field != "")
}

// validOptionalPhone keeps a phone number as it was typed, because numbers are
// written many ways, and only requires that it could be one.
func validOptionalPhone(field *string) bool {
	if !trimmedFits(field, maxPhoneRunes) {
		return false
	}
	return field == nil || *field == "" || strings.ContainsFunc(*field, unicode.IsDigit)
}

func validOptionalEmail(field *string) bool {
	if field == nil {
		return true
	}
	*field = strings.TrimSpace(*field)
	return *field == "" || validEmail(*field)
}

// validOptionalDomain lowercases a domain in place and checks it is a bare host
// name with at least two labels, so "https://acme.example" is refused.
func validOptionalDomain(field *string) bool {
	if field == nil {
		return true
	}
	*field = strings.ToLower(strings.TrimSpace(*field))
	return *field == "" || validDomain(*field)
}

func validDomain(domain string) bool {
	if len(domain) > maxDomainBytes {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > maxLabelBytes || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if c != '-' && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
				return false
			}
		}
	}
	return true
}

// optionalString tells a field that was left out from one sent as null: leaving out
// a contact's company keeps the link, null removes it, and an update refuses null text.
type optionalString struct {
	Set   bool
	Value *string
}

func (o *optionalString) UnmarshalJSON(raw []byte) error {
	o.Set = true
	return json.Unmarshal(raw, &o.Value)
}

// optionalInt tells a number that was left out from one sent as null. A fraction or
// a string fails to decode, so a float can never become a value.
type optionalInt struct {
	Set   bool
	Value *int64
}

func (o *optionalInt) UnmarshalJSON(raw []byte) error {
	o.Set = true
	return json.Unmarshal(raw, &o.Value)
}

// intValue returns the value as an int, or nil if there is none.
func (o optionalInt) intValue() *int {
	if o.Value == nil {
		return nil
	}
	value := int(*o.Value)
	return &value
}

var errInvalidUTF8 = errors.New("body is not valid UTF-8")

// decodeText is decodeJSON for bodies that hold text to store. The JSON decoder would
// silently replace invalid UTF-8, changing what the client sent, so it is refused.
func decodeText(w http.ResponseWriter, r *http.Request, v any) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAuthBodyBytes))
	if err != nil {
		return err
	}
	if !utf8.Valid(body) {
		return errInvalidUTF8
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return decodeJSON(w, r, v)
}

type namedText struct {
	name  string
	value optionalString
}

// nullProblem reports the first text field sent as null, if any.
func nullProblem(fields []namedText) string {
	for _, f := range fields {
		if f.value.Set && f.value.Value == nil {
			return f.name + " must not be null"
		}
	}
	return ""
}
