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
	"fmt"
	"net/url"
	"strings"
)

// MirrorHosts returns the host names of every mirror in the provided list.
func MirrorHosts(mirrors []url.URL) []string {
	hosts := make([]string, len(mirrors))
	for i, m := range mirrors {
		hosts[i] = m.Host
	}
	return hosts
}

// MirrorByHost returns the mirror from the list whose host matches the given
// host (case-insensitive). The host may be a bare host like "libgen.li" or a
// full URL like "https://libgen.li"; a leading "www." is ignored. An error is
// returned, listing the available hosts, when no mirror matches.
func MirrorByHost(mirrors []url.URL, host string) (url.URL, error) {
	h := strings.ToLower(strings.TrimSpace(host))
	if strings.Contains(h, "://") {
		if u, err := url.Parse(h); err == nil && u.Host != "" {
			h = u.Host
		}
	}
	h = strings.TrimPrefix(h, "www.")
	for _, m := range mirrors {
		if strings.ToLower(m.Host) == h {
			return m, nil
		}
	}
	return url.URL{}, fmt.Errorf("mirror %q not found; available: %s",
		host, strings.Join(MirrorHosts(mirrors), ", "))
}

// SearchMirrors contains all valid and tested mirrors used for
// querying against Library Genesis.
var SearchMirrors = []url.URL{
	{
		Scheme: "https",
		Host:   "libgen.li",
		Path:   "index.php",
	},
	{
		Scheme: "https",
		Host:   "libgen.vg",
		Path:   "index.php",
	},
	{
		Scheme: "https",
		Host:   "libgen.bz",
		Path:   "index.php",
	},
	{
		Scheme: "https",
		Host:   "libgen.gl",
		Path:   "index.php",
	},
	{
		Scheme: "https",
		Host:   "libgen.is",
		Path:   "index.php",
	},
	{
		Scheme: "https",
		Host:   "libgen.la",
		Path:   "index.php",
	},
	{
		Scheme: "https",
		Host:   "libgen.rs",
		Path:   "index.php",
	},
	{
		Scheme: "https",
		Host:   "libgen.st",
		Path:   "index.php",
	},
	{
		Scheme: "https",
		Host:   "libgen.gs",
		Path:   "index.php",
	},
	{
		Scheme: "http",
		Host:   "gen.lib.rus.ec",
		Path:   "index.php",
	},
}

// DownloadMirrors contains all valid and tested mirrors used for
// downloading content from Library Genesis.
var DownloadMirrors = []url.URL{
	{
		Scheme: "https",
		Host:   "library.lol",
		Path:   "main/",
	},
	{
		Scheme: "https",
		Host:   "libgen.pm",
		Path:   "ads",
	},
}

var UploadMirrors = []url.URL{
	{
		Scheme: "https",
		Host:   "library.bz",
		Path:   "/main/upload",
	},
}

var DbdumpsMirrors = []url.URL{
	{
		Scheme: "https",
		Host:   "data.library.bz",
		Path:   "/dbdumps",
	},
}
