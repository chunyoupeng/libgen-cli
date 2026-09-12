// Copyright © 2019 Antoine Chiny <antoine.chiny@inria.fr>
// Copyright © 2019 Ryan Ciehanski <ryan@ciehanski.com>
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
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cheggaaa/pb/v3"
)

// DownloadBook grabs the download DownloadURL for the book requested.
// First, it queries Booksdl.org and then b-ok.cc for valid DownloadURL.
// Then, the download process is initiated with a progress bar displayed to
// the user's CLI.
func DownloadBook(book *Book, outputPath string) error {
	filename := getBookFilename(book)
	destPath, err := resolveDestinationPath(outputPath, filename)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", book.DownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Accept-Encoding", "*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	if book.PageURL != "" {
		req.Header.Set("Referer", book.PageURL)
	}
	client := http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
	r, err := client.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("unable to reach mirror %v: HTTP %v", req.Host, r.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	success := false
	defer func() {
		_ = out.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	bar := pb.Full.Start64(r.ContentLength)
	if _, err = io.Copy(out, bar.NewProxyReader(r.Body)); err != nil {
		return err
	}
	bar.Finish()

	if err := out.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return err
	}
	success = true

	return nil
}

// GetDownloadURL picks a download mirror to download the specified
// resource from. First tries the search mirror's ads.php page, then
// falls back to legacy download mirrors.
// GetDownloadURL resolves book.DownloadURL. If searchMirror is non-nil it is
// used as the search mirror for the primary ads.php lookup; otherwise a random
// working search mirror is chosen. The library.lol/libgen.pm fallback is always
// automatic.
func GetDownloadURL(book *Book, useIpfs bool, searchMirror *url.URL) error {
	// If IPFS is requested, DO NOT query search mirror's ads.php (which only yields HTTP links)
	if !useIpfs {
		if err := getSearchMirrorURL(book, searchMirror); err == nil && book.DownloadURL != "" {
			return nil
		}
	}

	// Deterministic fallback through configured download mirrors
	for _, mirror := range DownloadMirrors {
		switch mirror.Hostname() {
		case "library.lol":
			if useIpfs {
				if err := getLibraryLolURL(book, true); err == nil && book.DownloadURL != "" {
					return nil
				}
			} else {
				if err := getLibraryLolURL(book, false); err == nil && book.DownloadURL != "" {
					return nil
				}
				if err := getLibgenPMURL(book); err == nil && book.DownloadURL != "" {
					return nil
				}
			}
		case "libgen.pm":
			if !useIpfs {
				if err := getLibgenPMURL(book); err == nil && book.DownloadURL != "" {
					return nil
				}
				if err := getLibraryLolURL(book, false); err == nil && book.DownloadURL != "" {
					return nil
				}
			} else {
				// No IPFS URLs on libgen.pm pages, fallback to library.lol
				if err := getLibraryLolURL(book, true); err == nil && book.DownloadURL != "" {
					return nil
				}
			}
		}
		if book.DownloadURL != "" {
			return nil
		}
	}

	if book.DownloadURL == "" {
		return fmt.Errorf("unable to retrieve download link for desired resource")
	}
	return nil
}

// getSearchMirrorURL extracts the download URL from the search mirror's
// ads.php page, which contains a direct get.php download link.
func getSearchMirrorURL(book *Book, pinned *url.URL) error {
	var mirror url.URL
	if pinned != nil {
		mirror = *pinned
	} else {
		mirror = GetWorkingMirror(SearchMirrors)
	}
	mirror.Path = "ads.php"
	q := mirror.Query()
	q.Set("md5", book.Md5)
	mirror.RawQuery = q.Encode()

	book.PageURL = mirror.String()

	b, err := getBody(mirror.String())
	if err != nil {
		return err
	}

	// Match the get.php download link
	re := regexp.MustCompile(`get\.php\?md5=\w{32}&key=\w{16}`)
	match := re.FindString(string(b))
	if match == "" {
		return errors.New("no valid download URL found on ads.php page")
	}

	mirror.Path = match
	mirror.RawQuery = ""
	book.DownloadURL = mirror.Scheme + "://" + mirror.Host + "/" + match

	return nil
}

// DownloadDbdump downloads the selected database dump from
// Library Genesis.
func DownloadDbdump(filename string, outputPath string) error {
	mirror, err := FindWorkingMirror(DbdumpsMirrors)
	if err != nil {
		return err
	}
	destPath, err := resolveDestinationPath(outputPath, filename)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", mirror.String(), filename), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	client := http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
	r, err := client.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("unable to reach mirror: HTTP %v", r.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	success := false
	defer func() {
		_ = out.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	bar := pb.Full.Start64(r.ContentLength)
	if _, err = io.Copy(out, bar.NewProxyReader(r.Body)); err != nil {
		return err
	}
	bar.Finish()

	if err := out.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return err
	}
	success = true

	return nil
}

func getLibraryLolURL(book *Book, useIpfs bool) error {
	queryURL := DownloadMirrors[0].String() + book.Md5
	book.PageURL = queryURL

	b, err := getBody(queryURL)
	if err != nil {
		return err
	}

	downloadURL := []byte{}
	if useIpfs {
		// Attempt to find IPFS download URL via gateway.ipfs.io
		downloadURL = findMatch(libraryLolIPFSReg, b)
		if downloadURL == nil {
			// Fallback to cloudflare-ipfs.com
			downloadURL = findMatch(libraryLolIPFSCFReg, b)
			if downloadURL == nil {
				return errors.New("no valid download LibraryLol download URL found")
			}
		}
	} else {
		downloadURL = findMatch(libraryLolReg, b)
		if downloadURL == nil {
			return errors.New("no valid download LibraryLol download URL found")
		}
	}

	book.DownloadURL = string(downloadURL)

	return nil
}

func getLibgenPMURL(book *Book) error {
	queryURL := DownloadMirrors[1].String() + book.Md5
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

func resolveDestinationPath(outputPath, filename string) (string, error) {
	// Handle long titles safely without breaking UTF-8 or extension
	if len([]rune(filename)) > 200 {
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, ext)
		r := []rune(base)
		if len(r) > 180 {
			base = string(r[:180])
		}
		filename = base + ext
	}

	var targetDir string
	if outputPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		targetDir = filepath.Join(wd, "libgen")
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return "", err
		}
	} else {
		stat, err := os.Stat(outputPath)
		if err != nil || !stat.IsDir() {
			return "", errors.New("invalid output path")
		}
		targetDir = outputPath
	}
	return filepath.Join(targetDir, filename), nil
}

func makeFile(outputPath, filename string) (*os.File, error) {
	destPath, err := resolveDestinationPath(outputPath, filename)
	if err != nil {
		return nil, err
	}
	return os.Create(destPath)
}

// findMatch is a helper function that searches an []byte
// for a specified regex and returns the matches.
func findMatch(reg string, response []byte) []byte {
	re := regexp.MustCompile(reg)
	match := re.FindString(string(response))

	if match != "" {
		return []byte(match)
	}

	return nil
}

func sanitizeFilename(name string) string {
	invalidChars := regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]`)
	sanitized := invalidChars.ReplaceAllString(name, "_")
	return strings.TrimSpace(sanitized)
}

func getBookFilename(book *Book) string {
	title := sanitizeFilename(book.Title)
	if title == "" {
		title = book.Md5
	}
	author := sanitizeFilename(book.Author)
	ext := strings.TrimPrefix(sanitizeFilename(book.Extension), ".")
	if ext == "" {
		ext = "unknown"
	}
	if author != "" {
		return fmt.Sprintf("%s by %s.%s", title, author, ext)
	}
	return fmt.Sprintf("%s.%s", title, ext)
}
