// Copyright © 2019 Ryan Ciehanski <ryan@ciehanski.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package libgen_cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/chunyoupeng/libgen-cli/libgen"
)

var downloadAllCmd = &cobra.Command{
	Use:     "download-all",
	Short:   "Downloads all found resources for a specified query.",
	Long:    `Searches for a specific query and downloads all the results found.`,
	Example: "libgen download-all kubernetes",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) < 1 {
			if err := cmd.Help(); err != nil {
				fmt.Printf("error displaying CLI help: %v\n", err)
			}
			os.Exit(1)
		}

		// Get flags
		results, err := cmd.Flags().GetInt("results")
		if err != nil {
			fmt.Printf("error getting results flag: %v\n", err)
		}
		requireAuthor, err := cmd.Flags().GetBool("require-author")
		if err != nil {
			fmt.Printf("error getting require-author flag: %v\n", err)
		}
		extension, err := cmd.Flags().GetStringSlice("extension")
		if err != nil {
			fmt.Printf("error getting extension flag: %v\n", err)
		}
		output, err := cmd.Flags().GetString("output")
		if err != nil {
			fmt.Printf("error getting output flag: %v\n", err)
		}
		year, err := cmd.Flags().GetInt("year")
		if err != nil {
			fmt.Printf("error getting output flag: %v\n", err)
		}
		publisher, err := cmd.Flags().GetString("publisher")
		if err != nil {
			fmt.Printf("error getting publisher flag: %v\n", err)
		}
		language, err := cmd.Flags().GetString("language")
		if err != nil {
			fmt.Printf("error getting language flag: %v\n", err)
		}
		useIpfs, err := cmd.Flags().GetBool("ipfs-mirrors")
		if err != nil {
			fmt.Printf("error getting ipfs-mirrors flag: %v\n", err)
		}
		sortBy, err := cmd.Flags().GetString("sort-by")
		if err != nil {
			fmt.Printf("error getting sort-by flag: %v\n", err)
		}
		sortASC, err := cmd.Flags().GetBool("sort-asc")
		if err != nil {
			fmt.Printf("error getting sort-asc flag: %v\n", err)
		}
		mirror, err := cmd.Flags().GetString("mirror")
		if err != nil {
			fmt.Printf("error getting mirror flag: %v\n", err)
		}

		var cleanExt []string
		for _, e := range extension {
			if strings.TrimSpace(e) != "" {
				cleanExt = append(cleanExt, strings.TrimSpace(e))
			}
		}

		// Join args for complete search query in case
		// it contains spaces
		searchQuery := strings.Join(args, " ")
		fmt.Printf("++ Downloading all for: %s\n", searchQuery)

		searchMirror, pinnedMirror, err := resolveSearchMirror(mirror)
		if err != nil {
			fmt.Printf("error selecting search mirror: %v\n", err)
			os.Exit(1)
		}

		books, err := libgen.Search(&libgen.SearchOptions{
			Query:         searchQuery,
			SearchMirror:  searchMirror,
			Results:       results,
			RequireAuthor: requireAuthor,
			Extension:     cleanExt,
			Year:          year,
			Publisher:     publisher,
			Language:      language,
			SortBy:        sortBy,
			SortASC:       sortASC,
		})
		if err != nil {
			fmt.Printf("error completing search query: %v\n", err)
			os.Exit(1)
		}
		if len(books) == 0 {
			fmt.Printf("No books found for %q\n", searchQuery)
			os.Exit(0)
		}

		// Limit concurrent downloads to 2 to prevent network and terminal contention
		sem := make(chan struct{}, 2)
		var wg sync.WaitGroup
		var failedCount int
		var mu sync.Mutex

		for _, book := range books {
			if err := libgen.GetDownloadURL(book, useIpfs, pinnedMirror); err != nil {
				fmt.Printf("error getting download URL for %q: %v\n", book.Title, err)
				mu.Lock()
				failedCount++
				mu.Unlock()
				continue
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(curBook *libgen.Book) {
				defer func() {
					<-sem
					wg.Done()
				}()

				var dlErr error
				if useIpfs {
					dlErr = libgen.DownloadBookIPFS(curBook, output)
				} else {
					dlErr = libgen.DownloadBook(curBook, output)
				}
				if dlErr != nil {
					fmt.Printf("error downloading %q: %v\n", curBook.Title, dlErr)
					mu.Lock()
					failedCount++
					mu.Unlock()
				}
			}(book)
		}
		wg.Wait()

		if failedCount > 0 {
			fmt.Printf("%s Completed with %d failure(s) out of %d books.\n", color.YellowString("[WARN]"), failedCount, len(books))
			os.Exit(1)
		}

		if runtime.GOOS == "windows" {
			_, err = fmt.Fprintf(color.Output, "%s\n", color.GreenString("[DONE]"))
			if err != nil {
				fmt.Printf("error writing to Windows os.Stdout: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("%s\n", color.GreenString("[DONE]"))
		}
	},
}

func init() {
	downloadAllCmd.Flags().IntP("results", "r", 10, "controls "+
		"how many query results are displayed.")
	downloadAllCmd.Flags().BoolP("require-author", "a", false, "controls if the query "+
		"results will return any media without a listed author.")
	downloadAllCmd.Flags().StringSliceP("extension", "e", []string{""}, "controls if the query results "+
		"will return any media with a certain file extension.")
	downloadAllCmd.Flags().StringP("output", "o", "", "where you want libgen-cli to "+
		"save your download.")
	downloadAllCmd.Flags().IntP("year", "y", 0, "filters search query results by the "+
		"year provided.")
	downloadAllCmd.Flags().StringP("publisher", "p", "", "filters search query "+
		"results by the publisher provided")
	downloadAllCmd.Flags().StringP("language", "l", "", "filters search query "+
		"results by the language provided")
	downloadAllCmd.Flags().BoolP("ipfs-mirrors", "i", false, "enforces libgen-cli to download "+
		"results via IPFS mirrors instead of HTTP(S) mirrors.")
	downloadAllCmd.Flags().StringP("sort-by", "s", "", "sorts the queried results "+
		"by the specified string. (id, title, author, pub, year, lang, size, ext)")
	downloadAllCmd.Flags().Bool("sort-asc", true, "sorts the queried results "+
		"by ascension or descension.")
	downloadAllCmd.Flags().StringP("mirror", "m", "", "pin a specific search mirror "+
		"by host (e.g. libgen.li) instead of auto-selecting one. run 'libgen status -m search' to list mirrors.")
}
