// Copyright © 2023 Ryan Ciehanski <ryan@ciehanski.com>
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
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cheggaaa/pb/v3"
)

// IPFSGateways contains public IPFS gateways used for content retrieval.
var IPFSGateways = []string{
	"https://cloudflare-ipfs.com/ipfs/",
	"https://gateway.ipfs.io/ipfs/",
	"https://ipfs.io/ipfs/",
	"https://dweb.link/ipfs/",
}

// DownloadBookIPFS downloads a book via IPFS gateways with automatic gateway fallback.
func DownloadBookIPFS(book *Book, outputPath string) error {
	if book.DownloadURL == "" {
		return errors.New("book has no download URL")
	}

	cidPath, err := parseIPFSPath(book.DownloadURL)
	if err != nil {
		return fmt.Errorf("failed to parse IPFS path from %q: %w", book.DownloadURL, err)
	}

	// Prepare destination directory and filename
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
	tempPath := finalPath + ".tmp"

	// Try candidate URLs: first the book's download URL if it's already a gateway,
	// then the configured IPFS gateways.
	var candidateURLs []string
	if strings.HasPrefix(book.DownloadURL, "http://") || strings.HasPrefix(book.DownloadURL, "https://") {
		candidateURLs = append(candidateURLs, book.DownloadURL)
	}
	for _, gw := range IPFSGateways {
		gwURL := strings.TrimSuffix(gw, "/") + "/" + strings.TrimPrefix(cidPath, "/")
		candidateURLs = append(candidateURLs, gwURL)
	}

	var downloadErr error
	for _, candidate := range candidateURLs {
		err := streamDownloadToFile(context.Background(), candidate, tempPath, finalPath)
		if err == nil {
			return nil
		}
		downloadErr = err
	}

	return fmt.Errorf("failed to download via IPFS gateways: %w", downloadErr)
}

func parseIPFSPath(raw string) (string, error) {
	re := regexp.MustCompile(ipfsReg)
	matches := re.FindStringSubmatch(raw)
	if len(matches) > 1 {
		return matches[1], nil
	}
	// If the raw string is a bare CID
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) >= 46 && !strings.Contains(trimmed, "/") {
		return trimmed, nil
	}
	return "", fmt.Errorf("no valid IPFS CID found in %q", raw)
}

func streamDownloadToFile(ctx context.Context, downloadURL, tempPath, finalPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned HTTP %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/html") && resp.ContentLength < 10000 {
		return fmt.Errorf("gateway returned HTML error document")
	}

	f, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
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

	written, err := io.Copy(f, reader)
	if bar != nil {
		bar.Finish()
	}
	if err != nil {
		return err
	}
	if resp.ContentLength > 0 && written < resp.ContentLength {
		return fmt.Errorf("truncated download: expected %d bytes, got %d", resp.ContentLength, written)
	}

	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Atomically move from temp file to destination
	return os.Rename(tempPath, finalPath)
}
