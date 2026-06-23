// Copyright © 2019 Antoine Chiny <antoine.chiny@inria.fr>
// Copyright © 2019 Ryan Ciehanski <ryan@ciehanski.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package libgen

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
)

// Book is the struct of resources on Library Genesis.
type Book struct {
	ID          string
	Title       string
	Author      string
	Filesize    string
	Extension   string
	Md5         string
	Year        string
	Language    string
	Pages       string
	Publisher   string
	Edition     string
	CoverURL    string
	DownloadURL string
	PageURL     string
}

// SearchOptions are the optional parameters available for the Search
// function.
type SearchOptions struct {
	Query         string
	SearchMirror  url.URL
	Results       int
	Print         bool
	RequireAuthor bool
	Extension     []string
	Year          int
	Publisher     string
	Language      string
	SortBy        string
	SortASC       bool
}

// GetDetailsOptions are the optional parameters available for the GetDetails
// function.
type GetDetailsOptions struct {
	Hashes        []string
	SearchMirror  url.URL
	Print         bool
	RequireAuthor bool
	Extension     []string
	Year          int
	Publisher     string
	Language      string
	SortBy        string
}

// Search sends a query to the index.php page hosted by gen.lib.rus.ec(or any
// similar mirror) and then provides the web page's contents provided from the
// resulting http request to the parseHashes() function to extract the specific
// hashes of matches found from the search query provided.
func Search(options *SearchOptions) ([]*Book, error) {
	// libgen search only allows query Results of 25, 50 or 100.
	var res int
	switch {
	case options.Results <= 25:
		res = 25
	case options.Results <= 50:
		res = 50
	default:
		res = 100
	}

	// Define DownloadURL with required query parameters
	q := options.SearchMirror.Query()
	q.Set("req", options.Query)
	q.Set("lg_topic", "libgen")
	q.Set("open", "0")
	q.Set("view", "simple")
	q.Set("res", fmt.Sprint(res))
	q.Set("phrase", "1")
	q.Set("column", "def")
	// Handle sorting options
	switch options.SortBy {
	case "id":
		q.Set("sort", "id")
		setSortASC(q, options.SortASC)
	case "title":
		q.Set("sort", "title")
		setSortASC(q, options.SortASC)
	case "author":
		q.Set("sort", "author")
		setSortASC(q, options.SortASC)
	case "pub":
		q.Set("sort", "publisher")
		setSortASC(q, options.SortASC)
	case "ext":
		q.Set("sort", "extension")
		setSortASC(q, options.SortASC)
	case "year":
		q.Set("sort", "year")
		setSortASC(q, options.SortASC)
	case "size":
		q.Set("sort", "filesize")
		setSortASC(q, options.SortASC)
	case "lang":
		q.Set("sort", "language")
		setSortASC(q, options.SortASC)
	}
	options.SearchMirror.RawQuery = q.Encode()
	// fmt.Println("options.SearchMirror.String() = ", options.SearchMirror.String())
	b, err := getBody(options.SearchMirror.String())
	if err != nil {
		return nil, err
	}

	// Get hashes from raw webpage and store them in hashes
	hashes := parseHashes(b, options.Results)

	books, err := GetDetails(&GetDetailsOptions{
		Hashes:        hashes,
		SearchMirror:  options.SearchMirror,
		Print:         options.Print,
		RequireAuthor: options.RequireAuthor,
		Extension:     options.Extension,
		Year:          options.Year,
		Publisher:     options.Publisher,
		Language:      options.Language,
		SortBy:        options.SortBy,
	})
	if err != nil {
		return nil, err
	}

	return books, nil
}

// GetDetails retrieves more details about a specific piece of media
// based off of its unique hash/id. That information is then requested
// in JSON format and sanitized in an array of Books.
func GetDetails(options *GetDetailsOptions) ([]*Book, error) {
	var books []*Book

	// For each hash found on the page, parse it into a Book struct
	for _, hash := range options.Hashes {
		// Step 1: Get file info (filesize, extension, pages, md5, edition ID)
		options.SearchMirror.Path = "json.php"
		q := options.SearchMirror.Query()
		q.Set("object", "f")
		q.Set("md5", hash)
		options.SearchMirror.RawQuery = q.Encode()

		b, err := getBody(options.SearchMirror.String())
		if err != nil {
			return nil, err
		}

		book, editionID, err := parseFileResponse(b)
		if err != nil {
			continue
		}

		// Step 2: Get edition info (title, author, year, publisher, language)
		if editionID != "" {
			options.SearchMirror.Path = "json.php"
			q = options.SearchMirror.Query()
			q.Set("object", "e")
			q.Set("ids", editionID)
			options.SearchMirror.RawQuery = q.Encode()

			eb, err := getBody(options.SearchMirror.String())
			if err == nil {
				parseEditionResponse(eb, book)
			}
		}

		// Flag filters
		if options.RequireAuthor && book.Author == "" {
			continue
		}
		if len(options.Extension) > 0 {
			validExtension := false
			// 也就是说可以选择多个后缀，只要满足一个就可以
			for _, ext := range options.Extension {
				if ext == book.Extension {
					validExtension = true
				}
			}
			if !validExtension {
				continue
			}
		}
		if options.Year != 0 {
			y, err := strconv.Atoi(book.Year)
			if err != nil {
				return nil, err
			}
			if options.Year != y {
				continue
			}
		}
		// Many books don't have the year field set, so
		// if we are sorting by year, we need to skip any books
		// with a blank year field.
		if options.SortBy == "year" {
			if book.Year == "" || book.Year == "0" {
				continue
			}
		}
		if options.Publisher != "" {
			if !strings.Contains(strings.ToLower(book.Publisher), strings.ToLower(options.Publisher)) {
				continue
			}
		}
		if options.Language != "" {
			if !strings.EqualFold(book.Language, options.Language) {
				continue
			}
		}
		if options.Print {
			if err := printDetails(book); err != nil {
				return nil, err
			}
		}

		// Add valid book to the []Book for the search
		books = append(books, book)
	}

	return books, nil
}

// CheckMirror returns the HTTP status code of the DownloadURL provided.
func CheckMirror(url url.URL) int {
	status, err := probeMirror(url)
	if err != nil {
		return http.StatusBadGateway
	}
	return status
}

func probeMirror(url url.URL) (int, error) {
	client := http.Client{
		Timeout: HTTPClientTimeout,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
	r, err := client.Get(url.String())
	if err != nil {
		return http.StatusBadGateway, err
	}
	if r.StatusCode != http.StatusOK {
		return r.StatusCode, nil
	}
	return http.StatusOK, nil
}

// GetWorkingMirror selects a random mirror from the []url.DownloadURL
// provided and checks the mirror for a proper HTTP status code
// for working order.
func GetWorkingMirror(urls []url.URL) url.URL {
	mirror, err := FindWorkingMirror(urls)
	if err != nil {
		return url.URL{}
	}
	return mirror
}

// FindWorkingMirror checks each mirror at most once in random order and
// returns the first mirror that responds with HTTP 200.
func FindWorkingMirror(urls []url.URL) (url.URL, error) {
	var mirror url.URL
	if len(urls) == 0 {
		return mirror, errors.New("no mirrors configured")
	}

	var failures []string
	for _, i := range rand.Perm(len(urls)) {
		randMirror := urls[i]
		status, err := probeMirror(randMirror)
		if err == nil && status == http.StatusOK {
			return randMirror, nil
		}

		reason := fmt.Sprintf("HTTP %d", status)
		if err != nil {
			reason = err.Error()
		}
		failures = append(failures, fmt.Sprintf("%s: %s", randMirror.String(), reason))
	}

	return mirror, fmt.Errorf("no working mirrors found (%d checked): %s", len(urls), strings.Join(failures, "; "))
}

// ParseDbdumps takes in a HTTP response and scans it for
// any string that matches a filepath and returns all results.
func ParseDbdumps(response []byte) []string {
	re := regexp.MustCompile(dbdumpReg)
	dbdumps := re.FindAllString(string(response), -1)

	for i, dbdump := range dbdumps {
		dbdumps[i] = RemoveQuotes(dbdump)
	}

	return dbdumps
}

func getBody(baseURL string) ([]byte, error) {
	client := http.Client{
		Timeout: HTTPClientTimeout,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
	r, err := client.Get(baseURL)
	if err != nil {
		log.Printf("http.Get(%q) error: %v", baseURL, err)
		return nil, err
	}
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to reach to mirror %v: %v", baseURL, r.StatusCode)
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	if err := r.Body.Close(); err != nil {
		return nil, err
	}

	return b, nil
}

// parseHashes takes in a HTTP response and scans it for
// an MD5 hash and then returns the found hashes.
func parseHashes(response []byte, results int) []string {
	var hashes []string
	re := regexp.MustCompile(SearchHref)
	matches := re.FindAllString(string(response), -1)
	// os.WriteFile("response.html", response, 0644)
	// fmt.Println("matches = ", matches)
	var counter int
	for _, m := range matches {
		if counter >= results {
			break
		}
		re := regexp.MustCompile(SearchMD5)
		hash := re.FindString(m)
		if len(hash) == 32 {
			hashes = append(hashes, hash)
			counter++
		}
	}

	return hashes
}

// parseFileResponse parses the JSON response from object=f API.
// Returns a Book with file-level fields and the edition ID for further lookup.
func parseFileResponse(response []byte) (*Book, string, error) {
	var book Book

	// New format: {"file_id": {"md5": "...", "filesize": "...", "editions": {...}}}
	var resp map[string]map[string]interface{}
	if err := json.Unmarshal(response, &resp); err != nil {
		return nil, "", err
	}
	if len(resp) == 0 {
		return nil, "", errors.New("empty response or unexpected JSON")
	}

	var editionID string
	for id, item := range resp {
		str := func(key string) string {
			if v, ok := item[key]; ok {
				return fmt.Sprint(v)
			}
			return ""
		}
		book.ID = id
		book.Filesize = str("filesize")
		book.Extension = str("extension")
		book.Md5 = str("md5")
		book.Pages = str("pages")

		// Extract edition ID from nested editions object
		if editions, ok := item["editions"]; ok {
			if edMap, ok := editions.(map[string]interface{}); ok {
				for _, ev := range edMap {
					if edInfo, ok := ev.(map[string]interface{}); ok {
						if eid, ok := edInfo["e_id"]; ok {
							editionID = fmt.Sprint(eid)
						}
					}
					break // take the first edition
				}
			}
		}
		break // only take the first file entry
	}

	return &book, editionID, nil
}

// parseEditionResponse parses the JSON response from object=e API
// and fills in the book metadata fields (title, author, year, etc.).
func parseEditionResponse(response []byte, book *Book) {
	var resp map[string]map[string]interface{}
	if err := json.Unmarshal(response, &resp); err != nil {
		return
	}

	for _, item := range resp {
		str := func(key string) string {
			if v, ok := item[key]; ok {
				return fmt.Sprint(v)
			}
			return ""
		}
		book.Title = str("title")
		book.Author = str("author")
		book.Year = str("year")
		book.Language = str("language")
		book.Publisher = str("publisher")
		book.Edition = str("edition")
		book.CoverURL = str("cover_url")
		break
	}
}

func printDetails(book *Book) error {
	var fsize string
	size, err := strconv.Atoi(book.Filesize)
	if err != nil {
		fsize = "N/A"
	} else {
		fsize = humanize.Bytes(uint64(size))
	}

	// Print separation lines
	fmt.Println(strings.Repeat("-", 80))

	// Print md5 + Title
	fTitle := fmt.Sprintf("MD5: %5s %s", color.New(color.FgHiBlue).Sprintf(book.Md5), book.Title)
	fTitle = formatTitle(fTitle, TitleMaxLength)
	if runtime.GOOS == "windows" {
		_, err = fmt.Fprintf(color.Output, "%s\n    ++ ", fTitle)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("%s\n    ++ ", fTitle)
	}

	// Slice author name if it exceeds AuthorMaxLength
	var formatAuthor string
	if len(book.Author) > AuthorMaxLength {
		formatAuthor = book.Author[:AuthorMaxLength]
	} else if book.Author == "" {
		formatAuthor = "N/A"
	} else {
		formatAuthor = book.Author
	}

	err = prettify("author", formatAuthor, color.FgYellow, "-25")
	if err != nil {
		return err
	}
	err = prettify("year", book.Year, color.FgCyan, "4")
	if err != nil {
		return err
	}
	err = prettify("size", fsize, color.FgGreen, "6")
	if err != nil {
		return err
	}
	err = prettify("type", book.Extension, color.FgRed, "4")
	if err != nil {
		return err
	}
	fmt.Println()

	return nil
}

// formatTitle shortens the title of a Book down to
// the maximum allowed by TitleMaxLength.
func formatTitle(title string, maximumLength int) string {
	var fTitle []string
	var counter int

	if len(title) <= maximumLength {
		return title
	}

	title = strings.TrimSpace(title)
	for _, t := range strings.Split(title, " ") {
		counter += len(t)

		if counter > maximumLength {
			counter = 0
			t = t + "...\n"
		}
		fTitle = append(fTitle, t)
	}

	return strings.Join(fTitle, " ")
}

// prettify is a helper function that adds color and
// formats text returned to the user.
func prettify(key string, value string, col color.Attribute, align string) error {
	c := color.New(col).SprintFunc()
	a := fmt.Sprintf("%%%ss ", align)
	s := fmt.Sprintf("@%s "+a, c(key), value)
	if runtime.GOOS == "windows" {
		_, err := fmt.Fprintf(color.Output, a, s)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf(a, s)
	}
	return nil
}

// RemoveQuotes is a helper function that removes the quotes from
// dbdumps page results.
func RemoveQuotes(s string) string {
	if s == "" {
		return ""
	}
	s = s[1:]
	s = s[:len(s)-1]
	return s
}

func setSortASC(q url.Values, sortASC bool) {
	if sortASC {
		q.Set("sortmode", "ASC")
	} else {
		q.Set("sortmode", "DESC")
	}
}
