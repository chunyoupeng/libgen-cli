// Copyright © 2019 Antoine Chiny <antoine.chiny@inria.fr>
// Copyright © 2019 Ryan Ciehanski <ryan@ciehanski.com>
// Copyright © 2026 Chunyou Peng <chunyoupeng@gmail.com>
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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

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

type fileRecord struct {
	book      *Book
	editionID string
}

type editionInfo struct {
	title     string
	author    string
	year      string
	language  string
	publisher string
	edition   string
	coverURL  string
}

// Search sends a query to the index.php page hosted by Library Genesis
// and extracts matching MD5 hashes, then fetches detailed metadata.
func Search(options *SearchOptions) ([]*Book, error) {
	var res int
	switch {
	case options.Results <= 25:
		res = 25
	case options.Results <= 50:
		res = 50
	default:
		res = 100
	}

	q := url.Values{}
	q.Set("req", options.Query)
	q.Set("lg_topic", "libgen")
	q.Set("open", "0")
	q.Set("view", "simple")
	q.Set("res", fmt.Sprint(res))
	q.Set("phrase", "1")
	q.Set("column", "def")

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

	searchURL := endpoint(options.SearchMirror, "index.php", q)
	b, err := getBody(searchURL.String())
	if err != nil {
		return nil, err
	}

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

// GetDetails retrieves book details in two phases:
// Phase 1: bounded concurrent lookup of file records (object=f)
// Phase 2: batched lookup of edition records (object=e)
// It then maps metadata, applies filters, and optionally prints details.
func GetDetails(options *GetDetailsOptions) ([]*Book, error) {
	if len(options.Hashes) == 0 {
		return nil, nil
	}

	var validHashes []string
	for _, h := range options.Hashes {
		clean := strings.TrimSpace(h)
		if len(clean) == 32 {
			validHashes = append(validHashes, clean)
		}
	}
	if len(validHashes) == 0 {
		return nil, nil
	}

	// Phase 1: Fetch file info concurrently with bounded workers (up to 4)
	fileRecords := make([]*fileRecord, len(validHashes))
	workers := 4
	if len(validHashes) < workers {
		workers = len(validHashes)
	}

	type fetchJob struct {
		index int
		hash  string
	}
	jobs := make(chan fetchJob, len(validHashes))
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				q := url.Values{}
				q.Set("object", "f")
				q.Set("md5", job.hash)
				reqURL := endpoint(options.SearchMirror, "json.php", q)

				b, err := getBody(reqURL.String())
				if err != nil {
					continue
				}
				book, editionID, err := parseFileResponse(b)
				if err == nil && book != nil {
					fileRecords[job.index] = &fileRecord{
						book:      book,
						editionID: editionID,
					}
				}
			}
		}()
	}

	for i, h := range validHashes {
		jobs <- fetchJob{index: i, hash: h}
	}
	close(jobs)
	wg.Wait()

	// Phase 2: Collect unique edition IDs and batch-query edition metadata
	editionMap := make(map[string]editionInfo)
	var uniqueEditionIDs []string
	seenEditionIDs := make(map[string]bool)
	for _, rec := range fileRecords {
		if rec != nil && rec.editionID != "" && !seenEditionIDs[rec.editionID] {
			seenEditionIDs[rec.editionID] = true
			uniqueEditionIDs = append(uniqueEditionIDs, rec.editionID)
		}
	}

	batchSize := 30
	for i := 0; i < len(uniqueEditionIDs); i += batchSize {
		end := i + batchSize
		if end > len(uniqueEditionIDs) {
			end = len(uniqueEditionIDs)
		}
		chunk := uniqueEditionIDs[i:end]
		q := url.Values{}
		q.Set("object", "e")
		q.Set("ids", strings.Join(chunk, ","))
		reqURL := endpoint(options.SearchMirror, "json.php", q)

		eb, err := getBody(reqURL.String())
		if err == nil {
			parsed := parseEditionBatchResponse(eb)
			for eid, info := range parsed {
				editionMap[eid] = info
			}
		}
	}

	// Phase 3: Combine metadata, apply filters, preserving input order
	var validExts []string
	for _, ext := range options.Extension {
		trimmed := strings.ToLower(strings.TrimSpace(ext))
		if trimmed != "" {
			validExts = append(validExts, trimmed)
		}
	}

	var books []*Book
	for _, rec := range fileRecords {
		if rec == nil || rec.book == nil {
			continue
		}
		book := rec.book
		if info, ok := editionMap[rec.editionID]; ok {
			book.Title = info.title
			book.Author = info.author
			book.Year = info.year
			book.Language = info.language
			book.Publisher = info.publisher
			book.Edition = info.edition
			book.CoverURL = info.coverURL
		}

		// Flag filters
		if options.RequireAuthor && book.Author == "" {
			continue
		}
		if len(validExts) > 0 {
			matched := false
			bookExt := strings.ToLower(strings.TrimSpace(book.Extension))
			for _, ext := range validExts {
				if ext == bookExt {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if options.Year != 0 {
			y, err := strconv.Atoi(book.Year)
			if err != nil || options.Year != y {
				continue
			}
		}
		if options.SortBy == "year" && (book.Year == "" || book.Year == "0") {
			continue
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

		books = append(books, book)
	}

	return books, nil
}

// CheckMirror returns the HTTP status code of the provided URL.
func CheckMirror(targetURL url.URL) int {
	status, err := probeMirror(targetURL)
	if err != nil {
		return http.StatusBadGateway
	}
	return status
}

func probeMirror(targetURL url.URL) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return http.StatusBadGateway, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return http.StatusBadGateway, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// GetWorkingMirror selects a working mirror or returns an empty url.URL.
func GetWorkingMirror(urls []url.URL) url.URL {
	mirror, err := FindWorkingMirror(urls)
	if err != nil {
		return url.URL{}
	}
	return mirror
}

// FindWorkingMirror probes candidate mirrors concurrently with a bounded timeout
// and returns the first mirror that responds with HTTP 200.
func FindWorkingMirror(urls []url.URL) (url.URL, error) {
	if len(urls) == 0 {
		return url.URL{}, errors.New("no mirrors configured")
	}

	type probeResult struct {
		mirror url.URL
		status int
		err    error
	}

	perm := rand.Perm(len(urls))
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	resultChan := make(chan probeResult, len(urls))
	for _, idx := range perm {
		u := urls[idx]
		go func(target url.URL) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
			if err != nil {
				resultChan <- probeResult{mirror: target, status: http.StatusBadGateway, err: err}
				return
			}
			req.Header.Set("User-Agent", DefaultUserAgent)
			resp, err := defaultHTTPClient.Do(req)
			if err != nil {
				resultChan <- probeResult{mirror: target, status: http.StatusBadGateway, err: err}
				return
			}
			_ = resp.Body.Close()
			resultChan <- probeResult{mirror: target, status: resp.StatusCode, err: nil}
		}(u)
	}

	var failures []string
	for i := 0; i < len(urls); i++ {
		res := <-resultChan
		if res.err == nil && res.status == http.StatusOK {
			cancel()
			return res.mirror, nil
		}
		reason := fmt.Sprintf("HTTP %d", res.status)
		if res.err != nil {
			reason = res.err.Error()
		}
		failures = append(failures, fmt.Sprintf("%s: %s", res.mirror.String(), reason))
	}

	return url.URL{}, fmt.Errorf("no working mirrors found (%d checked): %s", len(urls), strings.Join(failures, "; "))
}

// ParseDbdumps scans HTTP response bytes for dbdump file paths.
func ParseDbdumps(response []byte) []string {
	re := regexp.MustCompile(dbdumpReg)
	dbdumps := re.FindAllString(string(response), -1)
	for i, dbdump := range dbdumps {
		dbdumps[i] = RemoveQuotes(dbdump)
	}
	return dbdumps
}

// parseHashes extracts MD5 hashes from the search result HTML page.
func parseHashes(response []byte, results int) []string {
	var hashes []string
	re := regexp.MustCompile(SearchHref)
	matches := re.FindAllString(string(response), -1)

	var counter int
	md5Re := regexp.MustCompile(SearchMD5)
	for _, m := range matches {
		if counter >= results {
			break
		}
		hash := md5Re.FindString(m)
		if len(hash) == 32 {
			hashes = append(hashes, hash)
			counter++
		}
	}
	return hashes
}

// parseFileResponse parses the JSON response from object=f API.
func parseFileResponse(response []byte) (*Book, string, error) {
	var book Book
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
			if v, ok := item[key]; ok && v != nil {
				return fmt.Sprint(v)
			}
			return ""
		}
		book.ID = id
		book.Filesize = str("filesize")
		book.Extension = str("extension")
		book.Md5 = str("md5")
		book.Pages = str("pages")

		if editions, ok := item["editions"]; ok {
			if edMap, ok := editions.(map[string]interface{}); ok {
				for _, ev := range edMap {
					if edInfo, ok := ev.(map[string]interface{}); ok {
						if eid, ok := edInfo["e_id"]; ok {
							editionID = fmt.Sprint(eid)
						}
					}
					break
				}
			}
		}
		break
	}

	return &book, editionID, nil
}

// parseEditionBatchResponse parses the JSON response from object=e API for multiple IDs.
func parseEditionBatchResponse(response []byte) map[string]editionInfo {
	res := make(map[string]editionInfo)
	var resp map[string]map[string]interface{}
	if err := json.Unmarshal(response, &resp); err != nil {
		return res
	}

	for id, item := range resp {
		str := func(key string) string {
			if v, ok := item[key]; ok && v != nil {
				return fmt.Sprint(v)
			}
			return ""
		}
		res[id] = editionInfo{
			title:     str("title"),
			author:    str("author"),
			year:      str("year"),
			language:  str("language"),
			publisher: str("publisher"),
			edition:   str("edition"),
			coverURL:  str("cover_url"),
		}
	}
	return res
}

// parseEditionResponse fills in book metadata fields from an object=e JSON response.
func parseEditionResponse(response []byte, book *Book) {
	parsed := parseEditionBatchResponse(response)
	for _, info := range parsed {
		book.Title = info.title
		book.Author = info.author
		book.Year = info.year
		book.Language = info.language
		book.Publisher = info.publisher
		book.Edition = info.edition
		book.CoverURL = info.coverURL
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

	fmt.Println(strings.Repeat("-", 80))

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

	var formatAuthor string
	if len(book.Author) > AuthorMaxLength {
		formatAuthor = book.Author[:AuthorMaxLength]
	} else if book.Author == "" {
		formatAuthor = "N/A"
	} else {
		formatAuthor = book.Author
	}

	if err := prettify("author", formatAuthor, color.FgYellow, "-25"); err != nil {
		return err
	}
	if err := prettify("year", book.Year, color.FgCyan, "4"); err != nil {
		return err
	}
	if err := prettify("size", fsize, color.FgGreen, "6"); err != nil {
		return err
	}
	if err := prettify("type", book.Extension, color.FgRed, "4"); err != nil {
		return err
	}
	fmt.Println()
	return nil
}

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

func RemoveQuotes(s string) string {
	if len(s) < 2 {
		return ""
	}
	return s[1 : len(s)-1]
}

func setSortASC(q url.Values, sortASC bool) {
	if sortASC {
		q.Set("sortmode", "ASC")
	} else {
		q.Set("sortmode", "DESC")
	}
}
