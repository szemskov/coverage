package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type TestCase struct {
	ID      string
	Token   string
	Request *SearchRequest
	Result  *SearchResponse
	IsError bool
}

const (
	AccessToken      = "abc"
	BadAccessToken   = "bca"
	BadNetworkTestId = "Bad network"
	BadFileTestId    = "Bad file"
	TCPTimeoutTestId = "TCP timeout"
)

var (
	OrderFieldEnabled = map[string]bool{
		"Id":     true,
		"Age":    true,
		"Name":   true,
		"About":  false,
		"Gender": false,
	}
	FilePath = "./dataset.xml"
)

type SearchItem struct {
	Id        int    `xml:"id"`
	FirstName string `xml:"first_name"`
	LastName  string `xml:"last_name"`
	Age       int    `xml:"age"`
	About     string `xml:"about"`
	Gender    string `xml:"gender"`
}

type SearchData struct {
	List []SearchItem `xml:"row"`
}

func SearchUsers(query string) ([]User, error) {
	xmlData, err := ioutil.ReadFile(FilePath)
	if err != nil {
		return nil, fmt.Errorf("error: %v", err)
	}

	users := make([]User, 0, 25)
	v := new(SearchData)
	err = xml.Unmarshal(xmlData, &v)
	if err != nil {
		return nil, fmt.Errorf("error: %v", err)
	}
	for _, u := range v.List {
		if strings.Contains(u.FirstName, query) ||
			strings.Contains(u.LastName, query) ||
			strings.Contains(u.About, query) {
			users = append(users, User{
				Id:     u.Id,
				Name:   u.FirstName + " " + u.LastName,
				Age:    u.Age,
				About:  u.About,
				Gender: u.Gender,
			})
		}
	}

	return users, nil
}

func BadResponseSearchServer(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Accesstoken")

	if len(token) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Unauthorized")
		return
	}
	if token != AccessToken {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf(`{"Error": "%s"}`, errTest.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(errTest.Error()))
}

func SearchServer(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Accesstoken")

	if token != AccessToken {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"Error": "access_denied"}`)
		return
	}

	limit, err := strconv.Atoi(r.FormValue("limit"))
	offset, err := strconv.Atoi(r.FormValue("offset"))
	query := r.FormValue("query")
	orderField := r.FormValue("order_field")
	orderBy, err := strconv.Atoi(r.FormValue("order_by"))

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf(`{"Error": "%s"}`, err.Error()))
		return
	}

	if orderField == "" {
		orderField = "Name"
	}

	if _, ok := OrderFieldEnabled[orderField]; !ok {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf(`{"Error": "UnexpectedField"}`))
		return
	}

	if !OrderFieldEnabled[orderField] {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf(`{"Error": "ErrorBadOrderField"}`))
		return
	}

	users, err := SearchUsers(query)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, fmt.Sprintf(`{"Error": "%s"}`, err.Error()))
		return
	}

	lenResult := len(users)
	if offset > lenResult {
		offset = lenResult
	}

	if (offset + limit) > lenResult {
		limit = lenResult - offset
	}
	users = users[offset : offset+limit]

	if sort.SearchStrings([]string{"Name",}, orderField) > -1 {
		sort.Slice(users, func(i, j int) bool {
			if orderBy == OrderByDesc {
				return users[i].Name < users[j].Name
			}

			if orderBy == OrderByAsc {
				return users[i].Name > users[j].Name
			}

			return false
		})
	}

	if sort.SearchStrings([]string{"Age", "Id"}, orderField) != len([]string{"Age", "Id"}) {
		sort.Slice(users, func(i, j int) bool {
			a := reflect.ValueOf(&users[i]).Elem().FieldByName(orderField).Interface().(int)
			b := reflect.ValueOf(&users[j]).Elem().FieldByName(orderField).Interface().(int)

			if orderBy == OrderByAsc {
				return a < b
			}

			if orderBy == OrderByDesc {
				return a > b
			}

			return false
		})
	}

	if orderField == "Name" {
		sort.Slice(users, func(i, j int) bool {
			a := reflect.ValueOf(&users[i]).Elem().FieldByName(orderField).Interface().(string)
			b := reflect.ValueOf(&users[j]).Elem().FieldByName(orderField).Interface().(string)

			if orderBy == OrderByAsc {
				return a < b
			}

			if orderBy == OrderByDesc {
				return a > b
			}

			return false
		})
	}

	w.WriteHeader(http.StatusOK)
	result, err := json.Marshal(users)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, fmt.Sprintf(`{"Error": "%s"}`, err.Error()))
		return
	}

	w.Write(result)
}

func TestSearchServerTimeout(t *testing.T) {
	defer func() {
		client = &http.Client{Timeout: time.Second}
	}()

	testCase := TestCase{
		ID:    TCPTimeoutTestId,
		Token: AccessToken,
		Request: &SearchRequest{
			Query:      "Alex",
			Limit:      1,
			Offset:     0,
			OrderField: "Age",
			OrderBy:    1,
		},
		IsError: true,
	}

	client = &http.Client{Timeout: time.Millisecond}

	checkTestCase(t, &testCase, SearchServer)
}

func TestSearchServerDatabase(t *testing.T) {
	defer func() {
		FilePath = "./dataset.xml"
	}()

	FilePath = "./undefined.xml"

	testCase := TestCase{
		ID:    BadNetworkTestId,
		Token: AccessToken,
		Request: &SearchRequest{
			Query:      "Alex",
			Limit:      1,
			Offset:     0,
			OrderField: "Age",
			OrderBy:    1,
		},
		IsError: true,
	}

	checkTestCase(t, &testCase, SearchServer)
}

func TestSearchServerConnection(t *testing.T) {
	testCase := TestCase{
		ID:    BadFileTestId,
		Token: AccessToken,
		Request: &SearchRequest{
			Query:      "Alex",
			Limit:      1,
			Offset:     0,
			OrderField: "Age",
			OrderBy:    1,
		},
		IsError: true,
	}

	c := &SearchClient{
		AccessToken: testCase.Token,
		URL:         "",
	}

	_, err := c.FindUsers(*(testCase.Request))
	if err == nil && testCase.IsError {
		t.Errorf("[%s] expected error, got nil", testCase.ID)
	}
}

func checkTestCase(t *testing.T, testCase *TestCase, server func(w http.ResponseWriter, r *http.Request)) {
	ts := httptest.NewServer(http.HandlerFunc(server))
	defer ts.Close()

	c := &SearchClient{
		AccessToken: testCase.Token,
		URL:         ts.URL,
	}

	_, err := c.FindUsers(*(testCase.Request))
	if err != nil && !testCase.IsError {
		t.Errorf("[%s] unexpected error: %#v", testCase.ID, err)
	}

	if err == nil && testCase.IsError {
		t.Errorf("[%s] expected error, got nil", testCase.ID)
	}

	//fmt.Printf("%#v\n", result)
}

func TestBadResponseSearchServer(t *testing.T) {
	cases := []TestCase{
		TestCase{
			ID:    "Bad response",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Boyd",
				Limit:      0,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    OrderByAsc,
			},
			IsError: true,
		},
		TestCase{
			ID:    "Parse error JSON response",
			Token: "",
			Request: &SearchRequest{
				Query:      "Boyd",
				Limit:      0,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    OrderByAsc,
			},
			IsError: true,
		},
		TestCase{
			ID:    "Bad error response",
			Token: BadAccessToken,
			Request: &SearchRequest{
				Query:      "Boyd",
				Limit:      0,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    OrderByAsc,
			},
			IsError: true,
		},
	}

	for _, item := range cases {
		checkTestCase(t, &item, BadResponseSearchServer)
	}
}

func TestSearchServer(t *testing.T) {
	cases := []TestCase{
		TestCase{
			ID:    "Zero result",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Boyd",
				Limit:      0,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    OrderByAsIs,
			},
			IsError: false,
		},
		TestCase{
			ID:    "One request",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Boyd",
				Limit:      1,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    OrderByAsIs,
			},
			IsError: false,
		},
		TestCase{
			ID:    "Offset result",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "minim",
				Limit:      5,
				Offset:     5,
				OrderField: "Name",
				OrderBy:    OrderByDesc,
			},
			IsError: false,
		},
		TestCase{
			ID:    "Test sort",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "minim",
				Limit:      5,
				Offset:     5,
				OrderField: "Name",
				OrderBy:    OrderByAsc,
			},
			IsError: false,
		},
		TestCase{
			ID:    "Sort by age",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "minim",
				Limit:      5,
				Offset:     5,
				OrderField: "Age",
				OrderBy:    OrderByAsc,
			},
			IsError: false,
		},
		TestCase{
			ID:    "Sort by Id",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "minim",
				Limit:      5,
				Offset:     5,
				OrderField: "Id",
				OrderBy:    OrderByAsc,
			},
			IsError: false,
		},
		TestCase{
			ID:    "Sort as is",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Boyd",
				Limit:      1,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    OrderByAsIs,
			},
			IsError: false,
		},
		TestCase{
			ID:    "Sort desc",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Boyd",
				Limit:      1,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    OrderByDesc,
			},
			IsError: false,
		},
		TestCase{
			ID:    "Bad access Token",
			Token: BadAccessToken,
			Request: &SearchRequest{
				Query:      "Alex",
				Limit:      1,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    1,
			},
			IsError: true,
		},
		TestCase{
			ID:      "Empty search request",
			Token:   AccessToken,
			Request: &SearchRequest{},
			IsError: false,
		},
		TestCase{
			ID:    "Bad low limit",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Alex",
				Limit:      -10,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    1,
			},
			IsError: true,
		},
		TestCase{
			ID:    "Correct high limit",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Alex",
				Limit:      30,
				Offset:     0,
				OrderField: "Age",
				OrderBy:    1,
			},
			IsError: false,
		},
		TestCase{
			ID:    "Bad low offset",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Alex",
				Limit:      30,
				Offset:     -1,
				OrderField: "Age",
				OrderBy:    1,
			},
			IsError: true,
		},
		TestCase{
			ID:    "Bad sort param",
			Token: AccessToken,
			Request: &SearchRequest{
				Query:      "Alex",
				Limit:      1,
				Offset:     0,
				OrderField: "About",
				OrderBy:    1,
			},
			IsError: true,
		},
	}

	for _, item := range cases {
		checkTestCase(t, &item, SearchServer)
	}
}
