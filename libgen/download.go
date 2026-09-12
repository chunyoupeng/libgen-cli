// Copyright © 2019 Antoine Chiny <antoine.chiny@inria.fr>
// Copyright © 2019 Ryan Ciehanski <ryan@ciehanski.com>
// Copyright © 2026 Chunyou Peng <chunyoupeng@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cheggaaa/pb/v3"
)

// DownloadBook downloads the resource identified by book.DownloadURL to outputPath.
// It streams into a temporary file in the destination directory, validates content,
// and atomically renames to the final filename upon completion.
func DownloadBook(book *Book, outputPath string) error {
	if book == nil || book.DownloadURL == "" {
		return errors.New("no download URL available for book")
	}

	filename := getBookFilename(book)
	targetDir := outputPath
	if targetDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		targetDir = filepath.Join(wd, "libgen")
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	finalPath := filepath.Join(targetDir, filename)
	if stat, err := os.Stat(finalPath); err == nil && stat.Size() > 0 {
		return nil
	}

	tempPath := finalPath + ".tmp"
	var totalSize int64
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		var downloaded int64
		if stat, err := os.Stat(tempPath); err == nil {
			downloaded = stat.Size()
		}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, book.DownloadURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", DefaultUserAgent)
		req.Header.Set("Accept", "*/*")
		if book.PageURL != "" {
			req.Header.Set("Referer", book.PageURL)
		} else if u, err := url.Parse(book.DownloadURL); err == nil && u.Scheme != "" && u.Host != "" {
			req.Header.Set("Referer", fmt.Sprintf("%s://%s/index.php", u.Scheme, u.Host))
		}

		if downloaded > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
		}

		resp, err := downloadHTTPClient.Do(req)
		if err == nil && (resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests) {
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			_ = GetDownloadURL(book, false, nil)
			continue
		}
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			_ = GetDownloadURL(book, false, nil)
			continue
		}

		ct := resp.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "text/html") && resp.ContentLength < 10000 {
			resp.Body.Close()
			return fmt.Errorf("mirror returned HTML error page instead of media")
		}

		var f *os.File
		if resp.StatusCode == http.StatusPartialContent {
			if totalSize == 0 {
				if cr := resp.Header.Get("Content-Range"); cr != "" {
					parts := strings.Split(cr, "/")
					if len(parts) == 2 {
						totalSize, _ = strconv.ParseInt(parts[1], 10, 64)
					}
				}
				if totalSize == 0 {
					totalSize = downloaded + resp.ContentLength
				}
			}
			f, err = os.OpenFile(tempPath, os.O_WRONLY|os.O_APPEND, 0644)
		} else {
			totalSize = resp.ContentLength
			downloaded = 0
			f, err = os.Create(tempPath)
		}
		if err != nil {
			resp.Body.Close()
			return err
		}

		var reader io.Reader = resp.Body
		var bar *pb.ProgressBar
		if totalSize > 0 {
			bar = pb.Full.Start64(totalSize)
			bar.SetCurrent(downloaded)
			reader = bar.NewProxyReader(resp.Body)
		}

		copied, copyErr := io.Copy(f, reader)
		downloaded += copied
		if bar != nil {
			bar.Finish()
		}
		_ = f.Sync()
		_ = f.Close()
		_ = resp.Body.Close()

		if copyErr == nil && (totalSize == 0 || downloaded >= totalSize) {
			return os.Rename(tempPath, finalPath)
		}

		// Interrupted by network timeout or EOF, retry and resume with Range
		time.Sleep(2 * time.Second)
	}

	_ = os.Remove(tempPath)
	return fmt.Errorf("download failed after %d retries", maxRetries)
}

// GetDownloadURL resolves book.DownloadURL using a multi-tiered fallback.
// In normal mode:
//   Tier 1: search mirror's ads.php page (with same-origin Referer)
//   Tier 2: alternative search mirrors if primary fails
//   Tier 3: search mirror's file.php page if book.ID is available
//   Tier 4: library.lol / libgen.pm download mirrors
// In useIpfs mode:
//   Resolves IPFS gateway links via file.php or library.lol
func GetDownloadURL(book *Book, useIpfs bool, searchMirror *url.URL) error {
	if book == nil {
		return errors.New("book is nil")
	}

	if useIpfs {
		// IPFS mode: extract IPFS gateway link from file.php or library.lol
		if book.ID != "" {
			if err := getIPFSFromFilePage(book, searchMirror); err == nil && book.DownloadURL != "" {
				return nil
			}
		}
		if err := getLibraryLolURL(book, true); err == nil && book.DownloadURL != "" {
			return nil
		}
		return fmt.Errorf("unable to retrieve IPFS download link for book: %s", book.Title)
	}

	// Tier 1: Search mirror ads.php
	if err := getSearchMirrorURL(book, searchMirror); err == nil && book.DownloadURL != "" {
		return nil
	}

	// Tier 2: Try other search mirrors if unpinned
	if searchMirror == nil {
		working := GetWorkingMirror(SearchMirrors)
		if working.Host != "" {
			if err := getSearchMirrorURL(book, &working); err == nil && book.DownloadURL != "" {
				return nil
			}
		}
	}

	// Tier 3: Search mirror file.php
	if book.ID != "" {
		if err := getDirectLinkFromFilePage(book, searchMirror); err == nil && book.DownloadURL != "" {
			return nil
		}
	}

	// Tier 4: Fallback to download mirrors (library.lol / libgen.pm)
	for _, dm := range DownloadMirrors {
		switch dm.Hostname() {
		case "library.lol":
			if err := getLibraryLolURL(book, false); err == nil && book.DownloadURL != "" {
				return nil
			}
		case "libgen.pm":
			if err := getLibgenPMURL(book); err == nil && book.DownloadURL != "" {
				return nil
			}
		}
	}

	return fmt.Errorf("unable to retrieve download link for desired resource: %s", book.Title)
}

// getSearchMirrorURL extracts the get.php download URL from the search mirror's ads.php page.
func getSearchMirrorURL(book *Book, pinned *url.URL) error {
	var mirror url.URL
	if pinned != nil && pinned.Host != "" {
		mirror = *pinned
	} else {
		working := GetWorkingMirror(SearchMirrors)
		if working.Host == "" {
			return errors.New("no working search mirror available")
		}
		mirror = working
	}

	q := url.Values{}
	q.Set("md5", book.Md5)
	adsURL := endpoint(mirror, "ads.php", q)
	referer := endpoint(mirror, "index.php", nil)

	book.PageURL = adsURL.String()

	b, err := getBodyWithReferer(context.Background(), adsURL.String(), referer.String())
	if err != nil {
		return err
	}

	re := regexp.MustCompile(`get\.php\?md5=\w{32}&key=\w{16}`)
	match := re.FindString(string(b))
	if match == "" {
		return errors.New("no valid download URL found on ads.php page")
	}

	book.DownloadURL = fmt.Sprintf("%s://%s/%s", mirror.Scheme, mirror.Host, match)
	return nil
}

// getIPFSFromFilePage extracts IPFS gateway links from the file.php page.
func getIPFSFromFilePage(book *Book, pinned *url.URL) error {
	var mirror url.URL
	if pinned != nil && pinned.Host != "" {
		mirror = *pinned
	} else {
		mirror = GetWorkingMirror(SearchMirrors)
	}
	if mirror.Host == "" {
		return errors.New("no working search mirror available")
	}

	q := url.Values{}
	q.Set("id", book.ID)
	fileURL := endpoint(mirror, "file.php", q)
	referer := endpoint(mirror, "index.php", nil)

	b, err := getBodyWithReferer(context.Background(), fileURL.String(), referer.String())
	if err != nil {
		return err
	}

	re := regexp.MustCompile(`https?://[^"]*ipfs[^"]*`)
	matches := re.FindAllString(string(b), -1)
	for _, m := range matches {
		if strings.Contains(m, "/ipfs/") {
			book.DownloadURL = m
			book.PageURL = fileURL.String()
			return nil
		}
	}

	return errors.New("no IPFS link found on file.php")
}

// getDirectLinkFromFilePage checks file.php for direct download or mirror links.
func getDirectLinkFromFilePage(book *Book, pinned *url.URL) error {
	var mirror url.URL
	if pinned != nil && pinned.Host != "" {
		mirror = *pinned
	} else {
		mirror = GetWorkingMirror(SearchMirrors)
	}
	if mirror.Host == "" {
		return errors.New("no working search mirror available")
	}

	q := url.Values{}
	q.Set("id", book.ID)
	fileURL := endpoint(mirror, "file.php", q)
	referer := endpoint(mirror, "index.php", nil)

	b, err := getBodyWithReferer(context.Background(), fileURL.String(), referer.String())
	if err != nil {
		return err
	}

	// Try IPFS gateway link as direct HTTP fallback
	re := regexp.MustCompile(`https?://(cloudflare-ipfs\.com|gateway\.ipfs\.io)/ipfs/[^"]+`)
	match := re.FindString(string(b))
	if match != "" {
		book.DownloadURL = match
		book.PageURL = fileURL.String()
		return nil
	}

	return errors.New("no alternative download link found on file.php")
}

// DownloadDbdump downloads the selected database dump from Library Genesis.
func DownloadDbdump(filename string, outputPath string) error {
	mirror, err := FindWorkingMirror(DbdumpsMirrors)
	if err != nil {
		return err
	}

	targetDir := outputPath
	if targetDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		targetDir = filepath.Join(wd, "libgen")
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	finalPath := filepath.Join(targetDir, filename)
	tempPath := finalPath + ".tmp"

	downloadURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(mirror.String(), "/"), filename)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unable to reach mirror: HTTP %v", resp.StatusCode)
	}

	f, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		if _, statErr := os.Stat(tempPath); statErr == nil {
			_ = os.Remove(tempPath)
		}
	}()

	var reader io.Reader = resp.Body
	var bar *pb.ProgressBar
	if resp.ContentLength > 0 {
		bar = pb.Full.Start64(resp.ContentLength)
		reader = bar.NewProxyReader(resp.Body)
	}

	_, err = io.Copy(f, reader)
	if bar != nil {
		bar.Finish()
	}
	if err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, finalPath)
}

func getLibraryLolURL(book *Book, useIpfs bool) error {
	queryURL := strings.TrimSuffix(DownloadMirrors[0].String(), "/") + "/" + book.Md5
	book.PageURL = queryURL

	b, err := getBody(queryURL)
	if err != nil {
		return err
	}

	var downloadURL []byte
	if useIpfs {
		downloadURL = findMatch(libraryLolIPFSReg, b)
		if downloadURL == nil {
			downloadURL = findMatch(libraryLolIPFSCFReg, b)
		}
	} else {
		downloadURL = findMatch(libraryLolReg, b)
	}

	if downloadURL == nil {
		return errors.New("no valid LibraryLol download URL found")
	}

	book.DownloadURL = string(downloadURL)
	return nil
}

func getLibgenPMURL(book *Book) error {
	queryURL := strings.TrimSuffix(DownloadMirrors[1].String(), "/") + "/" + book.Md5
	book.PageURL = queryURL

	b, err := getBody(queryURL)
	if err != nil {
		return err
	}

	downloadURL := findMatch(libgenPMReg, b)
	if downloadURL == nil {
		return errors.New("no valid LibgenPM download URL found")
	}
	book.DownloadURL = fmt.Sprintf("https://libgen.rocks/%s", string(downloadURL))
	return nil
}

func findMatch(reg string, response []byte) []byte {
	re := regexp.MustCompile(reg)
	match := re.FindString(string(response))
	if match != "" {
		return []byte(match)
	}
	return nil
}

func sanitizeFilename(name string) string {
	illegal := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\n", "\r", "\t"}
	clean := name
	for _, char := range illegal {
		clean = strings.ReplaceAll(clean, char, "_")
	}
	return strings.TrimSpace(clean)
}

func getBookFilename(book *Book) string {
	title := sanitizeFilename(book.Title)
	if title == "" {
		title = book.Md5
	}
	author := sanitizeFilename(book.Author)
	ext := sanitizeFilename(book.Extension)
	if ext == "" {
		ext = "pdf"
	}
	ext = strings.TrimPrefix(ext, ".")

	var filename string
	if author != "" && author != "N_A" && author != "N/A" {
		filename = fmt.Sprintf("%s by %s.%s", title, author, ext)
	} else {
		filename = fmt.Sprintf("%s.%s", title, ext)
	}

	// Preserve extension when truncating
	if len(filename) > 200 {
		suffix := fmt.Sprintf(".%s", ext)
		maxTitleLen := 200 - len(suffix)
		if maxTitleLen > 0 && len(filename) > maxTitleLen {
			filename = filename[:maxTitleLen] + suffix
		}
	}
	return filename
}
