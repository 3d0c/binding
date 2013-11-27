package binding

import (
	"encoding/json"
	"github.com/codegangsta/martini"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
)

/*
	To the land of Middle-ware Earth:
		One func to rule them all,
		One func to find them,
		One func to bring them all,
		And in this package BIND them.
			- Sincerely, Sauron
*/
func Bind(obj interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request, resp http.ResponseWriter) {
		contentType := req.Header.Get("Content-Type")

		if strings.Contains(contentType, "form-urlencoded") {
			context.Invoke(Form(obj))
		} else if strings.Contains(contentType, "json") {
			context.Invoke(Json(obj))
		} else {
			context.Invoke(Json(obj))
			if getErrors(context).Count() > 0 {
				context.Invoke(Form(obj))
			}
		}

		bailIfErrors(resp, context)

		context.Invoke(Validate(obj))

		bailIfErrors(resp, context)
	}
}

func Form(formStruct interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		errors := newErrors()
		parseErr := req.ParseForm()

		if parseErr != nil {
			errors.Overall[DeserializationError] = parseErr.Error()
		}

		typ := reflect.TypeOf(formStruct).Elem()

		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if nameInTag := field.Tag.Get("form"); nameInTag != "" {
				val := req.Form.Get(nameInTag)
				reflect.ValueOf(formStruct).Elem().Field(i).SetString(val)
			}
		}

		context.Map(*errors)
		context.Map(formStruct)
	}
}

func Json(jsonStruct interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		if req.Body != nil {
			defer req.Body.Close()
		}
		errors := newErrors()

		content, err := ioutil.ReadAll(req.Body)
		if err != nil {
			errors.Overall[ReaderError] = err.Error()
		} else if err = json.Unmarshal(content, jsonStruct); err != nil {
			errors.Overall[DeserializationError] = err.Error()
		}

		context.Map(*errors)
		context.Map(jsonStruct)
	}
}

func Validate(obj interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		typ := reflect.TypeOf(obj).Elem()
		errors := newErrors()

		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if hasRequired(string(field.Tag)) && !reflect.ValueOf(field).IsValid() {
				errors.Fields[field.Name] = RequireError
			}
		}

		if validator, ok := obj.(Validator); ok {
			validator.Validate(errors)
		}

		context.Map(*errors)
	}
}

// Parsing tags on our own? Madness, you say: The reflect package
// does this for us! Well, not really. The built-in parsing
// done by .Get() gets the value only, and doesn't detect if the
// key is there. Example: .Get("key") is "" for both `key:""` and ``.
// We just want to know if the required key is present in the tag.
func hasRequired(tag string) bool {
	word, required := "", "required"
	skip := false

	for i := 0; i < len(tag); i++ {
		char := tag[i]
		letter := tag[i : i+1]

		if char == '"' {
			skip = !skip
		}

		if skip {
			continue
		} else if char == ' ' || char == '\t' || char == ':' { // `required:"whatever"` will still return true
			if word == required {
				return true
			}
			word = ""
		} else {
			word += letter
		}

		if i == len(tag)-1 {
			if word == required {
				return true
			}
		}
	}

	return false
}

func newErrors() *Errors {
	return &Errors{make(map[string]string), make(map[string]string)}
}

func bailIfErrors(resp http.ResponseWriter, context martini.Context) {
	errs := getErrors(context)
	if errs.Count() > 0 {
		resp.WriteHeader(http.StatusBadRequest)
		errOutput, _ := json.Marshal(errs)
		resp.Write(errOutput)
		return
	}
}

func getErrors(context martini.Context) Errors {
	return context.Get(errsType).Interface().(Errors)
}

func (self Errors) Count() int {
	return len(self.Overall) + len(self.Fields)
}

type (
	// Errors represents the contract of the response body when the
	// binding step fails before getting to the application.
	Errors struct {
		Overall map[string]string `json:"overall"`
		Fields  map[string]string `json:"fields"`
	}

	// Implement the Validator interface to define your own input
	// validation before the request even gets to your application.
	Validator interface {
		Validate(*Errors)
	}
)

var (
	errsType = reflect.TypeOf(Errors{})
)

const (
	RequireError         string = "Required"
	DeserializationError string = "DeserializationError"
	ReaderError          string = "ReaderError"
)
